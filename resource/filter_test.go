package resource

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Backwards compatibility tests (existing behavior) ---

func TestParseFilter_SingleEquality(t *testing.T) {
	clause, args, err := ParseFilter("name=foo")
	require.NoError(t, err)
	assert.Equal(t, "name = ?", clause)
	assert.Equal(t, []interface{}{"foo"}, args)
}

func TestParseFilter_MultipleEqualities(t *testing.T) {
	clause, args, err := ParseFilter("name=foo,kind=bar")
	require.NoError(t, err)
	assert.Equal(t, "name = ? AND kind = ?", clause)
	assert.Equal(t, []interface{}{"foo", "bar"}, args)
}

func TestParseFilter_EmptyString(t *testing.T) {
	clause, args, err := ParseFilter("")
	require.NoError(t, err)
	assert.Empty(t, clause)
	assert.Nil(t, args)
}

func TestParseFilter_EmptyFieldName(t *testing.T) {
	_, _, err := ParseFilter("=value")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty field name")
}

func TestParseFilter_InvalidCharsInFieldName(t *testing.T) {
	_, _, err := ParseFilter("drop;table=x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid field name")
}

func TestParseFilter_FieldNameTooLong(t *testing.T) {
	longName := strings.Repeat("a", 65)
	_, _, err := ParseFilter(longName + "=x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid field name")
}

// --- New operator tests ---

func TestParseFilter_NotEqual(t *testing.T) {
	clause, args, err := ParseFilter("name!=old-machine")
	require.NoError(t, err)
	assert.Equal(t, "name != ?", clause)
	assert.Equal(t, []interface{}{"old-machine"}, args)
}

func TestParseFilter_GreaterThan(t *testing.T) {
	clause, args, err := ParseFilter("cpus>4")
	require.NoError(t, err)
	assert.Equal(t, "cpus > ?", clause)
	assert.Equal(t, []interface{}{"4"}, args)
}

func TestParseFilter_GreaterThanOrEqual(t *testing.T) {
	clause, args, err := ParseFilter("cpus>=4")
	require.NoError(t, err)
	assert.Equal(t, "cpus >= ?", clause)
	assert.Equal(t, []interface{}{"4"}, args)
}

func TestParseFilter_LessThan(t *testing.T) {
	clause, args, err := ParseFilter("cpus<8")
	require.NoError(t, err)
	assert.Equal(t, "cpus < ?", clause)
	assert.Equal(t, []interface{}{"8"}, args)
}

func TestParseFilter_LessThanOrEqual(t *testing.T) {
	clause, args, err := ParseFilter("cpus<=8")
	require.NoError(t, err)
	assert.Equal(t, "cpus <= ?", clause)
	assert.Equal(t, []interface{}{"8"}, args)
}

func TestParseFilter_MissingOperator(t *testing.T) {
	_, _, err := ParseFilter("nooperator")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing operator")
}

// --- AND tests ---

func TestParseFilter_ANDKeyword(t *testing.T) {
	clause, args, err := ParseFilter("name=foo AND cpus>=4")
	require.NoError(t, err)
	assert.Equal(t, "name = ? AND cpus >= ?", clause)
	assert.Equal(t, []interface{}{"foo", "4"}, args)
}

func TestParseFilter_CommaAndANDMixed(t *testing.T) {
	clause, args, err := ParseFilter("name=foo,kind=bar AND cpus>=4")
	require.NoError(t, err)
	assert.Equal(t, "name = ? AND kind = ? AND cpus >= ?", clause)
	assert.Equal(t, []interface{}{"foo", "bar", "4"}, args)
}

// --- OR tests ---

func TestParseFilter_OR(t *testing.T) {
	clause, args, err := ParseFilter("name=foo OR name=bar")
	require.NoError(t, err)
	assert.Equal(t, "(name = ?) OR (name = ?)", clause)
	assert.Equal(t, []interface{}{"foo", "bar"}, args)
}

func TestParseFilter_ORWithMultipleANDTerms(t *testing.T) {
	clause, args, err := ParseFilter("name=foo,kind=vm OR name=bar,kind=host")
	require.NoError(t, err)
	assert.Equal(t, "(name = ? AND kind = ?) OR (name = ? AND kind = ?)", clause)
	assert.Equal(t, []interface{}{"foo", "vm", "bar", "host"}, args)
}

func TestParseFilter_ThreeORGroups(t *testing.T) {
	clause, args, err := ParseFilter("a=1 OR b=2 OR c=3")
	require.NoError(t, err)
	assert.Equal(t, "(a = ?) OR (b = ?) OR (c = ?)", clause)
	assert.Equal(t, []interface{}{"1", "2", "3"}, args)
}

// --- JSONB label/annotation tests ---

func TestParseFilter_LabelSelector(t *testing.T) {
	clause, args, err := ParseFilter("labels.env=prod")
	require.NoError(t, err)
	assert.Equal(t, "labels->>'env' = ?", clause)
	assert.Equal(t, []interface{}{"prod"}, args)
}

