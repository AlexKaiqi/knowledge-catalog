package knowledge

// Domain Schema parsing, instance validation and compatibility. This is the
// protocol interpretation shared by Writer and Reader; the System Repository
// that publishes the Meta Schema lives in system_repository.go.

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"

	"kc/kernel"
)

const (
	// SystemRepositoryID is the deployment-managed, universally readable
	// protocol Repository. Catalog only sees it as another Repository ID; its
	// knowledge meaning is owned at layer ② and wired by the application root.
	SystemRepositoryID kernel.RepositoryID = "kr://kc/system"

	MetaSchemaV1                   ObjectID = "schema/meta/schema-definition/v1"
	CoreResourceDescriptorSchemaV1 ObjectID = "schema/core/resource-descriptor/v1"
	CoreRelationSchemaV1           ObjectID = "schema/core/relation/v1"
)

var supportedSchemaTypes = map[string]struct{}{
	"": {}, "string": {}, "boolean": {}, "number": {}, "integer": {},
	"object": {}, "record": {}, "array": {}, "object_ref": {},
	"object_ref_list": {}, "relation_endpoint_list": {},
}

var supportedSchemaAccess = map[string]struct{}{"text": {}, "filter": {}, "sort": {}}

// SchemaFieldDefinition is one logical Domain Schema field. Access contains
// logical query affordances, never physical provider settings.
type SchemaFieldDefinition struct {
	Path     string   `json:"path"`
	Type     string   `json:"type,omitempty"`
	Required bool     `json:"required,omitempty"`
	Access   []string `json:"access,omitempty"`
	RefTypes []string `json:"refTypes,omitempty"`
}

// SchemaDefinition is the normalized protocol subset understood by Writer and
// Reader. Address matching, required fields and additionalProperties apply to
// every schema/* object; declaring metaSchema explicitly is documentation, not
// an opt-in to being validated.
type SchemaDefinition struct {
	ObjectID             ObjectID                `json:"objectId"`
	MetaSchema           ObjectID                `json:"metaSchema"`
	Entity               string                  `json:"entity"`
	Aspect               string                  `json:"aspect,omitempty"`
	Pattern              string                  `json:"pattern"`
	AdditionalProperties bool                    `json:"additionalProperties"`
	Fields               []SchemaFieldDefinition `json:"fields"`
}

// ParseSchemaDefinition validates one schema/* value against the built-in V1
// Meta Schema contract and returns a deterministic normalized definition.
func ParseSchemaDefinition(objectID ObjectID, value any) (SchemaDefinition, error) {
	if !IsSchemaObject(objectID) {
		return SchemaDefinition{}, kernel.Fail(kernel.ErrSchemaUnsupported,
			"schema object %s must use the schema/* namespace", objectID)
	}
	body, ok := schemaObject(value)
	if !ok {
		return SchemaDefinition{}, kernel.Fail(kernel.ErrSchemaUnsupported,
			"schema %s must be an object", objectID)
	}
	meta, _, err := optionalSchemaString(body, "metaSchema")
	if err != nil {
		return SchemaDefinition{}, schemaUnsupported(objectID, "%v", err)
	}
	if meta == "" {
		meta = string(MetaSchemaV1)
	}
	if meta != string(MetaSchemaV1) {
		return SchemaDefinition{}, schemaUnsupported(objectID,
			"metaSchema %q is unsupported; expected %s", meta, MetaSchemaV1)
	}
	entity, _, err := optionalSchemaString(body, "entity")
	if err != nil || entity == "" {
		return SchemaDefinition{}, schemaUnsupported(objectID, "requires non-empty entity")
	}
	aspect, _, err := optionalSchemaString(body, "aspect")
	if err != nil {
		return SchemaDefinition{}, schemaUnsupported(objectID, "%v", err)
	}
	pattern, _, err := optionalSchemaString(body, "pattern")
	if err != nil {
		return SchemaDefinition{}, schemaUnsupported(objectID, "%v", err)
	}
	pattern = strings.ReplaceAll(strings.ToLower(pattern), " ", "_")
	if pattern == "" {
		pattern = "record"
	}
	if pattern != "record" && pattern != "keyed_collection" {
		return SchemaDefinition{}, schemaUnsupported(objectID,
			"pattern %q is unsupported; expected record or keyed_collection", pattern)
	}
	if pattern == "keyed_collection" && aspect == "" {
		return SchemaDefinition{}, schemaUnsupported(objectID,
			"keyed_collection requires aspect")
	}
	additional := true
	if raw, exists := body["additionalProperties"]; exists {
		value, ok := raw.(bool)
		if !ok {
			return SchemaDefinition{}, schemaUnsupported(objectID,
				"additionalProperties must be boolean")
		}
		additional = value
	}
	fields, err := parseSchemaFields(objectID, body["fields"])
	if err != nil {
		return SchemaDefinition{}, err
	}
	return SchemaDefinition{
		ObjectID: objectID, MetaSchema: ObjectID(meta), Entity: entity, Aspect: aspect,
		Pattern: pattern, AdditionalProperties: additional,
		Fields: fields,
	}, nil
}

