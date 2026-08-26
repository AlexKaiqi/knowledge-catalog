package gitea

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"kc/internal/gitdir"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

const (
	archivedRef    = "refs/kc/archived"
	archivedTag    = "kc-archived"
	archivedBranch = "kc-archived"
	defaultBranch  = "main"
	rootMarkerPath = ".kc/root"
	rootMarkerBody = "knowledge-catalog\n"
)

// Repository is a Gitea-hosted snapshot.Store with optional Knowledge capability.
// It has no worktree and no RootDir.
type Repository struct {
	id     kernel.RepositoryID
	ep     Endpoint
	token  string
	cli    *client
	branch string
	mu     sync.Mutex
	wip    int
	// trees and blobBodies cache only immutable Gitea transport results. Parsed
	// knowledge trees belong to layer ② and are deliberately not retained here.
	trees      map[kernel.CommitID]map[string]string
	blobBodies map[string]string
}

var (
	_ snapshot.Store       = (*Repository)(nil)
	_ snapshot.TreeStore   = (*Repository)(nil)
	_ knowledge.Repository = (*Repository)(nil)
)

// Open attaches (or creates) a Gitea repository as a Catalog member Snapshot.
// Token is KC_GITEA_TOKEN when empty.
func Open(id kernel.RepositoryID, dsn, token string) (*Repository, error) {
	ep, err := ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	if token == "" {
		token = strings.TrimSpace(os.Getenv(EnvToken))
	}
	if token == "" {
		return nil, fmt.Errorf("gitea token missing; set %s", EnvToken)
	}
	r := &Repository{
		id:     id,
		ep:     ep,
		token:  token,
		cli:    newClient(ep.API, token),
		branch: defaultBranch,
	}
	r.resetCache()
	if err := r.ensureRepo(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Repository) ID() kernel.RepositoryID { return r.id }

func (r *Repository) resetCache() {
	r.trees = map[kernel.CommitID]map[string]string{}
	r.blobBodies = map[string]string{}
}

func (r *Repository) invalidate() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resetCache()
}

func (r *Repository) ensureRepo() error {
	var info repoInfo
	status, _, err := r.cli.do(http.MethodGet, r.ep.repoPath(""), nil, &info)
	if status == http.StatusOK {
		if info.DefaultBranch != "" {
			r.branch = info.DefaultBranch
		}
		if info.Empty || !r.hasBranch(r.branch) {
			return r.initMain()
		}
		return nil
	}
	if status != http.StatusNotFound {
		if err != nil {
			return err
		}
		return fmt.Errorf("gitea GET %s: HTTP %d", r.ep.repoPath(""), status)
	}
	var me userInfo
	if _, _, err := r.cli.do(http.MethodGet, "/user", nil, &me); err != nil {
		return err
	}
	body := createRepoBody{Name: r.ep.Name, Private: true, AutoInit: true, DefaultBranch: defaultBranch}
	createPath := "/user/repos"
	if me.Login != "" && me.Login != r.ep.Owner {
		createPath = "/orgs/" + url.PathEscape(r.ep.Owner) + "/repos"
	}
	if _, _, err := r.cli.do(http.MethodPost, createPath, body, &info); err != nil {
		return err
	}
	r.branch = defaultBranch
	if info.DefaultBranch != "" {
		r.branch = info.DefaultBranch
	}
	if info.Empty || !r.hasBranch(r.branch) {
		return r.initMain()
	}
	return nil
}

func (r *Repository) hasBranch(name string) bool {
	_, ok := r.lookupBranch(name)
	return ok
}

func (r *Repository) initMain() error {
	name, email, msg := gitdir.Signature{Message: "root"}.Format()
	body := changeFilesBody{
		Message:   msg,
		Branch:    r.branch,
		NewBranch: r.branch,
		Author:    gitIdentity{Name: name, Email: email},
		Committer: gitIdentity{Name: name, Email: email},
		Files: []changeFileOp{{
			Operation: "create",
			Path:      rootMarkerPath,
			Content:   base64.StdEncoding.EncodeToString([]byte(rootMarkerBody)),
		}},
	}
	var out filesResponse
	if _, _, err := r.cli.do(http.MethodPost, r.ep.repoPath("contents"), body, &out); err != nil {
		return err
	}
	r.invalidate()
	return nil
}

