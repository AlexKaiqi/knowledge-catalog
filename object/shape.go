package object

import (
	"sort"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"

	"kc/kernel"
)

// shapeTokenLimit bounds parser work for one request-scoped shape.
const shapeTokenLimit = 4096

// Shape is a GraphQL selection set over one object: top-level names are
// Aspects, the `keys` argument selects Members, nested names select fields.
// It is request scoped; it is never published or stored.
type Shape struct {
	Aspects []ShapeAspect
}

// ShapeAspect selects one Aspect. Keys nil selects every Member (or the whole
// Aspect record); Fields nil returns the complete selected value.
type ShapeAspect struct {
	Name   string
	Keys   []string
	Fields []ShapeField
}

// ShapeField selects a field; Children nil returns its complete value.
type ShapeField struct {
	Name     string
	Children []ShapeField
}

// ShapeSelection is one maintenance-level read compiled from a Shape. Each
// selection is planned and read on its own, so unselected units are never read.
type ShapeSelection struct {
	Aspect string
	Key    string
	Keyed  bool
	Fields []ShapeField
}

// Locator is the JSON Pointer of the selected Aspect or Member.
func (s ShapeSelection) Locator() string {
	tokens := []string{s.Aspect}
	if s.Keyed {
		tokens = append(tokens, s.Key)
	}
	return FormatPointer(tokens)
}

// Path is the result path of the selection inside the shaped value.
func (s ShapeSelection) Path() []string {
	if s.Keyed {
		return []string{s.Aspect, s.Key}
	}
	return []string{s.Aspect}
}

// ShapeError locates a selected position whose value is null in the result.
// A legitimate null has no error entry.
type ShapeError struct {
	Path    []string         `json:"path"`
	Code    kernel.ErrorCode `json:"code"`
	Message string           `json:"message"`
}

// ShapedValue is the GraphQL-style result of a shaped read: data has the
// shape's structure, errors explain selected nulls, and each read unit keeps
// its complete maintenance digest for conditional writes.
type ShapedValue struct {
	Reference    string                `json:"reference"`
	Shape        string                `json:"shape"`
	ObjectID     ObjectID              `json:"objectId"`
	Data         map[string]any        `json:"data"`
	Errors       []ShapeError          `json:"errors,omitempty"`
	Declarations []UnitDeclaration     `json:"declarations,omitempty"`
	Sources      []SourceBasis         `json:"sources,omitempty"`
	Withheld     []kernel.RepositoryID `json:"withheld,omitempty"`
	Observations []UnitObservation     `json:"observations,omitempty"`
	Failures     []UnitReadFailure     `json:"failures,omitempty"`
}

// ParseShape accepts one anonymous query selection set. Everything beyond
// projection (aliases, fragments, directives, variables, operation names,
// arguments other than Aspect `keys`) is rejected rather than ignored.
func ParseShape(text string) (Shape, error) {
	doc, err := parser.ParseQueryWithTokenLimit(&ast.Source{Name: "shape", Input: text}, shapeTokenLimit)
	if err != nil {
		return Shape{}, shapeInvalid("shape is not a GraphQL selection set: %s", err.Error())
	}
	if len(doc.Fragments) > 0 || len(doc.Operations) != 1 {
		return Shape{}, shapeInvalid("shape must be exactly one selection set without fragments")
	}
	op := doc.Operations[0]
	if op.Operation != ast.Query || op.Name != "" || len(op.VariableDefinitions) > 0 || len(op.Directives) > 0 {
		return Shape{}, shapeInvalid("shape must be an anonymous selection set without variables or directives")
	}
	fields, err := shapeFields(op.SelectionSet, true)
	if err != nil {
		return Shape{}, err
	}
	shape := Shape{}
	for _, field := range fields {
		shape.Aspects = append(shape.Aspects, ShapeAspect{Name: field.name, Keys: field.keys, Fields: field.children})
	}
	return shape, nil
}

type parsedShapeField struct {
	name     string
	keys     []string
	children []ShapeField
}

func shapeFields(set ast.SelectionSet, top bool) ([]parsedShapeField, error) {
	if len(set) == 0 {
		return nil, shapeInvalid("shape selection set is empty")
	}
	out := make([]parsedShapeField, 0, len(set))
	seen := map[string]bool{}
	for _, selection := range set {
		field, ok := selection.(*ast.Field)
		if !ok {
			return nil, shapeInvalid("shape does not support fragments")
		}
		if field.Alias != "" && field.Alias != field.Name {
			return nil, shapeInvalid("shape does not support aliases (%s)", field.Alias)
		}
		if len(field.Directives) > 0 {
			return nil, shapeInvalid("shape does not support directives on %s", field.Name)
		}
		if seen[field.Name] {
			return nil, shapeInvalid("shape selects %s twice", field.Name)
		}
		seen[field.Name] = true
		parsed := parsedShapeField{name: field.Name}
		keys, err := shapeKeys(field, top)
		if err != nil {
			return nil, err
		}
		parsed.keys = keys
		if len(field.SelectionSet) > 0 {
			children, err := shapeFields(field.SelectionSet, false)
			if err != nil {
				return nil, err
			}
			for _, child := range children {
				parsed.children = append(parsed.children, ShapeField{Name: child.name, Children: child.children})
			}
		}
		out = append(out, parsed)
	}
	return out, nil
}

