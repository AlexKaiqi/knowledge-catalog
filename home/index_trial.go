package home

import (
	retrieval "kc/discovery"
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/snapshot"
)

// TrialIndexDeclarations checks proposed publication material without applying
// it. authorize must prove complete read access to every Repository consulted;
// the application supplies its existing authorization seam.
func (ws *Home) TrialIndexDeclarations(name string, request retrieval.DeclarationTrialRequest, authorize func(kernel.RepositoryID) error) (retrieval.DeclarationTrialResult, error) {
	out := retrieval.DeclarationTrialResult{Repository: request.Repository, Commit: request.Commit, Members: []retrieval.DeclarationTrialMember{}}
	if authorize == nil || request.Repository == "" || len(request.Documents) == 0 {
		return out, kernel.Fail(kernel.ErrUsageInvalid, "trial requires a target Repository, declaration documents and authorized Schema reads")
	}
	if request.Commit != "" && request.Ref != "" {
		return out, kernel.Fail(kernel.ErrUsageInvalid, "trial chooses --commit or --ref")
	}
	if err := authorize(request.Repository); err != nil {
		return out, err
	}
	store, err := ws.Store.Require(request.Repository, kernel.ErrObjectRefUnresolved)
	if err != nil {
		return out, err
	}
	ref := snapshot.RefOrDefault(request.Ref)
	if out.Commit == "" {
		out.Commit, err = store.Head(ref)
		if err != nil {
			return out, err
		}
	}
	cs, units, err := trialChangeSet(request, out.Commit, ref, ws.storedSchemaRef(request.Repository, out.Commit))
	if err != nil {
		return out, err
	}
	if err := ws.authorizeTrialSchemas(cs, units, authorize); err != nil {
		return out, err
	}
	for i, unit := range units {
		if !objectmodel.IsIndexDeclarationRef(unit.SchemaRef) {
			continue
		}
		member := retrieval.DeclarationTrialMember{Path: unit.Path, Member: unit.Address, Status: retrieval.DeclarationInvalid}
		declaration, fields, err := ws.Writer.CheckIndexDeclaration(memberChangeSet(cs, units, i), unit.Address)
		if err != nil {
			member.Reason, member.Code = err.Error(), string(kernel.CodeOf(err))
			out.Members = append(out.Members, member)
			continue
		}
		member.Catalog = declaration.Catalog
		capabilities, selection, err := ws.Plugins.DeclarationCapabilities(name, declaration.Catalog)
		if err != nil {
			member.Status, member.Reason, member.Code = retrieval.DeclarationUnsupported, err.Error(), string(kernel.CodeOf(err))
			out.Members = append(out.Members, member)
			continue
		}
		inputs := make([]retrieval.DeclarationInput, len(fields))
		for i, field := range fields {
			inputs[i] = retrieval.DeclarationInput{Type: field.Type}
		}
		member.Instance = selection.Instance
		member.Status = "supported"
		if reason := capabilities.Check(declaration, inputs); reason != "" {
			member.Status, member.Reason = retrieval.DeclarationUnsupported, reason
		}
		rebuild, err := ws.trialRebuild(request.Repository, out.Commit, unit.Address, declaration, capabilities.Rebuilds)
		if err != nil {
			return out, err
		}
		member.Rebuild = &rebuild
		out.Members = append(out.Members, member)
	}
	if len(out.Members) == 0 {
		return out, kernel.Fail(kernel.ErrUsageInvalid, "trial documents contain no index declaration members")
	}
	return out, nil
}

// trialChangeSet keeps each operation's schema_ref exactly as written, so
// Writer applies its own inheritance; the unit copy carries the effective ref
// only to decide which documents are declaration members.
func trialChangeSet(request retrieval.DeclarationTrialRequest, commit kernel.CommitID, ref string, stored func(objectmodel.Address) (string, error)) (objectmodel.ChangeSet, []repofile.Unit, error) {
	cs := objectmodel.ChangeSet{TargetRepository: request.Repository, TargetRef: ref, BaseCommit: commit, ExpectedTargetCommit: commit}
	tree, units := repofile.NewTree(), []repofile.Unit{}
	for _, document := range request.Documents {
		path, err := repofile.SafeRelativePath(document.Path)
		if err != nil {
			return cs, nil, err
		}
		unit := repofile.Parse(document.Content)
		if unit == nil {
			return cs, nil, kernel.Fail(kernel.ErrUsageInvalid, "%s has no publication frontmatter", path)
		}
		written := unit.SchemaRef
		if written == "" && !objectmodel.IsSchemaObject(unit.ObjectID) {
			if unit.SchemaRef, err = stored(unit.Address); err != nil {
				return cs, nil, err
			}
		}
		if !objectmodel.IsSchemaObject(unit.ObjectID) && !objectmodel.IsIndexDeclarationRef(unit.SchemaRef) {
			return cs, nil, kernel.Fail(kernel.ErrUsageInvalid, "%s is neither an index declaration nor a same-batch Schema draft", path)
		}
		if err := repofile.Ingest(tree, unit, path); err != nil {
			return cs, nil, err
		}
		unit.Path = path
		units = append(units, *unit)
		cs.Operations = append(cs.Operations, objectmodel.Operation{Op: objectmodel.OpPut, Address: unit.Address, SchemaRef: written, ValueSource: unit.ValueSource, Value: unit.Value, PathHint: repofile.PathHintForIngest(unit.Address, unit.PathHint, path)})
	}
	return cs, units, nil
}

