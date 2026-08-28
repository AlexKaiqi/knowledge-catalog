package writer

import (
	"strings"

	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// validateSchemaRefs rejects PUTs whose schema_ref cannot resolve in the target repository.
//
// Empty schema_ref is skipped. A relative schema/* object may be created in the same ChangeSet.
// A kc:// form must name this repository. A pin must exist and resolve; an unpinned ref must
// resolve at ExpectedTargetCommit (or current Head).
//
// A target that does not interpret knowledge files is fine as long as nothing
// claims a schema_ref: writing files into a plain Git Repository is layer ⓪ work.
//
// Args:
//
//	target: target repository of the ChangeSet.
//	cs: snapshot ChangeSet about to apply.
//
// Returns:
//
//	SCHEMA_REVISION_UNRESOLVED when the ref cannot be parsed or resolved; otherwise nil.
func validateSchemaRefs(target snapshot.Store, cs knowledge.ChangeSet) error {
	claimed := false
	batch := map[knowledge.ObjectID]struct{}{}
	for _, op := range cs.Operations {
		if op.Op != knowledge.OpPut {
			continue
		}
		batch[op.Address.ObjectID] = struct{}{}
		if strings.TrimSpace(op.SchemaRef) != "" {
			claimed = true
		}
	}
	if !claimed {
		return nil
	}
	native, nativeOK := target.(knowledge.NativeRepository)
	tree, treeOK := snapshot.TreeStoreOf(target)
	if !nativeOK && !treeOK {
		return kernel.Fail(kernel.ErrSchemaRevisionUnresolved,
			"repository %s has no immutable tree access for schema resolution", target.ID())
	}
	at := cs.ExpectedTargetCommit
	if at == "" {
		head, err := target.Head(cs.TargetRef)
		if err != nil {
			return err
		}
		at = head
	}
	checked := map[string]struct{}{}
	for _, op := range cs.Operations {
		if op.Op != knowledge.OpPut || strings.TrimSpace(op.SchemaRef) == "" {
			continue
		}
		ref := strings.TrimSpace(op.SchemaRef)
		if _, ok := checked[ref]; ok {
			continue
		}
		var err error
		if nativeOK {
			err = checkNativeSchemaRef(target, native, at, batch, ref)
		} else {
			err = checkSchemaRef(target, tree, at, batch, ref)
		}
		if err != nil {
			return err
		}
		checked[ref] = struct{}{}
	}
	return nil
}

// checkNativeSchemaRef resolves schema identity through a provider that owns
// layer ②. Native repositories such as Dolt may expose a compatibility tree,
// but that projection is not their canonical knowledge lookup surface.
func checkNativeSchemaRef(repo snapshot.Store, native knowledge.NativeRepository, at kernel.CommitID, batch map[knowledge.ObjectID]struct{}, ref string) error {
	parsed, ok := knowledge.ParseSchemaRef(ref)
	if !ok {
		return kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "schema_ref %q is not a pinned schema object", ref)
	}
	if parsed.Repository != "" && parsed.Repository != repo.ID() {
		return kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "schema_ref %q must name the target repository", ref)
	}
	if parsed.Commit != "" {
		if !repo.HasCommit(parsed.Commit) {
			return kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "schema_ref %q commit does not exist", ref)
		}
		at = parsed.Commit
	} else if _, ok := batch[parsed.Object]; ok {
		return nil
	}
	resolved, err := native.Resolve(parsed.Object, at)
	if err != nil || resolved.Status != knowledge.StatusResolved {
		return kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "schema_ref %q does not resolve to a schema object", ref)
	}
	return nil
}

// checkSchemaRef verifies one schema_ref against the target Repository and write base.
//
// Args:
//
//	repo: target Repository.
//	at: commit used when the ref has no pin (ExpectedTargetCommit or Head).
//	batch: object_ids PUT in the same ChangeSet.
//	ref: raw schema_ref string.
//
// Returns:
//
//	SCHEMA_REVISION_UNRESOLVED on parse, foreign-Repository, or missing object; otherwise nil.
func checkSchemaRef(repo snapshot.Store, tree snapshot.TreeStore, at kernel.CommitID, batch map[knowledge.ObjectID]struct{}, ref string) error {
	parsed, ok := knowledge.ParseSchemaRef(ref)
	if !ok {
		return kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "schema_ref %q is not a pinned schema object", ref)
	}
	if parsed.Repository != "" && parsed.Repository != repo.ID() {
		return kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "schema_ref %q must name the target repository", ref)
	}
	if parsed.Commit != "" {
		if !repo.HasCommit(parsed.Commit) {
			return kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "schema_ref %q commit does not exist", ref)
		}
		at = parsed.Commit
	} else if _, ok := batch[parsed.Object]; ok {
		return nil
	}
	index, err := readKnowledgeTree(tree, at)
	if err != nil {
		return kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "schema_ref %q does not resolve to a schema object", ref)
	}
	if len(index.ObjectUnits(parsed.Object)) == 0 {
		return kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "schema_ref %q does not resolve to a schema object", ref)
	}
	return nil
}
