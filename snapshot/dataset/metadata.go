package dataset

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

var (
	knowledgeSetType   = reflect.TypeOf(KnowledgeSet{})
	metadataRecipeType = reflect.TypeOf(KnowledgeSetRecipe{})
	rawJSONType        = reflect.TypeOf(json.RawMessage{})
)

// ValidateMetadataJSON checks only Dataset and portable-recipe metadata selected
// by the target Go type. Transports can retain their own JSON number and byte
// budgets; opaque interface values and RawMessage payloads are not interpreted.
func ValidateMetadataJSON(body []byte, target any) error {
	return validateMetadataType(body, reflect.TypeOf(target))
}

func containsMetadataType(typ reflect.Type, seen map[reflect.Type]bool) bool {
	if typ == nil || typ == rawJSONType || seen[typ] {
		return false
	}
	if typ == knowledgeSetType || typ == metadataRecipeType {
		return true
	}
	seen[typ] = true
	defer delete(seen, typ)
	switch typ.Kind() {
	case reflect.Pointer, reflect.Array, reflect.Slice, reflect.Map:
		return containsMetadataType(typ.Elem(), seen)
	case reflect.Struct:
		for _, field := range metadataJSONFields(typ) {
			if containsMetadataType(field.typ, seen) {
				return true
			}
		}
	}
	return false
}

func validateMetadataType(body []byte, typ reflect.Type) error {
	if !containsMetadataType(typ, map[reflect.Type]bool{}) || bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return nil
	}
	if typ.Kind() == reflect.Pointer {
		return validateMetadataType(body, typ.Elem())
	}
	if typ == knowledgeSetType || typ == metadataRecipeType {
		return validateMetadataObject(body)
	}
	switch typ.Kind() {
	case reflect.Array, reflect.Slice:
		var values []json.RawMessage
		if err := json.Unmarshal(body, &values); err != nil {
			return err
		}
		for i, value := range values {
			if typ.Kind() == reflect.Array && i >= typ.Len() {
				break // encoding/json ignores excess elements of a fixed array.
			}
			if err := validateMetadataType(value, typ.Elem()); err != nil {
				return err
			}
		}
	case reflect.Map, reflect.Struct:
		var values map[string]json.RawMessage
		if err := json.Unmarshal(body, &values); err != nil {
			return err
		}
		fields := metadataJSONFields(typ)
		for name, value := range values {
			var valueType reflect.Type
			if typ.Kind() == reflect.Struct {
				valueType = metadataJSONFieldType(fields, name)
			} else {
				valueType = typ.Elem()
			}
			if err := validateMetadataType(value, valueType); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateMetadataObject(body []byte) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	start, err := dec.Token()
	if err != nil || start != json.Delim('{') {
		return err
	}
	found := false
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return err
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return err
		}
		name, ok := key.(string)
		if !ok || !strings.EqualFold(name, "metadata") {
			continue
		}
		if found {
			return fmt.Errorf("metadata field must occur only once")
		}
		found = true
		var metadata map[string]string
		if err := decodeMetadataJSON(value, &metadata); err != nil {
			return err
		}
	}
	return nil
}

type metadataJSONField struct {
	name      string
	typ       reflect.Type
	index     []int
	tagged    bool
	ambiguous bool
}

// Field selection follows JSON's promotion rules: a shallower field wins;
// at equal depth an explicit tag wins, otherwise a collision is ignored.
func metadataJSONFields(typ reflect.Type) []metadataJSONField {
	if typ.Kind() != reflect.Struct {
		return nil
	}
	byName := map[string]metadataJSONField{}
	collectMetadataFields(typ, nil, byName, map[reflect.Type]bool{})
	fields := make([]metadataJSONField, 0, len(byName))
	for _, field := range byName {
		if !field.ambiguous {
			fields = append(fields, field)
		}
	}
	slices.SortFunc(fields, func(a, b metadataJSONField) int { return slices.Compare(a.index, b.index) })
	return fields
}

func collectMetadataFields(typ reflect.Type, index []int, fields map[string]metadataJSONField, seen map[reflect.Type]bool) {
	if seen[typ] {
		return
	}
	seen[typ] = true
	defer delete(seen, typ)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		base := field.Type
		if base.Kind() == reflect.Pointer {
			base = base.Elem()
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "-" || (field.PkgPath != "" && (!field.Anonymous || base.Kind() != reflect.Struct)) {
			continue
		}
		path := append(slices.Clone(index), i)
		if field.Anonymous && name == "" && base.Kind() == reflect.Struct {
			collectMetadataFields(base, path, fields, seen)
			continue
		}
		tagged := name != ""
		if name == "" {
			name = field.Name
		}
		candidate := metadataJSONField{name: name, typ: field.Type, index: path, tagged: tagged}
		previous, exists := fields[name]
		if !exists || len(path) < len(previous.index) || (len(path) == len(previous.index) && tagged && !previous.tagged) {
			fields[name] = candidate
		} else if len(path) == len(previous.index) && tagged == previous.tagged {
			previous.ambiguous = true
			fields[name] = previous
		}
	}
}

func metadataJSONFieldType(fields []metadataJSONField, name string) reflect.Type {
	for _, field := range fields {
		if field.name == name {
			return field.typ
		}
	}
	for _, field := range fields {
		if strings.EqualFold(field.name, name) {
			return field.typ
		}
	}
	return nil
}