func (r *Repository) Head(ref string) (kernel.CommitID, error) {
	if ref == "" {
		ref = "HEAD"
	}
	commit, ok := r.GetRef(ref)
	if !ok {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "ref %s does not exist", ref)
	}
	return commit, nil
}

func (r *Repository) GetRef(ref string) (kernel.CommitID, bool) {
	if ref == "" || ref == "HEAD" {
		ref = "refs/heads/" + r.branch
	}
	if ref == archivedRef {
		if sha, ok := r.lookupTag(archivedTag); ok {
			return sha, true
		}
		return r.lookupBranch(archivedBranch)
	}
	if strings.HasPrefix(ref, "refs/heads/") {
		return r.lookupBranch(strings.TrimPrefix(ref, "refs/heads/"))
	}
	if strings.HasPrefix(ref, "refs/tags/") {
		return r.lookupTag(strings.TrimPrefix(ref, "refs/tags/"))
	}
	return r.getGitRef(ref)
}

func (r *Repository) lookupBranch(name string) (kernel.CommitID, bool) {
	if name == "" {
		return "", false
	}
	var b branchInfo
	status, _, err := r.cli.do(http.MethodGet, r.ep.repoPath("branches/"+encodeRefPath(name)), nil, &b)
	if err != nil || status != http.StatusOK {
		return "", false
	}
	sha := b.commitID()
	if sha == "" {
		return "", false
	}
	return sha, true
}

func (r *Repository) lookupTag(name string) (kernel.CommitID, bool) {
	if name == "" {
		return "", false
	}
	var t tagInfo
	status, _, err := r.cli.do(http.MethodGet, r.ep.repoPath("tags/"+encodeRefPath(name)), nil, &t)
	if err != nil || status != http.StatusOK {
		return "", false
	}
	sha := t.commitID()
	if sha == "" {
		return "", false
	}
	return sha, true
}

func (r *Repository) getGitRef(ref string) (kernel.CommitID, bool) {
	trimmed := strings.TrimPrefix(ref, "refs/")
	var single gitRef
	status, raw, err := r.cli.do(http.MethodGet, r.ep.repoPath("git/refs/"+encodeRefPath(trimmed)), nil, &single)
	if err != nil || status != http.StatusOK {
		return "", false
	}
	if single.Object.SHA != "" {
		return kernel.CommitID(single.Object.SHA), true
	}
	var many []gitRef
	if json.Unmarshal(raw, &many) == nil {
		for _, item := range many {
			if item.Ref == ref || item.Ref == "refs/"+trimmed {
				if item.Object.SHA != "" {
					return kernel.CommitID(item.Object.SHA), true
				}
			}
		}
	}
	return "", false
}

func (r *Repository) HasCommit(commitID kernel.CommitID) bool {
	if commitID == "" {
		return false
	}
	escaped := url.PathEscape(string(commitID))
	status, _, err := r.cli.do(http.MethodGet, r.ep.repoPath("commits/"+escaped), nil, &commitRow{})
	if err == nil && status == http.StatusOK {
		return true
	}
	status, _, err = r.cli.do(http.MethodGet, r.ep.repoPath("git/commits/"+escaped), nil, &gitCommit{})
	return err == nil && status == http.StatusOK
}

func (r *Repository) CreateRef(ref string, commitID kernel.CommitID) error {
	if err := r.denyIfArchived(); err != nil && ref != archivedRef && ref != "refs/heads/"+archivedBranch && ref != "refs/tags/"+archivedTag {
		return err
	}
	if _, ok := r.GetRef(ref); ok {
		return kernel.Fail(kernel.ErrPreconditionFailed, "ref %s already exists", ref)
	}
	if !r.HasCommit(commitID) {
		return kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commitID)
	}
	if ref == archivedRef || ref == "refs/tags/"+archivedTag {
		return r.createTag(archivedTag, commitID)
	}
	if strings.HasPrefix(ref, "refs/tags/") {
		return r.createTag(strings.TrimPrefix(ref, "refs/tags/"), commitID)
	}
	name := strings.TrimPrefix(ref, "refs/heads/")
	if name == ref {
		name = ref
	}
	return r.createBranch(name, commitID)
}

