package discovery

import (
	"fmt"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"sort"
	"strings"
)

// FieldValue is a normalized logical scalar and the declaration that permits it.
// Physical providers choose their own typed storage from this shared meaning.
type FieldValue struct {
	Field AccessField
	Value string
	// Member is the interpreting declaration's identity, never query input.
	Member string
	form   string
}

// Key identifies one member's physical realization of a Schema field.
func (v FieldValue) Key() string { return memberFieldKey(v.Field.FieldRef, v.Member) }

// FieldSet records applicability separately from value presence. An unobserved
// Binding does not promise MISSING; an observed null can establish applicability.
type FieldSet struct {
	Kind           objectmodel.AddressKind
	EligibleFields []string
	Values         []FieldValue
	Text           string
	Semantic       []SemanticField
	applicable     map[string]bool
}

// SemanticField is one explicit vector member's input text and registered model.
type SemanticField struct {
	Member string `json:"member"`
	Model  string `json:"model"`
	Text   string `json:"text"`
}

// ExtractFields interprets the actual adopted Schema of each maintained unit.
func ExtractFields(repo objectmodel.Repository, value objectmodel.ObjectValue, observations []objectmodel.UnitObservation, spec AccessSpec) (FieldSet, error) {
	doc := FieldSet{Kind: fieldObjectKind(value), applicable: map[string]bool{}}
	eligible := map[string]struct{}{}
	seenCells := map[string]struct{}{}
	textParts := []string{}
	observed := make(map[string]objectmodel.UnitObservation, len(observations))
	for _, observation := range observations {
		observed[objectmodel.AddressKey(observation.Address)] = observation
	}

	for _, unit := range fieldUnits(repo, value) {
		if !spec.hasDeclarationNamespace(unit.SchemaRef) {
			continue
		}
		unitObserved := false
		if isBindingUnit(unit) {
			observation, ok := observed[objectmodel.AddressKey(unit.Address)]
			if !ok {
				continue
			}
			// Hydrated declarations already carry the observed declaration
			// version (unit plus ResourceDescriptor digest).
			if observation.DeclarationCommit == "" || observation.DeclarationCommit != observationDeclarationCommit(value, unit) || observation.DeclarationDigest != unit.DeclarationDigest {
				return FieldSet{}, kernel.Fail(kernel.ErrPreconditionFailed,
					"observation basis does not match Binding declaration at %s", objectmodel.AddressKey(unit.Address))
			}
			unitObserved = true
		}
		unitValue, ok := fieldUnitValue(value.Value, unit.Address)
		if unitObserved {
			// An observed JSON null is a complete unit observation. It proves
			// applicability even though it contributes no value cell.
			ok = true
		}
		if !ok {
			continue
		}
		fields, err := spec.FieldsForDeclaration(unit.SchemaRef)
		if err != nil {
			return FieldSet{}, err
		}
		for _, field := range fields {
			if !fieldApplies(unit, field) {
				continue
			}
			key := field.FieldRef.Key()
			form := fieldSchemaForm(unit.SchemaRef, field.Schema, observationDeclarationCommit(value, unit))
			doc.applicable[declaredFieldKey(form, field.Path)] = true
			if field.Has(reader.HintFilter) {
				eligible[key] = struct{}{}
			}
			raw, exists := lookupPath(unitValue, field.Path)
			if !exists || raw == nil {
				continue
			}
			for _, item := range scalarValues(raw) {
				normalized, valid := NormalizeScalarValue(field.Type, item)
				var err error
				if !valid {
					err = fmt.Errorf("value %v is not a valid %s scalar", item, field.Type)
				}
				cell := FieldValue{Field: field, Value: normalized, form: form}
				if err != nil {
					return FieldSet{}, kernel.Fail(kernel.ErrUsageInvalid, "projection %s unit %s field %s: %v", value.Address.ObjectID, objectmodel.AddressKey(unit.Address), key, err)
				}
				identity := cell.Field.FieldRef.Key() + "\x00" + form + "\x00" + cell.Value + "\x00" + fmt.Sprint(cell.Field.Access)
				if _, duplicate := seenCells[identity]; duplicate {
					continue
				}
				seenCells[identity] = struct{}{}
				doc.Values = append(doc.Values, cell)
				if cell.Field.Has(reader.HintText) {
					textParts = append(textParts, cell.Value)
				}
			}
		}
	}

	for key := range eligible {
		doc.EligibleFields = append(doc.EligibleFields, key)
	}
	sort.Strings(doc.EligibleFields)
	sort.Slice(doc.Values, func(i, j int) bool {
		a, b := doc.Values[i], doc.Values[j]
		if a.Field.FieldRef.Key() != b.Field.FieldRef.Key() {
			return a.Field.FieldRef.Key() < b.Field.FieldRef.Key()
		}
		if a.Value != b.Value {
			return a.Value < b.Value
		}
		return fmt.Sprint(a.Field.Access) < fmt.Sprint(b.Field.Access)
	})
	doc.Text = strings.Join(textParts, " ")
	if len(spec.builds) > 0 {
		return constructDeclaredFields(doc, spec)
	}
	return doc, nil
}

