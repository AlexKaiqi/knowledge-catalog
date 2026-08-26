package gitea

import (
	"sort"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// TreeStore (snapshot.TreeStore): literal path read/write at a
// commit, no frontmatter, no object_id. Sibling to the Knowledge methods in
// read.go / commit.go, not a replacement for them. scanAt already fetches
// every blob's SHA (blobs map) regardless of whether it is a knowledge-shaped
// path — this just reads/writes through that instead of the object_id index.
var _ snapshot.TreeStore = (*Repository)(nil)

func (r *Repository) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	_, blobs, err := r.scanAt(commit)
	if err != nil {
		return nil, err
	}
	sha, ok := blobs[path]
	if !ok {
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "path %s is missing at commit %s", path, commit)
	}
	content, err := r.readBlob(sha)
	if err != nil {
		return nil, err
	}
	return []byte(content), nil
}

func (r *Repository) ListFiles(commit kernel.CommitID) ([]string, error) {
	_, blobs, err := r.scanAt(commit)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(blobs))
	for p := range blobs {
		// Gitea cannot create a branch in a completely empty repository, so the
		// adapter seeds this private sentinel during init. It is repository
		// plumbing, not a caller-authored TreeStore file.
		if p == rootMarkerPath {
			continue
		}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths, nil
}

// ApplyTreeCommit mirrors ApplyKnowledgeCommit's wip-branch dance (commit.go) but skips
// repofile.Apply entirely: literal bytes at literal paths, no Address, so a
// path that is not knowledge-shaped (no frontmatter) is exactly as writable
// as one that is.
func (r *Repository) ApplyTreeCommit(cs snapshot.TreeChangeSet) (kernel.CommitID, error) {
	if err := r.denyIfArchived(); err != nil {
		return "", err
	}
	if cs.TargetRepository != r.id {
		return "", kernel.Fail(kernel.ErrTargetRepositoryDenied, "target %s does not match %s", cs.TargetRepository, r.id)
	}
	if cs.BaseCommit != cs.ExpectedTargetCommit {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "baseCommit must equal expectedTargetCommit")
	}
	if len(cs.Changes) == 0 {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "raw changeset has no changes")
	}
	targetRef := cs.TargetRef
	if targetRef == "" || targetRef == "HEAD" {
		targetRef = "refs/heads/" + r.branch
	}
	current, ok := r.GetRef(targetRef)
	if !ok {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "ref %s does not exist", targetRef)
	}
	if current != cs.ExpectedTargetCommit {
		return "", kernel.Fail(kernel.ErrNonFastForward, "ref %s moved: expected commit %s, actual %s", targetRef, cs.ExpectedTargetCommit, current)
	}
	_, blobs, err := r.scanAt(cs.ExpectedTargetCommit)
	if err != nil {
		return "", err
	}
	toWrite := map[string]string{}
	toDelete := map[string]struct{}{}
	for _, ch := range cs.Changes {
		clean, err := repofile.SafeRelativePath(ch.Path)
		if err != nil {
			return "", err
		}
		if ch.Remove {
			toDelete[clean] = struct{}{}
			continue
		}
		toWrite[clean] = string(ch.Content)
	}
	files := changeOps(toWrite, toDelete, blobs)
	if len(files) == 0 {
		return current, nil
	}
	name, email, msg := commitSignature(knowledge.ChangeSet{
		Author: cs.Author, Message: cs.Message, RequestID: cs.RequestID, RuleID: cs.RuleID,
	}).Format()
	wip := r.newWipName()
	if err := r.createBranch(wip, cs.ExpectedTargetCommit); err != nil {
		return "", err
	}
	defer r.deleteBranch(wip)
	sha, err := r.changeFiles(wip, files, name, email, msg)
	if err != nil {
		return "", mapWriteErr(err, current)
	}
	if err := r.updateBranch(branchName(targetRef, r.branch), sha, cs.ExpectedTargetCommit); err != nil {
		return "", mapWriteErr(err, current)
	}
	r.invalidate()
	return sha, nil
}