func (r *Repository) createBranch(name string, commitID kernel.CommitID) error {
	body := createBranchBody{NewBranchName: name, OldRefName: string(commitID)}
	_, _, err := r.cli.do(http.MethodPost, r.ep.repoPath("branches"), body, nil)
	return err
}

func (r *Repository) deleteBranch(name string) {
	_, _, _ = r.cli.do(http.MethodDelete, r.ep.repoPath("branches/"+encodeRefPath(name)), nil, nil)
}

func (r *Repository) createTag(name string, commitID kernel.CommitID) error {
	body := createTagBody{TagName: name, Message: "archived", Target: string(commitID)}
	_, _, err := r.cli.do(http.MethodPost, r.ep.repoPath("tags"), body, nil)
	return err
}

func (r *Repository) Merge(targetRef string, candidate, expected kernel.CommitID) (kernel.CommitID, error) {
	if err := r.denyIfArchived(); err != nil {
		return "", err
	}
	current, ok := r.GetRef(targetRef)
	if !ok {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "ref %s does not exist", targetRef)
	}
	if current != expected {
		return "", kernel.Fail(kernel.ErrNonFastForward, "ref %s moved: expected commit %s, actual %s", targetRef, expected, current)
	}
	if candidate == expected {
		return candidate, nil
	}
	if !r.isAncestor(expected, candidate) {
		return "", kernel.Fail(kernel.ErrNonFastForward, "commit %s is not a descendant of %s", candidate, expected)
	}
	if err := r.updateBranch(branchName(targetRef, r.branch), candidate, expected); err != nil {
		cur, _ := r.GetRef(targetRef)
		return "", kernel.Fail(kernel.ErrNonFastForward, "ref %s moved: expected commit %s, actual %s", targetRef, expected, cur)
	}
	r.invalidate()
	return candidate, nil
}

func (r *Repository) updateBranch(name string, sha, expected kernel.CommitID) error {
	body := updateBranchBody{NewCommitID: string(sha), OldCommitID: string(expected)}
	_, _, err := r.cli.do(http.MethodPut, r.ep.repoPath("branches/"+encodeRefPath(name)), body, nil)
	return err
}

func (r *Repository) isAncestor(base, head kernel.CommitID) bool {
	if base == head {
		return true
	}
	seen := map[kernel.CommitID]struct{}{}
	var walk func(kernel.CommitID) bool
	walk = func(sha kernel.CommitID) bool {
		if sha == base {
			return true
		}
		if _, ok := seen[sha]; ok {
			return false
		}
		seen[sha] = struct{}{}
		if len(seen) > 256 {
			return false
		}
		var c gitCommit
		escaped := url.PathEscape(string(sha))
		if _, _, err := r.cli.do(http.MethodGet, r.ep.repoPath("git/commits/"+escaped), nil, &c); err != nil {
			if _, _, err = r.cli.do(http.MethodGet, r.ep.repoPath("commits/"+escaped), nil, &c); err != nil {
				return false
			}
		}
		for _, p := range c.Parents {
			parent := kernel.CommitID(p.SHA)
			if parent == "" {
				parent = kernel.CommitID(p.ID)
			}
			if walk(parent) {
				return true
			}
		}
		return false
	}
	return walk(head)
}

func (r *Repository) Archived() bool {
	_, ok := r.GetRef(archivedRef)
	return ok
}

func (r *Repository) Archive() error {
	if r.Archived() {
		return nil
	}
	head, err := r.Head("refs/heads/" + r.branch)
	if err != nil {
		return err
	}
	if err := r.CreateRef(archivedRef, head); err == nil {
		return nil
	}
	return r.CreateRef("refs/heads/"+archivedBranch, head)
}

func (r *Repository) denyIfArchived() error {
	if r.Archived() {
		return kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", r.id)
	}
	return nil
}

func encodeRefPath(ref string) string {
	parts := strings.Split(ref, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}
