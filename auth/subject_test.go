package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewTenantSet(t *testing.T) {
	ts := NewTenantSet("a", "b")
	assert.True(t, ts.Contains("a"))
	assert.True(t, ts.Contains("b"))
	assert.False(t, ts.Contains("c"))
	assert.Equal(t, 2, ts.Len())
	assert.False(t, ts.IsUniversal())
}

func TestUniversalTenantSet(t *testing.T) {
	ts := UniversalTenantSet()
	assert.True(t, ts.Contains("anything"))
	assert.True(t, ts.Contains(""))
	assert.True(t, ts.IsUniversal())
	assert.Equal(t, -1, ts.Len())
	assert.Nil(t, ts.Values())
}

func TestTenantSet_Add(t *testing.T) {
	ts := NewTenantSet("a")
	assert.Equal(t, 1, ts.Len())
	ts.Add("b")
	assert.Equal(t, 2, ts.Len())
	assert.True(t, ts.Contains("b"))
}

func TestUniversalTenantSet_AddIsNoOp(t *testing.T) {
	ts := UniversalTenantSet()
	ts.Add("x")
	assert.True(t, ts.IsUniversal())
	assert.Nil(t, ts.Values())
}
