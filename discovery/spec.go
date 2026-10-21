package discovery

import (
	"strings"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

type FieldRef struct {
	Schema objectmodel.ObjectID `json:"schema"`
	Aspect string               `json:"aspect,omitempty"`
	Path   string               `json:"path"`
}

func (f FieldRef) Key() string {
	return string(f.Schema) + "\x1f" + f.Aspect + "\x1f" + f.Path
}

// AccessField is one AccessHint-bearing path compiled into a projection.
type AccessField struct {
	FieldRef
	Entity string              `json:"entity,omitempty"`
	Type   string              `json:"type,omitempty"`
	Access []reader.AccessHint `json:"access"`
}

func (f AccessField) Has(hint reader.AccessHint) bool {
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
//	true when at least one AccessField.Has(hint).
func (s AccessSpec) HasHint(hint reader.AccessHint) bool {
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
//	filter / text / sort that are present.
func (s AccessSpec) QueryLanes() []string {
	order := []reader.AccessHint{reader.HintFilter, reader.HintText, reader.HintSort}
	names := []string{"filter", "text", "sort"}
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
func (s AccessSpec) Bind(refs []string) AccessSpec {
	if len(refs) == 0 {
		return s
	}
	want := map[objectmodel.ObjectID]struct{}{}
	for _, ref := range refs {
		parsed, ok := objectmodel.ParseSchemaRef(ref)
		if !ok {
			continue
		}
		want[parsed.Object] = struct{}{}
	}
	if len(want) == 0 {
		return s
	}
	out := AccessSpec{Repository: s.Repository, Commit: s.Commit, Fields: []AccessField{}, Schemas: []objectmodel.ObjectID{}, AccessDigest: s.AccessDigest, Declarations: s.Declarations, schemaForms: s.schemaForms, currentFields: s.currentFields, adoptions: s.adoptions, builds: s.builds, declarationSchemas: s.declarationSchemas, inapplicable: s.inapplicable}
	seen := map[objectmodel.ObjectID]struct{}{}
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

// AccessSpec is the versioned logical access contract for one Repository commit.
type AccessSpec struct {
	Repository   kernel.RepositoryID    `json:"repository"`
	Commit       kernel.CommitID        `json:"commit"`
	Fields       []AccessField          `json:"fields"`
	Schemas      []objectmodel.ObjectID `json:"schemas"`
	AccessDigest kernel.Digest          `json:"accessDigest"`
	// Declarations are the index declaration members compiled at Commit, with
	// per-member status. They are the only source of Fields' retrieval faces.
	Declarations []DeclarationStatus `json:"declarations,omitempty"`
	// schemaForms is execution metadata for actually adopted releases. It is
	// not a second declaration form or an addition to the public wire shape.
	schemaForms        map[string][]AccessField
	currentFields      []AccessField
	adoptions          []SchemaAdoption
	builds             []declarationBuild
	declarationSchemas map[objectmodel.ObjectID]bool
	inapplicable       []declarationBuild
}

func AccessSpecFromReport(report reader.SchemaReport) AccessSpec {
	spec := AccessSpec{Repository: report.Repository, Commit: report.Commit, Fields: []AccessField{}, Schemas: []objectmodel.ObjectID{}}
	for _, schema := range report.Schemas {
		used := false
		for _, field := range schema.Fields {
			if len(field.Access) == 0 {
				continue
			}
			used = true
			spec.Fields = append(spec.Fields, AccessField{
				FieldRef: FieldRef{Schema: schema.ObjectID, Aspect: schema.Aspect, Path: field.Path},
				Entity:   schema.Entity,
				Type:     field.Type,
				Access:   append([]reader.AccessHint{}, field.Access...),
			})
		}
		if used {
			spec.Schemas = append(spec.Schemas, schema.ObjectID)
		}
	}
	sortAccessFields(spec.Fields)
	sortSchemaIDs(spec.Schemas)
	spec.AccessDigest = kernel.CanonicalDigest(spec.Fields)
	return spec
}

// ResolveField resolves a query field. A bare path is accepted only when it is
// unambiguous across (schema, aspect, path).
func (s AccessSpec) ResolveField(ref FieldRef) (AccessField, error) {
	var matches []AccessField
	for _, field := range s.Fields {
		if ref.Schema != "" && field.Schema != ref.Schema {
			continue
		}
		if ref.Aspect != "" && field.Aspect != ref.Aspect {
			continue
		}
		if field.Path == ref.Path {
			matches = append(matches, field)
		}
	}
	if len(matches) == 0 {
		if unready := s.unreadyDeclarers(ref); len(unready) > 0 {
			return AccessField{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
				"field %s is declared only by members that did not compile to ready: %s", displayFieldRef(ref), strings.Join(unready, "; "))
		}
		return AccessField{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "field %s is not in AccessSpec", displayFieldRef(ref))
	}
	if len(matches) > 1 {
		return AccessField{}, kernel.Fail(kernel.ErrUsageInvalid, "field path %s is ambiguous; use one of %s", ref.Path, qualifiedFieldChoices(matches))
	}
	return matches[0], nil
}

// qualifiedFieldChoices lists the flag spelling a caller can paste into --eq.
// Aspect may be empty; schema::::path still parses as three segments.
func qualifiedFieldChoices(matches []AccessField) string {
	ordered := append([]AccessField(nil), matches...)
	for i := 0; i < len(ordered); i++ {
		for j := i + 1; j < len(ordered); j++ {
			if indexFieldLess(ordered[j], ordered[i]) {
				ordered[i], ordered[j] = ordered[j], ordered[i]
			}
		}
	}
	choices := make([]string, len(ordered))
	for i, field := range ordered {
		choices[i] = string(field.Schema) + "::" + field.Aspect + "::" + field.Path
	}
	return strings.Join(choices, ", ")
}

// unreadyDeclarers names the non-ready members whose inputs carry ref, so a
// caller learns why a declared field has no searchable face instead of being
// told it is absent.
func (s AccessSpec) unreadyDeclarers(ref FieldRef) []string {
	var out []string
	for _, status := range s.Declarations {
		if status.Status == DeclarationReady {
			continue
		}
		for _, input := range status.Inputs {
			if input.Path != ref.Path || ref.Schema != "" && input.Schema != ref.Schema ||
				ref.Aspect != "" && input.Aspect != "" && input.Aspect != ref.Aspect {
				continue
			}
			label := objectmodel.AddressLabel(status.Member) + " (" + status.Status
			if status.Reason != "" {
				label += ": " + status.Reason
			}
			out = append(out, label+")")
			break
		}
	}
	return out
}

func displayFieldRef(ref FieldRef) string {
	parts := []string{string(ref.Schema), ref.Aspect, ref.Path}
	return strings.Trim(strings.Join(parts, "/"), "/")
}

func sortAccessFields(fields []AccessField) {
	for i := 0; i < len(fields); i++ {
		for j := i + 1; j < len(fields); j++ {
			if indexFieldLess(fields[j], fields[i]) {
				fields[i], fields[j] = fields[j], fields[i]
			}
		}
	}
}

func indexFieldLess(a, b AccessField) bool {
	if a.Schema != b.Schema {
		return a.Schema < b.Schema
	}
	if a.Aspect != b.Aspect {
		return a.Aspect < b.Aspect
	}
	return a.Path < b.Path
}

func sortSchemaIDs(ids []objectmodel.ObjectID) {
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if ids[j] < ids[i] {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
}
