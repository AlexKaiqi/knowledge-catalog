package gitea

import (
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"

	"kc/internal/gitdir"
	"kc/internal/treepath"
	"kc/kernel"
	"kc/snapshot"
)

// TreeStore (snapshot.TreeStore): literal path read/write at a
// commit, no frontmatter, no object_id. Sibling to the Knowledge methods in
// read.go / commit.go, not a replacement for them. scanAt already fetches
// every blob's SHA (blobs map) regardless of whether it is a knowledge-shaped
// path — this reads the raw immutable tree cache instead of the object_id index.
var _ snapshot.TreeStore = (*Repository)(nil)

func (r *Repository) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	clean, err := treepath.Clean(path)
	if err != nil {
		return nil, err
	}
	directory, name := splitTreePath(clean)
	treeSHA, err := r.resolveDirectoryTree(commit, directory)
	if err != nil {
		return nil, err
	}
	entry, ok, err := r.findTreeEntry(treeSHA, name)
	if err != nil {
		return nil, err
	}
	if !ok || entry.Type != "blob" {
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "path %s is missing at commit %s", path, commit)
	}
	content, err := r.readBlob(entry.SHA)
	if err != nil {
		return nil, err
	}
	return []byte(content), nil
}

func splitTreePath(value string) (string, string) {
	directory, file := path.Split(value)
	return strings.TrimSuffix(directory, "/"), file
}

func (r *Repository) treePage(treeish string, pageNumber, pageSize int) (gitTree, error) {
	query := "?recursive=false&page=" + strconv.Itoa(pageNumber) + "&per_page=" + strconv.Itoa(pageSize)
	var tree gitTree
	status, _, err := r.cli.do(http.MethodGet, r.ep.repoPath("git/trees/"+url.PathEscape(treeish)+query), nil, &tree)
	if missingCommit(status, err) {
		return gitTree{}, kernel.Fail(kernel.ErrVersionUnresolved, "tree %s does not exist", treeish)
	}
	if err != nil {
		return gitTree{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "gitea tree %s: %v", treeish, err)
	}
	return tree, nil
}

func (r *Repository) findTreeEntry(treeish, name string) (gitTreeEntry, bool, error) {
	for pageNumber := 1; ; pageNumber++ {
		tree, err := r.treePage(treeish, pageNumber, 500)
		if err != nil {
			return gitTreeEntry{}, false, err
		}
		for _, entry := range tree.Tree {
			if entry.Path == name {
				return entry, true, nil
			}
		}
		if len(tree.Tree) < 500 {
			return gitTreeEntry{}, false, nil
		}
	}
}

func (r *Repository) resolveDirectoryTree(commit kernel.CommitID, directory string) (string, error) {
	treeish := string(commit)
	if directory == "" {
		return treeish, nil
	}
	clean, err := treepath.Clean(directory)
	if err != nil {
		return "", err
	}
	for _, part := range strings.Split(clean, "/") {
		entry, ok, err := r.findTreeEntry(treeish, part)
		if err != nil {
			return "", err
		}
		if !ok || entry.Type != "tree" {
			return "", kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "directory %s is missing at commit %s", directory, commit)
		}
		treeish = entry.SHA
	}
	return treeish, nil
}

func (r *Repository) ReadDirectory(request snapshot.DirectoryRequest) (snapshot.DirectoryPage, error) {
	directory := strings.Trim(strings.TrimSpace(request.Directory), "/")
	if directory != "" {
		clean, err := treepath.Clean(directory)
		if err != nil {
			return snapshot.DirectoryPage{}, err
		}
		directory = clean
	}
	limit := request.Limit
	if limit == 0 {
		limit = 256
	}
	if limit < 1 || limit > 500 {
		return snapshot.DirectoryPage{}, kernel.Fail(kernel.ErrUsageInvalid, "directory limit must be between 1 and 500")
	}
	cursor, err := snapshot.DecodeDirectoryCursor(request.Continuation, request.Commit, directory)
	if err != nil {
		return snapshot.DirectoryPage{}, err
	}
	treeish, err := r.resolveDirectoryTree(request.Commit, directory)
	if err != nil {
		return snapshot.DirectoryPage{}, err
	}
	entries := make([]snapshot.DirectoryEntry, 0, limit+1)
	for pageNumber := 1; len(entries) <= limit; pageNumber++ {
		tree, pageErr := r.treePage(treeish, pageNumber, 500)
		if pageErr != nil {
			return snapshot.DirectoryPage{}, pageErr
		}
		for _, entry := range tree.Tree {
			if entry.Path <= cursor.Position || (directory == "" && entry.Path == rootMarkerPath) {
				continue
			}
			kind := ""
			switch entry.Type {
			case "blob":
				kind = "file"
			case "tree":
				kind = "directory"
			default:
				continue
			}
			entries = append(entries, snapshot.DirectoryEntry{Name: entry.Path, Kind: kind})
			if len(entries) > limit {
				break
			}
		}
		if len(tree.Tree) < 500 {
			break
		}
	}
	more := len(entries) > limit
	if more {
		entries = entries[:limit]
	}
	next := ""
	if more && len(entries) > 0 {
		next = snapshot.EncodeDirectoryCursor(snapshot.DirectoryCursor{Commit: request.Commit, Directory: directory, Position: entries[len(entries)-1].Name})
	}
	return snapshot.DirectoryPage{Entries: entries, Continuation: next, Exhausted: !more, Generation: string(request.Commit)}, nil
}

func (r *Repository) ListFiles(commit kernel.CommitID) ([]string, error) {
	blobs, err := r.treeAt(commit)
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

// ApplyTreeCommit uses a temporary branch to apply literal path changes with
// ref CAS. It has no Address, frontmatter, or other Knowledge semantics.
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
	blobs, err := r.treeAt(cs.ExpectedTargetCommit)
	if err != nil {
		return "", err
	}
	toWrite := map[string]string{}
	toDelete := map[string]struct{}{}
	for _, ch := range cs.Changes {
		clean, err := treepath.Clean(ch.Path)
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
	name, email, msg := (gitdir.Signature{
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