func shapeKeys(field *ast.Field, top bool) ([]string, error) {
	if len(field.Arguments) == 0 {
		return nil, nil
	}
	if !top || len(field.Arguments) != 1 || field.Arguments[0].Name != "keys" {
		return nil, shapeInvalid("shape accepts only the keys argument on an Aspect (%s)", field.Name)
	}
	value := field.Arguments[0].Value
	if value == nil || value.Kind != ast.ListValue || len(value.Children) == 0 {
		return nil, shapeInvalid("keys on %s must be a non-empty list of strings", field.Name)
	}
	keys := make([]string, 0, len(value.Children))
	seen := map[string]bool{}
	for _, child := range value.Children {
		if child.Value == nil || child.Value.Kind != ast.StringValue || child.Value.Raw == "" {
			return nil, shapeInvalid("keys on %s must be non-empty strings", field.Name)
		}
		if seen[child.Value.Raw] {
			return nil, shapeInvalid("keys on %s repeat %s", field.Name, child.Value.Raw)
		}
		seen[child.Value.Raw] = true
		keys = append(keys, child.Value.Raw)
	}
	return keys, nil
}

// Selections compiles the shape into maintenance-level reads.
func (s Shape) Selections() []ShapeSelection {
	out := make([]ShapeSelection, 0, len(s.Aspects))
	for _, aspect := range s.Aspects {
		if aspect.Keys == nil {
			out = append(out, ShapeSelection{Aspect: aspect.Name, Fields: aspect.Fields})
			continue
		}
		for _, key := range aspect.Keys {
			out = append(out, ShapeSelection{Aspect: aspect.Name, Key: key, Keyed: true, Fields: aspect.Fields})
		}
	}
	return out
}

// ProjectShapeFields trims one freshly read unit value to the selected fields.
// Selected but absent fields become null with a located error.
func ProjectShapeFields(value any, fields []ShapeField, path []string) (any, []ShapeError) {
	if fields == nil {
		return value, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		if value == nil {
			return nil, nil
		}
		return nil, []ShapeError{{Path: clonePath(path), Code: kernel.ErrUsageInvalid, Message: "selected fields on a value that is not an object"}}
	}
	out := make(map[string]any, len(fields))
	var errs []ShapeError
	for _, field := range fields {
		at := append(clonePath(path), field.Name)
		raw, exists := object[field.Name]
		if !exists {
			out[field.Name] = nil
			errs = append(errs, ShapeError{Path: at, Code: kernel.ErrObjectRefUnresolved, Message: "selected field is absent"})
			continue
		}
		projected, nested := ProjectShapeFields(raw, field.Children, at)
		out[field.Name] = projected
		errs = append(errs, nested...)
	}
	return out, errs
}

// ShapeSchemaMismatches names selected top-level fields that contradict an
// adopted Schema: names a closed Schema does not declare, and sub-selections
// under fields the Schema declares as scalars. Schema fields are top-level, so
// deeper levels are checked against the value instead.
func ShapeSchemaMismatches(fields []ShapeField, schema SchemaDefinition) []string {
	declared := make(map[string]string, len(schema.Fields))
	for _, field := range schema.Fields {
		declared[field.Path] = field.Type
	}
	var mismatches []string
	for _, field := range fields {
		fieldType, ok := declared[field.Name]
		switch {
		case !ok && !schema.AdditionalProperties:
			mismatches = append(mismatches, field.Name+" is not declared")
		case ok && field.Children != nil && scalarSchemaType(fieldType):
			mismatches = append(mismatches, field.Name+" is "+fieldType+" and has no sub-fields")
		}
	}
	sort.Strings(mismatches)
	return mismatches
}

func scalarSchemaType(fieldType string) bool {
	switch fieldType {
	case "string", "boolean", "number", "integer", "date", "datetime", "timestamp", "object_ref":
		return true
	}
	return false
}

// SetShapePath places value at path inside data, creating parents.
func SetShapePath(data map[string]any, path []string, value any) {
	current := data
	for _, token := range path[:len(path)-1] {
		next, ok := current[token].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[token] = next
		}
		current = next
	}
	current[path[len(path)-1]] = value
}

func clonePath(path []string) []string {
	return append([]string(nil), path...)
}

func shapeInvalid(format string, args ...any) error {
	return kernel.Fail(kernel.ErrUsageInvalid, format, args...)
}
