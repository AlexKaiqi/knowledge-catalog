package writer

import (
	"strings"

	"kc/kernel"
	"kc/repository"
)

// validateSchemaRefs rejects PUTs whose schema_ref cannot resolve in the target repository.
//
// Empty schema_ref is skipped. A relative schema/* object may be created in the same ChangeSet.
// A kc:// form must name this repository. A pin must exist and resolve; an unpinned ref must
// resolve at ExpectedTargetCommit (or current Head).
//
// A target that does not interpret knowledge files is fine as long as nothing
// claims a schema_ref: writing files into a plain git repo is layer ⓪ work.
//
// Args:
//
//	target: target repository of the ChangeSet.
//	cs: snapshot ChangeSet about to apply.
//
// Returns:
//
//	SCHEMA_REVISION_UNRESOLVED when the ref cannot be parsed or resolved; otherwise nil.
func validateSchemaRefs(target repository.SnapshotStore, cs repository.CommitChangeSet) error {
	claimed := false
	batch := map[kernel.ObjectID]struct{}{}
	for _, op := range cs.Operations {
		if op.Op != repository.OpPut {
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
	repo, err := knowledgeForSchema(target)
	if err != nil {
		return err
	}
	at := cs.ExpectedTargetCommit
	if at == "" {
		head, err := repo.Head(cs.TargetRef)
		if err != nil {
			return err
		}
		at = head
	}
	for _, op := range cs.Operations {
		if op.Op != repository.OpPut || strings.TrimSpace(op.SchemaRef) == "" {
			continue
		}
		if err := checkSchemaRef(repo, at, batch, op.SchemaRef); err != nil {
			return err
		}
	}
	return nil
}

// knowledgeForSchema reports a plain target as an unresolvable schema_ref rather
// than a mount problem: the repo is mounted, it just cannot resolve schema/*.
func knowledgeForSchema(target repository.SnapshotStore) (repository.Repository, error) {
	repo, ok := repository.KnowledgeOf(target)
	if !ok {
		return nil, kernel.Fail(kernel.ErrSchemaRevisionUnresolved,
			"repository %s is mounted as a plain snapshot and cannot resolve schema/* objects", target.ID())
	}
	return repo, nil
}

// checkSchemaRef verifies one schema_ref against the target repo and write base.
//
// Args:
//
//	repo: target repository.
//	at: commit used when the ref has no pin (ExpectedTargetCommit or Head).
//	batch: object_ids PUT in the same ChangeSet.
//	ref: raw schema_ref string.
//
// Returns:
//
//	SCHEMA_REVISION_UNRESOLVED on parse, foreign-repo, or missing object; otherwise nil.
func checkSchemaRef(repo repository.Repository, at kernel.CommitID, batch map[kernel.ObjectID]struct{}, ref string) error {
	parsed, ok := kernel.ParseSchemaRef(ref)
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
	res, err := repo.Resolve(parsed.Object, at)
	if err != nil {
		return kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "schema_ref %q does not resolve to a schema object", ref)
	}
	if res.Status != repository.StatusResolved {
		return kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "schema_ref %q does not resolve to a schema object", ref)
	}
	return nil
}