// A frozen object's structural Schema may be only a READ dependency. It does
// not enter retrieval unless the selected namespace or declarations name it.
// Selected Schema releases and selected declaration inputs stay strict.
func (s AccessSpec) hasDeclarationNamespace(raw string) bool {
	if s.schemaForms == nil || raw == "" {
		return true
	}
	ref, valid := objectmodel.ParseSchemaRef(raw)
	if !valid {
		return true // FieldsForDeclaration reports the malformed reference.
	}
	_, current := s.schemaForms[string(ref.Object)]
	_, adopted := s.schemaForms[schemaFormKey(ref)]
	return current || adopted || s.declarationSchemas[ref.Object]
}

func fieldSchemaForm(raw string, schema objectmodel.ObjectID, commit kernel.CommitID) string {
	ref, valid := objectmodel.ParseSchemaRef(raw)
	if !valid || ref.Commit == commit {
		return string(schema)
	}
	return schemaFormKey(ref)
}

func observationDeclarationCommit(value objectmodel.ObjectValue, unit objectmodel.UnitDeclaration) kernel.CommitID {
	if len(value.Sources) == 0 {
		return value.Commit
	}
	for _, source := range value.Sources {
		for _, aspect := range source.Aspects {
			if aspect == unit.Address.AspectName {
				return source.Commit
			}
		}
	}
	return ""
}

// A binding unit contributes only through an observation; stream bindings
// are never observed by ordinary READ, so they never contribute.
func isBindingUnit(unit objectmodel.UnitDeclaration) bool {
	return unit.ValueSource.IsBinding()
}

func fieldObjectKind(value objectmodel.ObjectValue) objectmodel.AddressKind {
	if value.Address.Kind == objectmodel.KindRelation {
		return objectmodel.KindRelation
	}
	for _, declaration := range value.Declarations {
		if declaration.Address.Kind == objectmodel.KindRelation {
			return objectmodel.KindRelation
		}
	}
	return objectmodel.KindEntity
}

func fieldUnits(repo objectmodel.Repository, value objectmodel.ObjectValue) []objectmodel.UnitDeclaration {
	if len(value.Declarations) > 0 {
		return value.Declarations
	}
	addresses := value.Units
	if len(addresses) == 0 {
		addresses = []objectmodel.Address{value.Address}
	}
	out := make([]objectmodel.UnitDeclaration, 0, len(addresses))
	for _, address := range addresses {
		declaration := objectmodel.UnitDeclaration{Address: address}
		if repo != nil {
			if resolved, err := repo.ResolveAddress(address, value.Commit); err == nil {
				declaration.SchemaRef = resolved.SchemaRef
				declaration.ValueSource = resolved.ValueSource
			}
		}
		out = append(out, declaration)
	}
	return out
}

func fieldUnitValue(assembled any, address objectmodel.Address) (any, bool) {
	if objectmodel.IsEntityBlob(address) || address.Kind == objectmodel.KindRelation {
		return assembled, assembled != nil
	}
	root, ok := assembled.(map[string]any)
	if !ok {
		return nil, false
	}
	aspect, ok := root[address.AspectName]
	if !ok {
		return nil, false
	}
	if address.Kind != objectmodel.KindMember {
		return aspect, true
	}
	members, ok := aspect.(map[string]any)
	if !ok {
		return nil, false
	}
	member, ok := members[address.MemberKey]
	return member, ok
}

func fieldApplies(unit objectmodel.UnitDeclaration, field AccessField) bool {
	if unit.SchemaRef != "" {
		parsed, ok := objectmodel.ParseSchemaRef(unit.SchemaRef)
		return ok && parsed.Object == field.Schema
	}
	// A unit without schema_ref is intentionally structurally typed. Entity is
	// descriptive schema metadata, not an identity discriminator; restricting by
	// object-id spelling would silently make otherwise valid untyped objects
	// unsearchable. Aspect still defines the unit-relative path namespace.
	if field.Aspect != "" {
		return field.Aspect == unit.Address.AspectName
	}
	return unit.Address.AspectName == ""
}

func scalarValues(value any) []any {
	switch list := value.(type) {
	case []any:
		out := []any{}
		for _, item := range list {
			out = append(out, scalarValues(item)...)
		}
		return out
	default:
		return []any{value}
	}
}

func lookupPath(value any, path string) (any, bool) {
	if path == "" {
		return value, value != nil
	}
	cur := value
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			continue
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = obj[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}
