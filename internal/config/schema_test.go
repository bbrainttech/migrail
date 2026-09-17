package config

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/bbrainttech/migrail/internal/golden"
)

func TestPublishedSchemaMatchesEmbedded(t *testing.T) {
	t.Parallel()

	golden.Assert(t, "../../schemas/config.schema.json", Schema)
}

func TestSchemaMatchesConfigStruct(t *testing.T) {
	t.Parallel()

	var schema map[string]any
	if err := json.Unmarshal([]byte(Schema), &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}

	fromSchema := []string{}
	collectSchemaKeys("", schema, &fromSchema)

	fromStruct := []string{}
	collectStructKeys("", reflect.TypeFor[Config](), &fromStruct)

	slices.Sort(fromSchema)
	slices.Sort(fromStruct)

	if !slices.Equal(fromSchema, fromStruct) {
		t.Errorf("schema keys and Config fields differ:\n schema %v\n struct %v", fromSchema, fromStruct)
	}
}

func collectSchemaKeys(prefix string, node map[string]any, keys *[]string) {
	if items, ok := node["items"].(map[string]any); ok {
		collectSchemaKeys(prefix, items, keys)
	}

	properties, ok := node["properties"].(map[string]any)
	if !ok {
		return
	}

	for name, child := range properties {
		key := strings.TrimPrefix(prefix+"."+name, ".")
		*keys = append(*keys, key)

		if childNode, ok := child.(map[string]any); ok {
			collectSchemaKeys(key, childNode, keys)
		}
	}
}

func collectStructKeys(prefix string, typ reflect.Type, keys *[]string) {
	for typ.Kind() == reflect.Slice || typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	if typ.Kind() != reflect.Struct {
		return
	}

	for field := range typ.Fields() {
		name := strings.Split(field.Tag.Get("yaml"), ",")[0]
		if name == "" || name == "-" {
			continue
		}

		key := strings.TrimPrefix(prefix+"."+name, ".")
		*keys = append(*keys, key)

		if field.Type.Kind() != reflect.Map {
			collectStructKeys(key, field.Type, keys)
		}
	}
}
