package reader

import "kc/kernel"

// IndexField is one AccessHint-bearing path compiled into a projection.
type IndexField struct {
	Schema kernel.ObjectID `json:"schema"`
	Entity string          `json:"entity,omitempty"`
	Aspect string          `json:"aspect,omitempty"`
	Path   string          `json:"path"`
	Type   string          `json:"type,omitempty"`
	Access []AccessHint    `json:"access"`
}

func (f IndexField) Has(hint AccessHint) bool {
	for _, item := range f.Access {
		if item == hint {
			return true
		}
	}
	return false
}

// HasHint reports whether any compiled field carries this retrieval face.
//
// Args:
//
//	hint: AccessHint to look for (text, filter, …).
//
// Returns:
//
//	true when at least one IndexField.Has(hint).
func (s IndexSpec) HasHint(hint AccessHint) bool {
	for _, field := range s.Fields {
		if field.Has(hint) {
			return true
		}
	}
	return false
}

// QueryLanes lists retrieval faces that appear in this spec, in stable order.
//
// Returns:
//
//	key / filter / text / sort that are present. summary and stored are not query lanes.
func (s IndexSpec) QueryLanes() []string {
	order := []AccessHint{HintKey, HintFilter, HintText, HintSort}
	names := []string{"key", "filter", "text", "sort"}
	var out []string
	for i, hint := range order {
		if s.HasHint(hint) {
			out = append(out, names[i])
		}
	}
	return out
}

// Bind restricts the spec to schemas named by schema_ref. Empty refs keep the full spec.
//
// Args:
//
//	refs: schema_ref strings from the object's units (schema/foo or kc://…/schema/foo).
//
// Returns:
//
//	a spec containing only fields whose Schema is in refs; digest is left as the parent digest.
func (s IndexSpec) Bind(refs []string) IndexSpec {
	if len(refs) == 0 {
		return s
	}
	want := map[kernel.ObjectID]struct{}{}
	for _, ref := range refs {
		parsed, ok := kernel.ParseSchemaRef(ref)
		if !ok {
			continue
		}
		want[parsed.Object] = struct{}{}
	}
	if len(want) == 0 {
		return s
	}
	out := IndexSpec{Fields: []IndexField{}, Schemas: []kernel.ObjectID{}, Digest: s.Digest}
	seen := map[kernel.ObjectID]struct{}{}
	for _, field := range s.Fields {
		if _, ok := want[field.Schema]; !ok {
			continue
		}
		out.Fields = append(out.Fields, field)
		if _, dup := seen[field.Schema]; dup {
			continue
		}
		seen[field.Schema] = struct{}{}
		out.Schemas = append(out.Schemas, field.Schema)
	}
	return out
}

// IndexSpec is the compiled recipe for one repo commit. Digest changes ⇒ rebuild.
type IndexSpec struct {
	Fields  []IndexField      `json:"fields"`
	Schemas []kernel.ObjectID `json:"schemas"`
	Digest  kernel.Digest     `json:"digest"`
}

func SpecFromReport(report SchemaReport) IndexSpec {
	spec := IndexSpec{Fields: []IndexField{}, Schemas: []kernel.ObjectID{}}
	for _, schema := range report.Schemas {
		used := false
		for _, field := range schema.Fields {
			if len(field.Access) == 0 {
				continue
			}
			used = true
			spec.Fields = append(spec.Fields, IndexField{
				Schema: schema.ObjectID,
				Entity: schema.Entity,
				Aspect: schema.Aspect,
				Path:   field.Path,
				Type:   field.Type,
				Access: append([]AccessHint{}, field.Access...),
			})
		}
		if used {
			spec.Schemas = append(spec.Schemas, schema.ObjectID)
		}
	}
	sortIndexFields(spec.Fields)
	sortSchemaIDs(spec.Schemas)
	spec.Digest = kernel.CanonicalDigest(spec.Fields)
	return spec
}

func sortIndexFields(fields []IndexField) {
	for i := 0; i < len(fields); i++ {
		for j := i + 1; j < len(fields); j++ {
			if indexFieldLess(fields[j], fields[i]) {
				fields[i], fields[j] = fields[j], fields[i]
			}
		}
	}
}

func indexFieldLess(a, b IndexField) bool {
	if a.Schema != b.Schema {
		return a.Schema < b.Schema
	}
	if a.Aspect != b.Aspect {
		return a.Aspect < b.Aspect
	}
	return a.Path < b.Path
}

func sortSchemaIDs(ids []kernel.ObjectID) {
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if ids[j] < ids[i] {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
}
