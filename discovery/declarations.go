package discovery

import (
	"fmt"
	"sort"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

// Declaration statuses. A declared member is not necessarily supported, and a
// supported member is not necessarily ready; each step is reported per member.
const (
	DeclarationReady       = "ready"
	DeclarationUnsupported = "unsupported"
	DeclarationInvalid     = "invalid"
	// DeclarationInapplicable marks a selected member whose input Schema is
	// not in this scope: no object here can carry the field, so a query on it
	// is a complete miss rather than an unsupported or undeclared condition.
	DeclarationInapplicable = "inapplicable"
)

// DeclarationStatus is one index declaration member as compiled against a
// fixed commit. Reason explains unsupported or invalid; other members build.
type DeclarationStatus struct {
	Repository kernel.RepositoryID `json:"repository,omitempty"`
	Commit     kernel.CommitID     `json:"commit,omitempty"`
	Member     objectmodel.Address `json:"member"`
	Catalog    string              `json:"catalog"`
	Status     string              `json:"status"`
	Reason     string              `json:"reason,omitempty"`
	Inputs     []FieldRef          `json:"inputs"`
	Uses       []string            `json:"uses"`
	Digest     kernel.Digest       `json:"digest"`
}

// DeclarationInput is one resolved input handed to a capability check.
type DeclarationInput struct {
	Field FieldRef
	Type  string
	// form is the Schema release the input names: `schema/x` follows every
	// adopted release that has the field, `schema/x@commit` only that release.
	form string
}

// DeclarationCheck is a plugin instance's capability manifest check. It returns
// "" when the instance supports the declaration, otherwise the reason. It never
// rewrites a declaration, for example by substituting a manifest version.
type DeclarationCheck func(declaration objectmodel.IndexDeclaration, inputs []DeclarationInput) string

// declaredAccess is the compiled retrieval faces of every ready member.
type declaredAccess struct {
	hints    map[string][]reader.AccessHint
	statuses []DeclarationStatus
	builds   []declarationBuild
	schemas  map[objectmodel.ObjectID]bool
	// inapplicable keeps selected members whose input Schema is outside the
	// scope, with their input locators, so queries on them miss completely.
	inapplicable []declarationBuild
}

// declarationBuild is execution metadata retained only for ready members.
// It keeps the declaration's construction rules separate from Schema structure.
type declarationBuild struct {
	member      objectmodel.Address
	declaration objectmodel.IndexDeclaration
	inputs      []DeclarationInput
}

func declaredFieldKey(form string, path string) string {
	return form + "\x1f" + path
}

// compileDeclarations is explicit maintenance: it locates the index
// declarations held at this commit through the bounded Schema referrer index
// and compiles them. Declarations are the only source of retrieval faces; a
// Schema never contributes one.
func compileDeclarations(repo objectmodel.Repository, commit kernel.CommitID, report reader.SchemaReport, check DeclarationCheck) (declaredAccess, error) {
	locator, ok := repo.(objectmodel.SchemaReferrerLocator)
	if !ok {
		return declaredAccess{hints: map[string][]reader.AccessHint{}}, nil
	}
	addresses, err := locator.SchemaReferrerAddresses(objectmodel.IndexDeclarationSchemaV1, commit)
	if err != nil {
		return declaredAccess{}, err
	}
	return compileDeclarationMembers(repo, commit, report, addresses, check)
}

// compileDeclarationMembers resolves the named declaration members by exact
// reads against the commit's Schema namespace. Consumers pass the members
// maintenance retained; they never enumerate referrers.
func compileDeclarationMembers(repo objectmodel.Repository, commit kernel.CommitID, report reader.SchemaReport, addresses []objectmodel.Address, check DeclarationCheck) (declaredAccess, error) {
	out := declaredAccess{hints: map[string][]reader.AccessHint{}, schemas: map[objectmodel.ObjectID]bool{}}
	schemas := map[string]reader.SchemaDescription{}
	for _, schema := range report.Schemas {
		schemas[string(schema.ObjectID)] = schema
	}
	release := func(ref objectmodel.ParsedSchemaRef) (string, reader.SchemaDescription, bool, error) {
		if ref.Repository == "" && ref.Commit == "" {
			schema, ok := schemas[string(ref.Object)]
			return string(ref.Object), schema, ok, nil
		}
		key := schemaFormKey(objectmodel.ParsedSchemaRef{Object: ref.Object, Commit: ref.Commit})
		pinned, current, err := describeSchemaRelease(repo, commit, ref)
		if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
			return key, reader.SchemaDescription{}, false, nil
		}
		if err != nil || len(pinned.Schemas) != 1 {
			return key, reader.SchemaDescription{}, false, err
		}
		if current {
			key = string(ref.Object)
		}
		schemas[key] = pinned.Schemas[0]
		return key, pinned.Schemas[0], true, nil
	}
	for _, address := range addresses {
		value, err := repo.ReadAddress(address, commit)
		if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
			// Outside the caller's Dataset scope: only declarations selected
			// into the Dataset take effect there.
			continue
		}
		if err != nil {
			return declaredAccess{}, err
		}
		status, inputs, declaration, err := compileDeclaration(address, value.Value, release)
		if err != nil {
			return declaredAccess{}, err
		}
		for _, input := range declaration.Inputs {
			out.schemas[input.Locator.Schema.Object] = true
		}
		status.Repository, status.Commit = value.Repository, value.Commit
		if status.Status == DeclarationReady && check != nil {
			if reason := check(declaration, inputs); reason != "" {
				status.Status, status.Reason = DeclarationUnsupported, reason
			}
		}
		if status.Status == DeclarationInapplicable {
			out.inapplicable = append(out.inapplicable, declarationBuild{member: address, declaration: declaration, inputs: locatorInputs(declaration)})
		}
		if status.Status == DeclarationReady {
			out.builds = append(out.builds, declarationBuild{member: address, declaration: declaration, inputs: inputs})
			for _, input := range inputs {
				key := declaredFieldKey(input.form, input.Field.Path)
				for _, use := range declaration.Uses {
					if hint, scalar := useHint(use); scalar {
						out.hints[key] = appendHint(out.hints[key], hint)
					}
				}
			}
		}
		out.statuses = append(out.statuses, status)
	}
	sort.Slice(out.statuses, func(i, j int) bool {
		return objectmodel.AddressKey(out.statuses[i].Member) < objectmodel.AddressKey(out.statuses[j].Member)
	})
	return out, nil
}

