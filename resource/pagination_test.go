package resource

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContinueToken_RoundTrip(t *testing.T) {
	original := ContinueToken{Offset: 42, ResourceVersion: 7}
	encoded := EncodeContinueToken(original)
	decoded, err := DecodeContinueToken(encoded)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestContinueToken_ZeroValueRoundTrip(t *testing.T) {
	original := ContinueToken{}
	encoded := EncodeContinueToken(original)
	decoded, err := DecodeContinueToken(encoded)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestDecodeContinueToken_InvalidBase64(t *testing.T) {
	_, err := DecodeContinueToken("!!!not-base64!!!")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid continue token encoding")
}

func TestDecodeContinueToken_ValidBase64InvalidJSON(t *testing.T) {
	encoded := base64.RawURLEncoding.EncodeToString([]byte("not json"))
	_, err := DecodeContinueToken(encoded)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid continue token payload")
}