// memberChangeSet judges one member against the shared same-batch Schema
// drafts only, so a sibling's defect never becomes this member's verdict.
func memberChangeSet(cs objectmodel.ChangeSet, units []repofile.Unit, member int) objectmodel.ChangeSet {
	out := cs
	out.Operations = nil
	for i, op := range cs.Operations {
		if i == member || objectmodel.IsSchemaObject(units[i].ObjectID) {
			out.Operations = append(out.Operations, op)
		}
	}
	return out
}

// storedSchemaRef returns the schema_ref already declared at an Address on the
// trial basis; a missing unit has none to inherit.
func (ws *Home) storedSchemaRef(repoID kernel.RepositoryID, commit kernel.CommitID) func(objectmodel.Address) (string, error) {
	return func(address objectmodel.Address) (string, error) {
		repo, err := ws.Reader.Require(repoID, kernel.ErrObjectRefUnresolved)
		if err != nil {
			return "", err
		}
		value, err := repo.ReadAddress(address, commit)
		if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		for _, declaration := range value.Declarations {
			if declaration.Address == address {
				return declaration.SchemaRef, nil
			}
		}
		return "", nil
	}
}

func (ws *Home) authorizeTrialSchemas(cs objectmodel.ChangeSet, units []repofile.Unit, authorize func(kernel.RepositoryID) error) error {
	drafts := map[objectmodel.ObjectID]bool{}
	for _, unit := range units {
		drafts[unit.ObjectID] = objectmodel.IsSchemaObject(unit.ObjectID)
	}
	repo, err := ws.Reader.Require(cs.TargetRepository, kernel.ErrObjectRefUnresolved)
	if err != nil {
		return err
	}
	for _, unit := range units {
		if !objectmodel.IsIndexDeclarationRef(unit.SchemaRef) {
			continue
		}
		declaration, err := objectmodel.ParseIndexDeclaration(unit.Address, unit.Value)
		if err != nil {
			continue // Writer returns the same shape failure, with no Schema read.
		}
		for _, input := range declaration.Inputs {
			ref := input.Locator.Schema
			if ref.Repository != "" {
				if err := authorize(ref.Repository); err != nil {
					return err
				}
				continue
			}
			if drafts[ref.Object] || ref.Commit != "" {
				continue
			}
			_, err := repo.ReadAddress(objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: ref.Object}, cs.BaseCommit)
			if err == nil {
				continue
			}
			if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
				return err
			}
			// A bare missing local Schema makes Writer inspect other source
			// namespaces. Prove every candidate before any such read; a refusal
			// reveals neither the private holder nor whether it exists there.
			for _, id := range ws.Store.IDs() {
				if id != cs.TargetRepository && authorize(id) != nil {
					return kernel.Fail(kernel.ErrForbidden, "trial cannot inspect all candidate Schema repositories; use an explicit kc://<repo>/<schema> locator with read access")
				}
			}
		}
	}
	return nil
}

func (ws *Home) trialRebuild(repoID kernel.RepositoryID, commit kernel.CommitID, address objectmodel.Address, next objectmodel.IndexDeclaration, rebuilds func(objectmodel.IndexDeclaration, objectmodel.IndexDeclaration) bool) (bool, error) {
	repo, err := ws.Reader.Require(repoID, kernel.ErrObjectRefUnresolved)
	if err != nil {
		return false, err
	}
	previous, err := repo.ReadAddress(address, commit)
	if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	parsed, err := objectmodel.ParseIndexDeclaration(address, previous.Value)
	if err != nil {
		return true, nil // replacing an invalid member requires a build.
	}
	return rebuilds(parsed, next), nil
}