// schemaRelease resolves a locator's Schema to the release it names: the
// commit's own release when unpinned, otherwise the pinned release. form is
// the key faces attach to (`schema/x` or `schema/x@commit`).
type schemaRelease func(ref objectmodel.ParsedSchemaRef) (form string, schema reader.SchemaDescription, found bool, err error)

func compileDeclaration(address objectmodel.Address, value any, release schemaRelease) (DeclarationStatus, []DeclarationInput, objectmodel.IndexDeclaration, error) {
	status := DeclarationStatus{Member: address, Status: DeclarationReady, Digest: kernel.CanonicalDigest(value), Inputs: []FieldRef{}, Uses: []string{}}
	declaration, err := objectmodel.ParseIndexDeclaration(address, value)
	if err != nil {
		status.Status, status.Reason = DeclarationInvalid, err.Error()
		return status, nil, declaration, nil
	}
	status.Catalog, status.Uses = declaration.Catalog, append([]string{}, declaration.Uses...)
	inputs := make([]DeclarationInput, 0, len(declaration.Inputs))
	for _, input := range declaration.Inputs {
		ref := input.Locator.Schema
		form, schema, exists, err := release(ref)
		if err != nil {
			return status, nil, declaration, err
		}
		if !exists {
			if ref.Repository == "" && ref.Commit == "" {
				// The scope's own Schema namespace does not hold it, so no
				// object in scope can carry this field (discovery 完整性).
				status.Status, status.Reason = DeclarationInapplicable,
					fmt.Sprintf("input %s names a Schema not in this scope; no object here can match", input.From)
				return status, nil, declaration, nil
			}
			status.Status, status.Reason = DeclarationUnsupported,
				fmt.Sprintf("input %s names a Schema release outside this scope", input.From)
			return status, nil, declaration, nil
		}
		field, found := schemaField(schema, input.Locator.Path)
		if !found {
			status.Status, status.Reason = DeclarationInvalid,
				fmt.Sprintf("schema %s no longer has field %s", ref.Object, input.Locator.Path)
			return status, nil, declaration, nil
		}
		fieldRef := FieldRef{Schema: schema.ObjectID, Aspect: schema.Aspect, Path: field.Path}
		status.Inputs = append(status.Inputs, fieldRef)
		inputs = append(inputs, DeclarationInput{Field: fieldRef, Type: field.Type, form: form})
	}
	return status, inputs, declaration, nil
}

