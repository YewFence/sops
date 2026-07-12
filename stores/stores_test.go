package stores

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMetadataTypesHaveTOMLTags(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeOf(metadata{}),
		reflect.TypeOf(keygroup{}),
		reflect.TypeOf(pgpkey{}),
		reflect.TypeOf(kmskey{}),
		reflect.TypeOf(gcpkmskey{}),
		reflect.TypeOf(vaultkey{}),
		reflect.TypeOf(azkvkey{}),
		reflect.TypeOf(agekey{}),
		reflect.TypeOf(hckmskey{}),
	}
	for _, typ := range types {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			mapstructureName := strings.Split(field.Tag.Get("mapstructure"), ",")[0]
			tomlName := strings.Split(field.Tag.Get("toml"), ",")[0]
			assert.NotEmpty(t, tomlName, "%s.%s is missing a TOML tag", typ.Name(), field.Name)
			assert.Equal(t, mapstructureName, tomlName, "%s.%s has mismatched serialization tags", typ.Name(), field.Name)
		}
	}
}

func TestValToString(t *testing.T) {
	assert.Equal(t, "1", ValToString(1))
	assert.Equal(t, "1.0", ValToString(1.0))
	assert.Equal(t, "1.1", ValToString(1.10))
	assert.Equal(t, "1.23", ValToString(1.23))
	assert.Equal(t, "1.2345678901234567", ValToString(1.234567890123456789))
	assert.Equal(t, "200000.0", ValToString(2e5))
	assert.Equal(t, "-2E+10", ValToString(-2e10))
	assert.Equal(t, "2E-10", ValToString(2e-10))
	assert.Equal(t, "1.2345E+100", ValToString(1.2345e100))
	assert.Equal(t, "1.2345E-100", ValToString(1.2345e-100))
	assert.Equal(t, "true", ValToString(true))
	assert.Equal(t, "false", ValToString(false))
	ts, _ := time.Parse(time.RFC3339, "2025-01-02T03:04:05Z")
	assert.Equal(t, "2025-01-02T03:04:05Z", ValToString(ts))
	assert.Equal(t, "a string", ValToString("a string"))
}
