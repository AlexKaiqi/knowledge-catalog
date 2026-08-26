package index

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/retrieval"
)

// compileProjectionDocument is the knowledge-to-projection boundary. It
// interprets schema paths relative to each independently maintained unit, then
// emits one complete object document. Providers never need Aspect/Member rules.
func compileProjectionDocument(repo knowledge.Repository, value knowledge.KnowledgeValue, spec retrieval.AccessSpec) (CompiledDoc, error) {
	doc := CompiledDoc{ObjectID: value.Address.ObjectID, Kind: projectionObjectKind(value)}
	eligible := map[string]struct{}{}
	seenCells := map[string]struct{}{}
	textParts := []string{}

	for _, unit := range projectionUnits(repo, value) {
		unitValue, ok := projectionUnitValue(value.Value, unit.Address)
		if !ok {
			continue
		}
		for _, field := range spec.Fields {
			if !projectionFieldApplies(unit, field) {
				continue
			}
			key := field.FieldRef.Key()
			eligible[key] = struct{}{}
			raw, exists := lookupPath(unitValue, field.Path)
			if !exists || raw == nil {
				continue
			}
			for _, item := range projectionScalarValues(raw) {
				cell, err := projectionCell(field, item)
				if err != nil {
					return CompiledDoc{}, kernel.Fail(kernel.ErrUsageInvalid, "projection %s unit %s field %s: %v", value.Address.ObjectID, knowledge.AddressKey(unit.Address), key, err)
				}
				identity := cell.Field + "\x00" + cell.Value
				if _, duplicate := seenCells[identity]; duplicate {
					continue
				}
				seenCells[identity] = struct{}{}
				doc.Cells = append(doc.Cells, cell)
				doc.Fields = append(doc.Fields, [2]string{cell.Field, cell.Value})
				if cell.TextValue != "" {
					textParts = append(textParts, cell.TextValue)
				}
			}
		}
	}

	for key := range eligible {
		doc.EligibleFields = append(doc.EligibleFields, key)
	}
	sort.Strings(doc.EligibleFields)
	sort.Slice(doc.Cells, func(i, j int) bool {
		if doc.Cells[i].Field != doc.Cells[j].Field {
			return doc.Cells[i].Field < doc.Cells[j].Field
		}
		return doc.Cells[i].Value < doc.Cells[j].Value
	})
	sort.Slice(doc.Fields, func(i, j int) bool {
		if doc.Fields[i][0] != doc.Fields[j][0] {
			return doc.Fields[i][0] < doc.Fields[j][0]
		}
		return doc.Fields[i][1] < doc.Fields[j][1]
	})
	doc.Text = strings.Join(textParts, " ")

	if doc.Kind == knowledge.KindRelation {
		address := knowledge.Address{Kind: knowledge.KindRelation, ObjectID: value.Address.ObjectID}
		relation, err := knowledge.DecodeRelation(address, value.Value)
		if err != nil {
			return CompiledDoc{}, err
		}
		doc.Relation = &ProjectionRelation{Type: relation.RelationType, Direction: relation.Direction}
		for _, endpoint := range relation.Endpoints {
			doc.Relation.Endpoints = append(doc.Relation.Endpoints, ProjectionRelationEndpoint{Role: endpoint.Role, ObjectRef: endpoint.ObjectRef})
		}
	}
	doc.ObjectDigest = kernel.CanonicalDigest(map[string]any{
		"objectId": doc.ObjectID, "kind": doc.Kind, "eligibleFields": doc.EligibleFields,
		"cells": doc.Cells, "relation": doc.Relation,
	})
	return doc, nil
}

func projectionObjectKind(value knowledge.KnowledgeValue) knowledge.AddressKind {
	if value.Address.Kind == knowledge.KindRelation {
		return knowledge.KindRelation
	}
	for _, declaration := range value.Declarations {
		if declaration.Address.Kind == knowledge.KindRelation {
			return knowledge.KindRelation
		}
	}
	return knowledge.KindEntity
}

