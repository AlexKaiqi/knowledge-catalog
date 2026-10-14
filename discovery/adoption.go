package discovery

import (
	"sort"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

type SchemaAdoption struct {
	SchemaRef string                `json:"schemaRef"`
	Witnesses []objectmodel.Address `json:"witnesses"`
}

// AccessSpecForRepository is explicit maintenance: it composes the current Schema namespace with releases
// explicitly adopted by indexed unit declarations. It uses the existing
// reverse dependency locator and exact declaration reads, never a tree scan.
func AccessSpecForRepository(repo objectmodel.Repository, commit kernel.CommitID) (AccessSpec, error) {
	report, err := reader.DescribeRepoSchema(repo, commit, "")
	if err != nil {
		return AccessSpec{}, err
	}
	spec := accessSpecWithCurrentForms(report)
	locator, ok := repo.(objectmodel.SchemaReferrerLocator)
	if !ok {
		return spec, nil
	}
	historical := map[string][]AccessField{}
	witnesses := map[string][]objectmodel.Address{}
	for _, schema := range report.Schemas {
		addresses, err := locator.SchemaReferrerAddresses(schema.ObjectID, commit)
		if err != nil {
			return AccessSpec{}, err
		}
		for _, address := range addresses {
			unit, err := repo.ResolveAddress(address, commit)
			if err != nil {
				return AccessSpec{}, err
			}
			ref, ok := objectmodel.ParseSchemaRef(unit.SchemaRef)
			if !ok || ref.Object != schema.ObjectID || (ref.Repository != "" && ref.Repository != repo.ID()) {
				return AccessSpec{}, kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "indexed schema adoption does not match its declaration")
			}
			if ref.Commit == "" || ref.Commit == commit {
				continue
			}
			key := schemaFormKey(ref)
			witnesses[key] = append(witnesses[key], address)
			if _, exists := historical[key]; exists {
				continue
			}
			adopted, err := reader.DescribeRepoSchema(repo, ref.Commit, ref.Object)
			if err != nil {
				return AccessSpec{}, err
			}
			historical[key] = AccessSpecFromReport(adopted).Fields
		}
	}
	keys := make([]string, 0, len(historical))
	for key := range historical {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fields := historical[key]
		spec.adoptions = append(spec.adoptions, SchemaAdoption{SchemaRef: key, Witnesses: witnesses[key]})
		spec.schemaForms[key] = fields
		if err := spec.mergeAccessFields(fields); err != nil {
			return AccessSpec{}, err
		}
	}
	finalizeAdoptedAccessSpec(&spec)
	return spec, nil
}

// AdoptionWitnesses is retained by projection maintenance at the same commit.
// It carries declarations, not object bodies or caller permissions.
func (s AccessSpec) AdoptionWitnesses() []SchemaAdoption {
	out := make([]SchemaAdoption, len(s.adoptions))
	for i, adoption := range s.adoptions {
		out[i] = SchemaAdoption{SchemaRef: adoption.SchemaRef, Witnesses: append([]objectmodel.Address(nil), adoption.Witnesses...)}
	}
	return out
}

// AccessSpecFromAdoptions verifies retained maintenance witnesses using exact
// reads under the caller's fixed scope. It never enumerates instance bodies or
// reads a repository-wide locator. A projection is not a Schema authority.
func AccessSpecFromAdoptions(repo objectmodel.Repository, commit kernel.CommitID, adoptions []SchemaAdoption) (AccessSpec, error) {
	report, err := reader.DescribeRepoSchema(repo, commit, "")
	if err != nil {
		return AccessSpec{}, err
	}
	spec := accessSpecWithCurrentForms(report)
	for _, adoption := range adoptions {
		ref, valid := objectmodel.ParseSchemaRef(adoption.SchemaRef)
		if !valid || ref.Commit == "" || len(adoption.Witnesses) == 0 || (ref.Repository != "" && ref.Repository != repo.ID()) {
			return AccessSpec{}, kernel.Fail(kernel.ErrPreconditionFailed, "projection contains an invalid Schema adoption")
		}
		if _, visible := spec.schemaForms[string(ref.Object)]; !visible {
			continue
		}
		verified := false
		for _, address := range adoption.Witnesses {
			unit, err := repo.ResolveAddress(address, commit)
			if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
				continue
			}
			if err != nil {
				return AccessSpec{}, err
			}
			actual, ok := objectmodel.ParseSchemaRef(unit.SchemaRef)
			if !ok || actual.Object != ref.Object || actual.Commit != ref.Commit || (actual.Repository != "" && actual.Repository != repo.ID()) {
				return AccessSpec{}, kernel.Fail(kernel.ErrPreconditionFailed, "projection Schema adoption does not match its fixed declaration")
			}
			verified = true
			break
		}
		if !verified {
			continue
		}
		adopted, err := reader.DescribeRepoSchema(repo, ref.Commit, ref.Object)
		if err != nil {
			return AccessSpec{}, err
		}
		fields := AccessSpecFromReport(adopted).Fields
		spec.schemaForms[schemaFormKey(ref)] = fields
		if err := spec.mergeAccessFields(fields); err != nil {
			return AccessSpec{}, err
		}
		spec.adoptions = append(spec.adoptions, adoption)
	}
	finalizeAdoptedAccessSpec(&spec)
	return spec, nil
}