func schemaObject(value any) (map[string]any, bool) {
	body, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	if _, hasEntity := body["entity"]; hasEntity {
		return body, true
	}
	// Reader may hand this parser an assembled single-Aspect schema object.
	for _, raw := range body {
		if child, ok := raw.(map[string]any); ok {
			if _, hasEntity := child["entity"]; hasEntity {
				return child, true
			}
		}
	}
	return body, true
}

func optionalSchemaString(body map[string]any, name string) (string, bool, error) {
	raw, exists := body[name]
	if !exists || raw == nil {
		return "", false, nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", true, fmt.Errorf("%s must be string", name)
	}
	return strings.TrimSpace(value), true, nil
}

func parseSchemaFields(objectID ObjectID, raw any) ([]SchemaFieldDefinition, error) {
	if raw == nil {
		return []SchemaFieldDefinition{}, nil
	}
	items, ok := raw.(map[string]any)
	if !ok {
		return nil, schemaUnsupported(objectID, "fields must be an object")
	}
	fields := make([]SchemaFieldDefinition, 0, len(items))
	for name, rawField := range items {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, schemaUnsupported(objectID, "field name must not be empty")
		}
		body, ok := rawField.(map[string]any)
		if !ok {
			return nil, schemaUnsupported(objectID, "field %s must be an object", name)
		}
		fieldType, _, err := optionalSchemaString(body, "type")
		if err != nil {
			return nil, schemaUnsupported(objectID, "field %s: %v", name, err)
		}
		fieldType = strings.ToLower(fieldType)
		if _, supported := supportedSchemaTypes[fieldType]; !supported {
			return nil, schemaUnsupported(objectID, "field %s has unsupported type %q", name, fieldType)
		}
		required := false
		if value, exists := body["required"]; exists {
			var ok bool
			required, ok = value.(bool)
			if !ok {
				return nil, schemaUnsupported(objectID, "field %s required must be boolean", name)
			}
		}
		access, err := schemaStringList(body["access"])
		if err != nil {
			return nil, schemaUnsupported(objectID, "field %s access: %v", name, err)
		}
		for _, hint := range access {
			if _, supported := supportedSchemaAccess[hint]; !supported {
				return nil, schemaUnsupported(objectID,
					"field %s has unsupported access %q; expected text, filter or sort", name, hint)
			}
		}
		refTypes, err := schemaStringList(body["refTypes"])
		if err != nil {
			return nil, schemaUnsupported(objectID, "field %s refTypes: %v", name, err)
		}
		fields = append(fields, SchemaFieldDefinition{
			Path: name, Type: fieldType, Required: required,
			Access: compactSorted(access), RefTypes: compactSorted(refTypes),
		})
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Path < fields[j].Path })
	return fields, nil
}

func schemaStringList(raw any) ([]string, error) {
	if raw == nil {
		return []string{}, nil
	}
	var values []string
	switch items := raw.(type) {
	case []any:
		for _, item := range items {
			value, ok := item.(string)
			if !ok || strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("must contain non-empty strings")
			}
			values = append(values, strings.ToLower(strings.TrimSpace(value)))
		}
	case []string:
		for _, item := range items {
			if strings.TrimSpace(item) == "" {
				return nil, fmt.Errorf("must contain non-empty strings")
			}
			values = append(values, strings.ToLower(strings.TrimSpace(item)))
		}
	case string:
		for _, item := range strings.Fields(items) {
			values = append(values, strings.ToLower(item))
		}
	default:
		return nil, fmt.Errorf("must be a string list")
	}
	return values, nil
}

