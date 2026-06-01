package resource

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONMap_ValueScanRoundTrip(t *testing.T) {
	original := JSONMap{"key1": "val1", "key2": "val2"}
	v, err := original.Value()
	require.NoError(t, err)

	var restored JSONMap
	err = restored.Scan(v)
	require.NoError(t, err)
	assert.Equal(t, original, restored)
}

func TestJSONMap_Nil(t *testing.T) {
	var m JSONMap
	v, err := m.Value()
	require.NoError(t, err)
	assert.Nil(t, v)

	var restored JSONMap
	err = restored.Scan(nil)
	require.NoError(t, err)
	assert.Nil(t, restored)
}

type testSpec struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestJSONField_ValueScanRoundTrip(t *testing.T) {
	original := JSONField[testSpec]{Data: testSpec{Name: "test", Count: 5}}
	v, err := original.Value()
	require.NoError(t, err)

	var restored JSONField[testSpec]
	err = restored.Scan(v)
	require.NoError(t, err)
	assert.Equal(t, original.Data, restored.Data)
}

func TestJSONField_ScanNil(t *testing.T) {
	f := JSONField[testSpec]{Data: testSpec{Name: "should-be-cleared"}}
	err := f.Scan(nil)
	require.NoError(t, err)
	assert.Equal(t, testSpec{}, f.Data)
}

func TestResourceAccessor(t *testing.T) {
	orgID := uuid.New()
	r := &Resource{
		OrgID:           orgID,
		Name:            "my-resource",
		ResourceVersion: 3,
		Generation:      2,
	}

	assert.Equal(t, orgID, r.GetOrgID())
	assert.Equal(t, "my-resource", r.GetName())
	assert.Equal(t, int64(3), r.GetResourceVersion())
	assert.Equal(t, int64(2), r.GetGeneration())

	r.SetResourceVersion(10)
	assert.Equal(t, int64(10), r.GetResourceVersion())

	r.SetGeneration(5)
	assert.Equal(t, int64(5), r.GetGeneration())
}

func TestResource_BeforeCreate(t *testing.T) {
	r := &Resource{}
	err := r.BeforeCreate(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), r.Generation)
	assert.Equal(t, int64(1), r.ResourceVersion)
}

func TestResource_BeforeCreate_PreserveExisting(t *testing.T) {
	r := &Resource{Generation: 5, ResourceVersion: 10}
	err := r.BeforeCreate(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(5), r.Generation)
	assert.Equal(t, int64(10), r.ResourceVersion)
}