func TestParseFilter_AnnotationSelector(t *testing.T) {
	clause, args, err := ParseFilter("annotations.team=sre")
	require.NoError(t, err)
	assert.Equal(t, "annotations->>'team' = ?", clause)
	assert.Equal(t, []interface{}{"sre"}, args)
}

func TestParseFilter_LabelNotEqual(t *testing.T) {
	clause, args, err := ParseFilter("labels.env!=staging")
	require.NoError(t, err)
	assert.Equal(t, "labels->>'env' != ?", clause)
	assert.Equal(t, []interface{}{"staging"}, args)
}

func TestParseFilter_LabelAndAnnotationAND(t *testing.T) {
	clause, args, err := ParseFilter("labels.env=prod AND annotations.team=sre")
	require.NoError(t, err)
	assert.Equal(t, "labels->>'env' = ? AND annotations->>'team' = ?", clause)
	assert.Equal(t, []interface{}{"prod", "sre"}, args)
}

func TestParseFilter_LabelWithRegularField(t *testing.T) {
	clause, args, err := ParseFilter("name=foo,labels.env=prod")
	require.NoError(t, err)
	assert.Equal(t, "name = ? AND labels->>'env' = ?", clause)
	assert.Equal(t, []interface{}{"foo", "prod"}, args)
}

// --- CEL-style syntax tests ---

func TestParseFilter_CELDoubleEquals(t *testing.T) {
	clause, args, err := ParseFilter("name==foo")
	require.NoError(t, err)
	assert.Equal(t, "name = ?", clause)
	assert.Equal(t, []interface{}{"foo"}, args)
}

func TestParseFilter_CELLogicalAnd(t *testing.T) {
	clause, args, err := ParseFilter("name==foo && cpus>=4")
	require.NoError(t, err)
	assert.Equal(t, "name = ? AND cpus >= ?", clause)
	assert.Equal(t, []interface{}{"foo", "4"}, args)
}

func TestParseFilter_CELLogicalOr(t *testing.T) {
	clause, args, err := ParseFilter("name==foo || name==bar")
	require.NoError(t, err)
	assert.Equal(t, "(name = ?) OR (name = ?)", clause)
	assert.Equal(t, []interface{}{"foo", "bar"}, args)
}

func TestParseFilter_CELMixedWithClassic(t *testing.T) {
	clause, args, err := ParseFilter("name==foo && labels.env=prod")
	require.NoError(t, err)
	assert.Equal(t, "name = ? AND labels->>'env' = ?", clause)
	assert.Equal(t, []interface{}{"foo", "prod"}, args)
}

// --- Edge cases ---

func TestParseFilter_SpacesAroundTerms(t *testing.T) {
	clause, args, err := ParseFilter("  name = foo , kind = bar  ")
	require.NoError(t, err)
	assert.Equal(t, "name = ? AND kind = ?", clause)
	assert.Equal(t, []interface{}{"foo", "bar"}, args)
}

func TestParseFilter_ValueWithHyphens(t *testing.T) {
	clause, args, err := ParseFilter("name=my-machine-01")
	require.NoError(t, err)
	assert.Equal(t, "name = ?", clause)
	assert.Equal(t, []interface{}{"my-machine-01"}, args)
}

func TestParseFilter_EmptyValue(t *testing.T) {
	clause, args, err := ParseFilter("name=")
	require.NoError(t, err)
	assert.Equal(t, "name = ?", clause)
	assert.Equal(t, []interface{}{""}, args)
}

// --- fieldToSQL unit tests ---

func TestFieldToSQL_RegularField(t *testing.T) {
	assert.Equal(t, "name", fieldToSQL("name"))
}

func TestFieldToSQL_LabelField(t *testing.T) {
	assert.Equal(t, "labels->>'env'", fieldToSQL("labels.env"))
}

func TestFieldToSQL_AnnotationField(t *testing.T) {
	assert.Equal(t, "annotations->>'team'", fieldToSQL("annotations.team"))
}

func TestFieldToSQL_NonJSONBDotField(t *testing.T) {
	// A field with a dot that isn't labels. or annotations. stays as-is.
	assert.Equal(t, "spec.cpu", fieldToSQL("spec.cpu"))
}

// --- isValidFieldName unit tests ---

func TestIsValidFieldName_Valid(t *testing.T) {
	assert.True(t, isValidFieldName("name"))
	assert.True(t, isValidFieldName("labels.env"))
	assert.True(t, isValidFieldName("org_id"))
	assert.True(t, isValidFieldName("Name123"))
}

func TestIsValidFieldName_Invalid(t *testing.T) {
	assert.False(t, isValidFieldName(""))
	assert.False(t, isValidFieldName("drop;table"))
	assert.False(t, isValidFieldName("field name"))
	assert.False(t, isValidFieldName(strings.Repeat("x", 65)))
}