func compactSorted(values []string) []string {
	sort.Strings(values)
	out := make([]string, 0, len(values))
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

func schemaUnsupported(objectID ObjectID, format string, args ...any) error {
	return kernel.Fail(kernel.ErrSchemaUnsupported, "schema %s: %s", objectID, fmt.Sprintf(format, args...))
}

// ValidateSchemaInstance checks one PUT against its exact resolved Domain
// Schema. Binding declarations have no Snapshot value and are validated when
// hydrated by the serving layer, so Writer calls this only for inline values.
func ValidateSchemaInstance(address Address, value any, schema SchemaDefinition) error {
	switch {
	case schema.Aspect == "" && address.AspectName != "":
		return schemaInstanceInvalid(address, "schema %s describes an entity/relation, not aspect %s", schema.ObjectID, address.AspectName)
	case schema.Aspect != "" && address.AspectName != schema.Aspect:
		return schemaInstanceInvalid(address, "schema %s requires aspect %s", schema.ObjectID, schema.Aspect)
	case schema.Pattern == "keyed_collection" && address.Kind != KindMember:
		return schemaInstanceInvalid(address, "schema %s requires a Member address", schema.ObjectID)
	case schema.Pattern == "record" && schema.Aspect != "" && address.Kind == KindMember:
		return schemaInstanceInvalid(address, "schema %s requires an Aspect/Record address", schema.ObjectID)
	}
	body, ok := value.(map[string]any)
	if !ok {
		return schemaInstanceInvalid(address, "schema %s requires an object value", schema.ObjectID)
	}
	declared := make(map[string]SchemaFieldDefinition, len(schema.Fields))
	for _, field := range schema.Fields {
		declared[field.Path] = field
		raw, exists := body[field.Path]
		if field.Required && (!exists || raw == nil) {
			return schemaInstanceInvalid(address, "required field %s is missing", field.Path)
		}
		if exists && raw != nil && !schemaTypeMatches(field.Type, raw) {
			return schemaInstanceInvalid(address, "field %s must be %s, got %s", field.Path, displaySchemaType(field.Type), valueType(raw))
		}
	}
	if !schema.AdditionalProperties {
		for name := range body {
			if _, ok := declared[name]; !ok {
				return schemaInstanceInvalid(address, "field %s is not declared by schema %s", name, schema.ObjectID)
			}
		}
	}
	return nil
}

// BreakingSchemaChanges returns deterministic reasons why reusing the same
// schema object ID would narrow or change its published contract. A provider
// must publish a new major schema identity for any returned change.
func BreakingSchemaChanges(previous, next SchemaDefinition) []string {
	changes := []string{}
	if previous.MetaSchema != next.MetaSchema {
		changes = append(changes, "metaSchema changed")
	}
	if previous.Entity != next.Entity {
		changes = append(changes, "entity changed")
	}
	if previous.Aspect != next.Aspect {
		changes = append(changes, "aspect changed")
	}
	if previous.Pattern != next.Pattern {
		changes = append(changes, "pattern changed")
	}
	if previous.AdditionalProperties && !next.AdditionalProperties {
		changes = append(changes, "additionalProperties was narrowed")
	}
	oldFields := map[string]SchemaFieldDefinition{}
	newFields := map[string]SchemaFieldDefinition{}
	for _, field := range previous.Fields {
		oldFields[field.Path] = field
	}
	for _, field := range next.Fields {
		newFields[field.Path] = field
		if _, exists := oldFields[field.Path]; !exists && field.Required {
			changes = append(changes, "required field "+field.Path+" was added")
		}
	}
	for name, oldField := range oldFields {
		newField, exists := newFields[name]
		if !exists {
			changes = append(changes, "field "+name+" was removed")
			continue
		}
		if oldField.Type != newField.Type {
			changes = append(changes, "field "+name+" type changed")
		}
		if !oldField.Required && newField.Required {
			changes = append(changes, "field "+name+" became required")
		}
		if !stringSetContains(newField.Access, oldField.Access) {
			changes = append(changes, "field "+name+" access was narrowed")
		}
		if refTypesNarrowed(oldField.RefTypes, newField.RefTypes) {
			changes = append(changes, "field "+name+" refTypes were narrowed")
		}
	}
	sort.Strings(changes)
	return changes
}

func stringSetContains(values, required []string) bool {
	set := map[string]struct{}{}
	for _, value := range values {
		set[value] = struct{}{}
	}
	for _, value := range required {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}

func refTypesNarrowed(previous, next []string) bool {
	if len(next) == 0 {
		return false
	}
	if len(previous) == 0 {
		return true
	}
	return !stringSetContains(next, previous)
}

func schemaInstanceInvalid(address Address, format string, args ...any) error {
	return kernel.Fail(kernel.ErrSchemaInstanceInvalid, "address %s: %s", AddressKey(address), fmt.Sprintf(format, args...))
}

func displaySchemaType(value string) string {
	if value == "" {
		return "any"
	}
	return value
}

func valueType(value any) string {
	if value == nil {
		return "null"
	}
	return reflect.TypeOf(value).String()
}

func schemaTypeMatches(kind string, value any) bool {
	switch kind {
	case "":
		return true
	case "string", "object_ref":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		return isSchemaNumber(value)
	case "integer":
		return isSchemaInteger(value)
	case "object", "record":
		_, ok := value.(map[string]any)
		return ok
	case "array", "object_ref_list", "relation_endpoint_list":
		items, ok := value.([]any)
		if !ok {
			return false
		}
		if kind == "object_ref_list" {
			for _, item := range items {
				if _, ok := item.(string); !ok {
					return false
				}
			}
		}
		if kind == "relation_endpoint_list" {
			for _, item := range items {
				if _, ok := item.(map[string]any); !ok {
					return false
				}
			}
		}
		return true
	default:
		return false
	}
}

func isSchemaNumber(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	default:
		return false
	}
}

func isSchemaInteger(value any) bool {
	switch number := value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case float32:
		return math.Trunc(float64(number)) == float64(number)
	case float64:
		return math.Trunc(number) == number
	default:
		return false
	}
}