func projectionUnits(repo knowledge.Repository, value knowledge.KnowledgeValue) []knowledge.UnitDeclaration {
	if len(value.Declarations) > 0 {
		return value.Declarations
	}
	addresses := value.Units
	if len(addresses) == 0 {
		addresses = []knowledge.Address{value.Address}
	}
	out := make([]knowledge.UnitDeclaration, 0, len(addresses))
	for _, address := range addresses {
		declaration := knowledge.UnitDeclaration{Address: address}
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

func projectionUnitValue(assembled any, address knowledge.Address) (any, bool) {
	if knowledge.IsEntityBlob(address) || address.Kind == knowledge.KindRelation {
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
	if address.Kind != knowledge.KindMember {
		return aspect, true
	}
	members, ok := aspect.(map[string]any)
	if !ok {
		return nil, false
	}
	member, ok := members[address.MemberKey]
	return member, ok
}

func projectionFieldApplies(unit knowledge.UnitDeclaration, field retrieval.AccessField) bool {
	if unit.SchemaRef != "" {
		parsed, ok := knowledge.ParseSchemaRef(unit.SchemaRef)
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

func projectionScalarValues(value any) []any {
	switch list := value.(type) {
	case []any:
		out := []any{}
		for _, item := range list {
			out = append(out, projectionScalarValues(item)...)
		}
		return out
	default:
		return []any{value}
	}
}

func projectionCell(field retrieval.AccessField, value any) (ProjectionCell, error) {
	normalized, ok := retrieval.NormalizeScalarValue(field.Type, value)
	if !ok {
		return ProjectionCell{}, fmt.Errorf("value %v is not a valid %s scalar", value, field.Type)
	}
	cell := ProjectionCell{Field: field.FieldRef.Key(), Value: normalized}
	if field.Has(reader.HintText) {
		cell.TextValue = normalized
	}
	typeName := strings.ToLower(strings.TrimSpace(field.Type))
	switch typeName {
	case "", "string":
		cell.StringValue = stringPointer(normalized)
	case "bool", "boolean":
		parsed, _ := strconv.ParseBool(normalized)
		cell.BooleanValue = &parsed
	case "int", "integer", "long":
		parsed, _ := strconv.ParseInt(normalized, 10, 64)
		cell.LongValue = &parsed
	case "number", "float", "double":
		parsed, _ := strconv.ParseFloat(normalized, 64)
		cell.DoubleValue = &parsed
	case "date", "datetime", "timestamp":
		cell.DateValue = normalized
	default:
		return ProjectionCell{}, fmt.Errorf("unsupported scalar type %q", field.Type)
	}
	return cell, nil
}

func stringPointer(value string) *string { return &value }

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

func fieldValue(assembled any, aspect, path string) (any, bool) {
	if aspect != "" {
		if obj, ok := assembled.(map[string]any); ok {
			if inner, ok := obj[aspect]; ok {
				if v, ok := lookupPath(inner, path); ok {
					return v, true
				}
			}
		}
	}
	return lookupPath(assembled, path)
}

func scalarString(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	b, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(b)
}

// boundSpec is the AccessSpec that applies to one object: recipe objects contribute
// nothing; schema_ref selects which declared schemas; otherwise the full spec.
func boundSpec(repo knowledge.Repository, value knowledge.KnowledgeValue, spec retrieval.AccessSpec) retrieval.AccessSpec {
	if knowledge.IsSchemaObject(value.Address.ObjectID) {
		return retrieval.AccessSpec{AccessDigest: spec.AccessDigest}
	}
	return spec.Bind(objectSchemaRefs(repo, value))
}

func objectSchemaRefs(repo knowledge.Repository, value knowledge.KnowledgeValue) []string {
	if repo == nil {
		return nil
	}
	if len(value.Units) == 0 {
		res, err := repo.Resolve(value.Address.ObjectID, value.Commit)
		if err != nil || res.SchemaRef == "" {
			return nil
		}
		return []string{res.SchemaRef}
	}
	var refs []string
	seen := map[string]struct{}{}
	for _, addr := range value.Units {
		res, err := repo.ResolveAddress(addr, value.Commit)
		if err != nil || res.SchemaRef == "" {
			continue
		}
		if _, ok := seen[res.SchemaRef]; ok {
			continue
		}
		seen[res.SchemaRef] = struct{}{}
		refs = append(refs, res.SchemaRef)
	}
	return refs
}

func documentText(value knowledge.KnowledgeValue, spec retrieval.AccessSpec) string {
	assembled := value.Value
	var parts []string
	for _, field := range spec.Fields {
		if !field.Has(reader.HintText) {
			continue
		}
		if v, ok := fieldValue(assembled, field.Aspect, field.Path); ok {
			values := []any{v}
			if list, ok := v.([]any); ok {
				values = list
			}
			for _, item := range values {
				if s := scalarString(item); s != "" {
					parts = append(parts, s)
				}
			}
		}
	}
	return strings.Join(parts, " ")
}

func indexedFields(value knowledge.KnowledgeValue, spec retrieval.AccessSpec) [][2]string {
	assembled := value.Value
	var out [][2]string
	seen := map[string]struct{}{}
	for _, field := range spec.Fields {
		if !field.Has(reader.HintFilter) && !field.Has(reader.HintSort) && !field.Has(reader.HintText) {
			continue
		}
		v, ok := fieldValue(assembled, field.Aspect, field.Path)
		if !ok {
			continue
		}
		key := field.FieldRef.Key()
		values := []any{v}
		if list, ok := v.([]any); ok {
			values = list
		}
		for _, item := range values {
			s, valid := retrieval.NormalizeScalarValue(field.Type, item)
			if !valid {
				continue
			}
			identity := key + "\x00" + s
			if _, dup := seen[identity]; dup {
				continue
			}
			seen[identity] = struct{}{}
			out = append(out, [2]string{key, s})
		}
	}
	return out
}
