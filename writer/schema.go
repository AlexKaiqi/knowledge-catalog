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
// Args:
//
//	repo: target repository of the ChangeSet.
//	cs: snapshot ChangeSet about to apply.
//
// Returns:
//
//	SCHEMA_REVISION_UNRESOLVED when the ref cannot be parsed or resolved; otherwise nil.
func validateSchemaRefs(repo repository.Repository, cs repository.CommitChangeSet) error {
	at := cs.ExpectedTargetCommit
	if at == "" {
		head, err := repo.Head(cs.TargetRef)
		if err != nil {
			return err
		}
		at = head
	}
	batch := map[kernel.ObjectID]struct{}{}
	for _, op := range cs.Operations {
		if op.Op == repository.OpPut {
			batch[op.Address.ObjectID] = struct{}{}
		}
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

// validateAppendSchemaRefs rejects stream entries whose schema_ref cannot resolve at HEAD.
//
// Args:
//
//	repo: target repository of the APPEND.
//	entries: stream entries about to append.
//
// Returns:
//
//	SCHEMA_REVISION_UNRESOLVED when a ref cannot be parsed or resolved; otherwise nil.
func validateAppendSchemaRefs(repo repository.Repository, entries []repository.AppendEntry) error {
	head, err := repo.Head("")
	if err != nil {
		return err
	}
	batch := map[kernel.ObjectID]struct{}{}
	for _, entry := range entries {
		if strings.TrimSpace(entry.SchemaRef) == "" {
			continue
		}
		if err := checkSchemaRef(repo, head, batch, entry.SchemaRef); err != nil {
			return err
		}
	}
	return nil
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
			return kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "schema_ref %q commit is unresolved", ref)
		}
		at = parsed.Commit
	} else if _, ok := batch[parsed.Object]; ok {
		return nil
	}
	res, err := repo.Resolve(parsed.Object, at)
	if err != nil {
		return kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "schema_ref %q is unresolved", ref)
	}
	if res.Status != repository.StatusResolved {
		return kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "schema_ref %q is unresolved", ref)
	}
	return nil
}
