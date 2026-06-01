// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

// EventRecord is the GORM model for the events table.
type EventRecord struct {
	ID        string    `gorm:"primaryKey"`
	OrgID     uuid.UUID `gorm:"type:uuid;index"`
	Kind      string    `gorm:"index"`
	Name      string
	Action    string
	Payload   []byte
	CreatedAt time.Time `gorm:"index"`
}

func (EventRecord) TableName() string { return "events" }

const pgChannel = "infractl_events"

// notifyPayload is the compact JSON sent via pg_notify.
type notifyPayload struct {
	ID     string `json:"id"`
	OrgID  string `json:"org_id"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Action string `json:"action"`
}

type pgSubscriber struct {
	kinds map[string]bool
	ch    chan Event
	ctx   context.Context
}

// PgBus is a PostgreSQL NOTIFY/LISTEN-backed event bus.
type PgBus struct {
	db      *gorm.DB
	connStr string
	logger  zerolog.Logger

	mu       sync.RWMutex
	subs     []pgSubscriber
	listening bool
	cancel   context.CancelFunc
}

var _ Bus = (*PgBus)(nil)

// NewPgBus creates a PostgreSQL-backed event bus. It auto-migrates the events
// table on construction.
func NewPgBus(db *gorm.DB, connStr string, logger zerolog.Logger) (*PgBus, error) {
	if err := db.AutoMigrate(&EventRecord{}); err != nil {
		return nil, err
	}
	return &PgBus{
		db:      db,
		connStr: connStr,
		logger:  logger.With().Str("component", "pg_bus").Logger(),
	}, nil
}

// Publish inserts an event into the events table and sends a pg_notify.
func (b *PgBus) Publish(ctx context.Context, event Event) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	record := EventRecord{
		ID:        event.ID,
		OrgID:     event.OrgID,
		Kind:      event.Kind,
		Name:      event.Name,
		Action:    event.Action,
		Payload:   event.Payload,
		CreatedAt: event.Timestamp,
	}
	if err := b.db.WithContext(ctx).Create(&record).Error; err != nil {
		return err
	}

	np := notifyPayload{
		ID:     event.ID,
		OrgID:  event.OrgID.String(),
		Kind:   event.Kind,
		Name:   event.Name,
		Action: event.Action,
	}
	payload, err := json.Marshal(np)
	if err != nil {
		return err
	}

	return b.db.WithContext(ctx).Exec("SELECT pg_notify(?, ?)", pgChannel, string(payload)).Error
}

// Subscribe returns a channel that receives events matching the given kinds.
// Pass an empty slice to receive all events.
func (b *PgBus) Subscribe(ctx context.Context, kinds []string) (<-chan Event, error) {
	ch := make(chan Event, 100)
	kindSet := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		kindSet[k] = true
	}

	b.mu.Lock()
	b.subs = append(b.subs, pgSubscriber{kinds: kindSet, ch: ch, ctx: ctx})
	shouldStart := !b.listening
	b.listening = true
	b.mu.Unlock()

	if shouldStart {
		listenCtx, cancel := context.WithCancel(context.Background())
		b.cancel = cancel
		go b.listenLoop(listenCtx)
	}

	go func() {
		<-ctx.Done()
		b.removeSub(ch)
	}()

	return ch, nil
}

func (b *PgBus) listenLoop(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := b.listen(ctx); err != nil {
			b.logger.Error().Err(err).Msg("LISTEN connection failed, reconnecting in 5s")
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func (b *PgBus) listen(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, b.connStr)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, "LISTEN "+pgChannel); err != nil {
		return err
	}

	b.logger.Info().Msg("LISTEN started on " + pgChannel)

	for {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}

		var np notifyPayload
		if err := json.Unmarshal([]byte(notification.Payload), &np); err != nil {
			b.logger.Warn().Err(err).Msg("malformed notification payload")
			continue
		}

		orgID, _ := uuid.Parse(np.OrgID)
		event := Event{
			ID:        np.ID,
			OrgID:     orgID,
			Kind:      np.Kind,
			Name:      np.Name,
			Action:    np.Action,
			Timestamp: time.Now(),
		}

		b.mu.RLock()
		for _, sub := range b.subs {
			if sub.ctx.Err() != nil {
				continue
			}
			if len(sub.kinds) == 0 || sub.kinds[event.Kind] {
				select {
				case sub.ch <- event:
				default:
				}
			}
		}
		b.mu.RUnlock()
	}
}

func (b *PgBus) removeSub(ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, sub := range b.subs {
		if sub.ch == ch {
			b.subs = append(b.subs[:i], b.subs[i+1:]...)
			close(ch)
			break
		}
	}
}

// StartCleanup runs periodic deletion of events older than retention.
func (b *PgBus) StartCleanup(ctx context.Context, retention, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().Add(-retention)
				result := b.db.WithContext(ctx).
					Where("created_at < ?", cutoff).
					Delete(&EventRecord{})
				if result.Error != nil {
					b.logger.Error().Err(result.Error).Msg("event cleanup failed")
				} else if result.RowsAffected > 0 {
					b.logger.Info().Int64("deleted", result.RowsAffected).Msg("cleaned up old events")
				}
			}
		}
	}()
}

// Close stops the LISTEN goroutine.
func (b *PgBus) Close() {
	if b.cancel != nil {
		b.cancel()
	}
}