func schemaFormKey(ref objectmodel.ParsedSchemaRef) string {
	key := string(ref.Object)
	if ref.Commit != "" {
		key += "@" + string(ref.Commit)
	}
	return key
}

// FieldsForDeclaration returns the exact release's promises. Untyped units
// remain structurally typed against the namespace's access declarations.
func (s AccessSpec) FieldsForDeclaration(ref string) ([]AccessField, error) {
	if s.schemaForms == nil {
		return cloneAccessFields(s.Fields), nil
	}
	if ref == "" {
		return cloneAccessFields(s.currentFields), nil
	}
	parsed, ok := objectmodel.ParseSchemaRef(ref)
	if !ok || (parsed.Repository != "" && parsed.Repository != s.Repository) {
		return nil, kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "invalid adopted Schema reference %q", ref)
	}
	if parsed.Commit == s.Commit {
		parsed.Commit = ""
	}
	fields, exists := s.schemaForms[schemaFormKey(parsed)]
	if !exists {
		return nil, kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "adopted Schema %q has no fixed access contract", ref)
	}
	return cloneAccessFields(fields), nil
}

func (s *AccessSpec) mergeAccessFields(fields []AccessField) error {
	for _, field := range fields {
		position := -1
		for i := range s.Fields {
			if s.Fields[i].FieldRef == field.FieldRef {
				position = i
				break
			}
		}
		if position < 0 {
			s.Fields = append(s.Fields, field)
			found := false
			for _, id := range s.Schemas {
				found = found || id == field.Schema
			}
			if !found {
				s.Schemas = append(s.Schemas, field.Schema)
			}
			continue
		}
		current := &s.Fields[position]
		if current.Type != field.Type {
			return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "adopted Schema releases disagree on scalar type for %s", field.FieldRef.Key())
		}
		for _, hint := range field.Access {
			if !current.Has(hint) {
				current.Access = append(append([]reader.AccessHint(nil), current.Access...), hint)
			}
		}
		sort.Slice(current.Access, func(i, j int) bool { return current.Access[i] < current.Access[j] })
	}
	return nil
}

func accessSpecWithCurrentForms(report reader.SchemaReport) AccessSpec {
	spec := AccessSpecFromReport(report)
	spec.currentFields = append([]AccessField(nil), spec.Fields...)
	spec.schemaForms = map[string][]AccessField{}
	for _, schema := range report.Schemas {
		spec.schemaForms[string(schema.ObjectID)] = AccessSpecFromReport(reader.SchemaReport{Schemas: []reader.SchemaDescription{schema}}).Fields
	}
	return spec
}

func finalizeAdoptedAccessSpec(spec *AccessSpec) {
	if len(spec.adoptions) == 0 {
		return
	}
	sortAccessFields(spec.Fields)
	sortSchemaIDs(spec.Schemas)
	spec.AccessDigest = kernel.CanonicalDigest(map[string]any{"fields": spec.Fields, "adopted": spec.schemaForms})
}

func cloneAccessFields(fields []AccessField) []AccessField {
	out := append([]AccessField(nil), fields...)
	for i := range out {
		out[i].Access = append([]reader.AccessHint(nil), out[i].Access...)
	}
	return out
}