func schemaField(schema reader.SchemaDescription, path string) (reader.FieldAccess, bool) {
	for _, field := range schema.Fields {
		if field.Path == path {
			return field, true
		}
	}
	return reader.FieldAccess{}, false
}

// useHint maps a declaration use onto the scalar retrieval face the access
// primitives plan with. Semantic recall is a separate lane, not a face.
func useHint(use string) (reader.AccessHint, bool) {
	switch use {
	case objectmodel.IndexUseText:
		return reader.HintText, true
	case objectmodel.IndexUseFilter:
		return reader.HintFilter, true
	case objectmodel.IndexUseSort:
		return reader.HintSort, true
	}
	return "", false
}

func appendHint(hints []reader.AccessHint, hint reader.AccessHint) []reader.AccessHint {
	for _, existing := range hints {
		if existing == hint {
			return hints
		}
	}
	hints = append(hints, hint)
	sort.Slice(hints, func(i, j int) bool { return hints[i] < hints[j] })
	return hints
}

// annotate attaches declared faces to a structure-only Schema report of the
// commit's own releases. Fields a Schema release does not have stay without
// faces.
func (d declaredAccess) annotate(report reader.SchemaReport) reader.SchemaReport {
	return d.annotateRelease(report, "")
}

// annotateRelease attaches faces to a report of releases pinned at commit
// (empty for the commit's own releases). Unpinned members follow every
// release; members pinned to a release apply to that release only.
func (d declaredAccess) annotateRelease(report reader.SchemaReport, commit kernel.CommitID) reader.SchemaReport {
	out := report
	out.Schemas = make([]reader.SchemaDescription, len(report.Schemas))
	for i, schema := range report.Schemas {
		copied := schema
		copied.Fields = make([]reader.FieldAccess, len(schema.Fields))
		pinned := ""
		if commit != "" {
			pinned = schemaFormKey(objectmodel.ParsedSchemaRef{Object: schema.ObjectID, Commit: commit})
		}
		for j, field := range schema.Fields {
			field.Access = append([]reader.AccessHint(nil), d.hints[declaredFieldKey(string(schema.ObjectID), field.Path)]...)
			if pinned != "" {
				for _, hint := range d.hints[declaredFieldKey(pinned, field.Path)] {
					field.Access = appendHint(field.Access, hint)
				}
			}
			copied.Fields[j] = field
		}
		out.Schemas[i] = copied
	}
	return out
}

// Statuses lists every index declaration member compiled into this spec.
func (s AccessSpec) Statuses() []DeclarationStatus {
	return append([]DeclarationStatus(nil), s.Declarations...)
}

// UnreadyTextMembers returns the declared text members that did not compile to
// ready. An unfielded MATCH covers every declared text member, so these are the
// members such a query silently cannot reach.
func (s AccessSpec) UnreadyTextMembers() []DeclarationStatus {
	var members []DeclarationStatus
	for _, status := range s.Declarations {
		if status.Status == DeclarationReady || status.Status == DeclarationInapplicable {
			continue
		}
		for _, use := range status.Uses {
			if use == objectmodel.IndexUseText {
				members = append(members, status)
				break
			}
		}
	}
	return members
}

// locatorInputs names an inapplicable member's inputs by their locators; no
// Schema release in scope describes them.
func locatorInputs(declaration objectmodel.IndexDeclaration) []DeclarationInput {
	inputs := make([]DeclarationInput, 0, len(declaration.Inputs))
	for _, input := range declaration.Inputs {
		inputs = append(inputs, DeclarationInput{Field: FieldRef{Schema: input.Locator.Schema.Object, Path: input.Locator.Path}})
	}
	return inputs
}

// SemanticMembers returns the ready vector declarations selected by this scope.
func (s AccessSpec) SemanticMembers() []string {
	var members []string
	for _, status := range s.Declarations {
		if status.Status != DeclarationReady {
			continue
		}
		for _, use := range status.Uses {
			if use == objectmodel.IndexUseSemantic {
				members = append(members, objectmodel.AddressKey(status.Member))
				break
			}
		}
	}
	return members
}
