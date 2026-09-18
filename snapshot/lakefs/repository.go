package lakefs

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"kc/internal/treepath"
	"kc/kernel"
	"kc/snapshot"
)

const (
	defaultBranch  = "main"
	archivedBranch = "kc-archived"
)

// Repository is a lakeFS Snapshot authority. Metadata and refs go through
// lakeFS; object bytes use lakeFS-issued presigned URLs from inside KC Server.
type Repository struct {
	id     kernel.RepositoryID
	ep     Endpoint
	client *apiClient
	branch string
	mu     sync.Mutex
	wip    uint64
}

var (
	_ snapshot.Store           = (*Repository)(nil)
	_ snapshot.TreeStore       = (*Repository)(nil)
	_ snapshot.DirectoryReader = (*Repository)(nil)
	_ snapshot.HistoryStore    = (*Repository)(nil)
	_ snapshot.ChangeStore     = (*Repository)(nil)
)

// OpenExisting attaches an initialized lakeFS repository. Published branches
// must be writable only by KC's service identity: the adapter serializes
// publication with an atomic hidden lock branch before using hard_reset.
func OpenExisting(id kernel.RepositoryID, dsn, credential string) (*Repository, error) {
	if strings.TrimSpace(string(id)) == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "repository id is required")
	}
	ep, err := ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	if credential == "" {
		credential = os.Getenv(EnvCredential)
	}
	client, err := newAPIClient(ep, credential)
	if err != nil {
		return nil, err
	}
	var info struct {
		DefaultBranch string `json:"default_branch"`
	}
	if _, _, err := client.doJSON(http.MethodGet, client.repositoryPath(""), nil, &info); err != nil {
		return nil, err
	}
	r := &Repository{id: id, ep: ep, client: client, branch: defaultBranch}
	if info.DefaultBranch != "" {
		r.branch = info.DefaultBranch
	}
	if _, err := r.Head(snapshot.DefaultRef); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Repository) ID() kernel.RepositoryID { return r.id }

func (r *Repository) branchName(ref string) (string, bool) {
	switch {
	case ref == "" || ref == "HEAD" || ref == snapshot.DefaultRef:
		return r.branch, true
	case strings.HasPrefix(ref, "refs/heads/"):
		name := strings.TrimPrefix(ref, "refs/heads/")
		if name == "" {
			return "", false
		}
		return encodeLakeFSBranch(name), true
	default:
		return "", false
	}
}

// encodeLakeFSBranch maps a KC branch name onto lakeFS's id alphabet
// (letters, digits, underscore, dash). Nested candidate refs such as
// candidates/PR-1 become candidates--PR-1.
func encodeLakeFSBranch(name string) string {
	return strings.ReplaceAll(name, "/", "--")
}

type refResponse struct {
	ID       string `json:"id"`
	CommitID string `json:"commit_id"`
}

func (r *Repository) getBranch(name string) (kernel.CommitID, bool, error) {
	var out refResponse
	status, _, err := r.client.doJSON(http.MethodGet,
		r.client.repositoryPath("branches/"+url.PathEscape(name)), nil, &out)
	if status == http.StatusNotFound {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if out.CommitID == "" {
		return "", false, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs branch %s omitted commit", name)
	}
	return kernel.CommitID(out.CommitID), true, nil
}

func (r *Repository) Head(ref string) (kernel.CommitID, error) {
	name, ok := r.branchName(ref)
	if !ok {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "ref %s does not exist", snapshot.RefOrDefault(ref))
	}
	commit, found, err := r.getBranch(name)
	if err != nil {
		return "", err
	}
	if !found {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "ref %s does not exist", snapshot.RefOrDefault(ref))
	}
	return commit, nil
}

func (r *Repository) GetRef(ref string) (kernel.CommitID, bool) {
	name, ok := r.branchName(ref)
	if !ok {
		return "", false
	}
	commit, found, err := r.getBranch(name)
	return commit, found && err == nil
}

func (r *Repository) HasCommit(commitID kernel.CommitID) bool {
	if commitID == "" {
		return false
	}
	var commit struct {
		ID string `json:"id"`
	}
	status, _, err := r.client.doJSON(http.MethodGet,
		r.client.repositoryPath("commits/"+url.PathEscape(string(commitID))), nil, &commit)
	return err == nil && status == http.StatusOK && commit.ID == string(commitID)
}

func (r *Repository) CreateRef(ref string, commitID kernel.CommitID) error {
	if err := r.denyIfArchived(); err != nil {
		return err
	}
	name, ok := r.branchName(ref)
	if !ok {
		return kernel.Fail(kernel.ErrUsageInvalid, "lakefs supports full branch refs only")
	}
	if !r.HasCommit(commitID) {
		return kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commitID)
	}
	body := struct {
		Name   string `json:"name"`
		Source string `json:"source"`
		Hidden bool   `json:"hidden,omitempty"`
	}{Name: name, Source: string(commitID)}
	status, _, err := r.client.doJSON(http.MethodPost, r.client.repositoryPath("branches"), body, nil)
	if status == http.StatusConflict {
		return kernel.Fail(kernel.ErrPreconditionFailed, "ref %s already exists", ref)
	}
	return err
}

func (r *Repository) isAncestor(expected, candidate kernel.CommitID) (bool, error) {
	after := ""
	for {
		query := url.Values{"first_parent": {"true"}}
		addPage(query, maxPageSize, after)
		var page struct {
			Pagination pagination `json:"pagination"`
			Results    []struct {
				ID string `json:"id"`
			} `json:"results"`
		}
		endpoint := r.client.repositoryPath("refs/"+url.PathEscape(string(candidate))+"/commits") + "?" + query.Encode()
		if _, _, err := r.client.doJSON(http.MethodGet, endpoint, nil, &page); err != nil {
			if statusOf(err) == http.StatusNotFound {
				return false, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", candidate)
			}
			return false, err
		}
		for _, commit := range page.Results {
			if kernel.CommitID(commit.ID) == expected {
				return true, nil
			}
		}
		if !page.Pagination.HasMore {
			return false, nil
		}
		if page.Pagination.NextOffset == "" || page.Pagination.NextOffset == after {
			return false, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs history returned an invalid continuation")
		}
		after = page.Pagination.NextOffset
	}
}

func (r *Repository) publicationLock(name string) string {
	digest := string(kernel.CanonicalDigest(string(r.id) + "\x00" + name))
	return "kc-publish-lock-" + digest[:24]
}

// publishBranch implements expected-old publication using only stock lakeFS
// operations. Atomic branch creation is the mutex; while holding it, KC checks
// expected, hard-resets the clean publication branch, verifies, then releases.
// Deployment policy must deny direct writers on publication branches.
func (r *Repository) publishBranch(name string, candidate, expected kernel.CommitID) error {
	lock := r.publicationLock(name)
	if err := r.createBranch(lock, candidate, true); err != nil {
		if statusOf(err) != http.StatusConflict {
			return err
		}
		current, found, readErr := r.getBranch(name)
		if readErr != nil {
			return readErr
		}
		if !found {
			return kernel.Fail(kernel.ErrVersionUnresolved, "ref refs/heads/%s does not exist", name)
		}
		lockHead, lockFound, lockErr := r.getBranch(lock)
		if lockErr != nil {
			return lockErr
		}
		if current == candidate {
			if lockFound && lockHead == candidate {
				r.deleteBranch(lock) // completed publisher crashed before cleanup.
			}
			return nil
		}
		if current != expected {
			if lockFound && current == lockHead {
				r.deleteBranch(lock)
			}
			return kernel.Fail(kernel.ErrNonFastForward,
				"ref refs/heads/%s moved: expected commit %s, actual %s", name, expected, current)
		}
		return kernel.Fail(kernel.ErrTemporaryUnavailable,
			"lakefs publication lock for refs/heads/%s is held", name)
	}
	defer r.deleteBranch(lock)

	current, found, err := r.getBranch(name)
	if err != nil {
		return err
	}
	if !found {
		return kernel.Fail(kernel.ErrVersionUnresolved, "ref refs/heads/%s does not exist", name)
	}
	if current == candidate {
		return nil
	}
	if current != expected {
		return kernel.Fail(kernel.ErrNonFastForward,
			"ref refs/heads/%s moved: expected commit %s, actual %s", name, expected, current)
	}
	query := url.Values{"ref": {string(candidate)}, "force": {"false"}}
	endpoint := r.client.repositoryPath("branches/"+url.PathEscape(name)+"/hard_reset") + "?" + query.Encode()
	if _, _, err := r.client.doJSON(http.MethodPut, endpoint, nil, nil); err != nil {
		return err
	}
	actual, found, err := r.getBranch(name)
	if err != nil {
		return err
	}
	if !found || actual != candidate {
		return kernel.Fail(kernel.ErrTemporaryUnavailable,
			"lakefs publication verification failed for refs/heads/%s", name)
	}
	return nil
}

func (r *Repository) Merge(targetRef string, candidate, expected kernel.CommitID) (kernel.CommitID, error) {
	if err := r.denyIfArchived(); err != nil {
		return "", err
	}
	name, ok := r.branchName(targetRef)
	if !ok {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "lakefs merge target must be a full branch ref")
	}
	current, found, err := r.getBranch(name)
	if err != nil {
		return "", err
	}
	if !found {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "ref %s does not exist", targetRef)
	}
	if current != expected {
		return "", kernel.Fail(kernel.ErrNonFastForward,
			"ref %s moved: expected commit %s, actual %s", targetRef, expected, current)
	}
	if candidate == expected {
		return candidate, nil
	}
	ancestor, err := r.isAncestor(expected, candidate)
	if err != nil {
		return "", err
	}
	if !ancestor {
		return "", kernel.Fail(kernel.ErrNonFastForward,
			"candidate %s is not a descendant of %s", candidate, expected)
	}
	if err := r.publishBranch(name, candidate, expected); err != nil {
		return "", err
	}
	return candidate, nil
}

func (r *Repository) Archived() bool {
	_, found, err := r.getBranch(archivedBranch)
	return err == nil && found
}

func (r *Repository) Archive() error {
	if r.Archived() {
		return nil
	}
	head, err := r.Head(snapshot.DefaultRef)
	if err != nil {
		return err
	}
	body := struct {
		Name   string `json:"name"`
		Source string `json:"source"`
		Hidden bool   `json:"hidden,omitempty"`
	}{Name: archivedBranch, Source: string(head), Hidden: true}
	status, _, err := r.client.doJSON(http.MethodPost, r.client.repositoryPath("branches"), body, nil)
	if status == http.StatusConflict {
		return nil
	}
	return err
}

func (r *Repository) denyIfArchived() error {
	if r.Archived() {
		return kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", r.id)
	}
	return nil
}

func (r *Repository) ReadFile(filePath string, commit kernel.CommitID) ([]byte, error) {
	clean, err := treepath.Clean(filePath)
	if err != nil {
		return nil, err
	}
	if !r.HasCommit(commit) {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	content, err := r.client.getObject(string(commit), clean)
	if statusOf(err) == http.StatusNotFound {
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved,
			"path %s is missing at commit %s", clean, commit)
	}
	return content, err
}

func (r *Repository) listObjects(commit kernel.CommitID, prefix, delimiter string, limit int, continuation string) (objectList, error) {
	query := url.Values{"prefix": {prefix}, "user_metadata": {"false"}}
	if delimiter != "" {
		query.Set("delimiter", delimiter)
	}
	addPage(query, limit, continuation)
	var out objectList
	endpoint := r.client.repositoryPath("refs/"+url.PathEscape(string(commit))+"/objects/ls") + "?" + query.Encode()
	_, _, err := r.client.doJSON(http.MethodGet, endpoint, nil, &out)
	return out, err
}

func (r *Repository) ListFiles(commit kernel.CommitID) ([]string, error) {
	if !r.HasCommit(commit) {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	var files []string
	continuation := ""
	for {
		page, err := r.listObjects(commit, "", "", maxPageSize, continuation)
		if err != nil {
			return nil, err
		}
		for _, item := range page.Results {
			if item.PathType == "object" {
				files = append(files, item.Path)
			}
		}
		if !page.Pagination.HasMore {
			sort.Strings(files)
			return files, nil
		}
		if page.Pagination.NextOffset == "" || page.Pagination.NextOffset == continuation {
			return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs object listing returned an invalid continuation")
		}
		continuation = page.Pagination.NextOffset
	}
}

func (r *Repository) ReadDirectory(request snapshot.DirectoryRequest) (snapshot.DirectoryPage, error) {
	if !r.HasCommit(request.Commit) {
		return snapshot.DirectoryPage{}, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", request.Commit)
	}
	directory := strings.Trim(strings.TrimSpace(request.Directory), "/")
	prefix := ""
	if directory != "" {
		prefix = directory + "/"
	}
	page, err := r.listObjects(request.Commit, prefix, "/", request.Limit, request.Continuation)
	if err != nil {
		return snapshot.DirectoryPage{}, err
	}
	out := snapshot.DirectoryPage{
		Continuation: page.Pagination.NextOffset,
		Exhausted:    !page.Pagination.HasMore,
		Generation:   string(request.Commit),
	}
	for _, item := range page.Results {
		name := strings.TrimSuffix(strings.TrimPrefix(item.Path, prefix), "/")
		if name == "" || strings.Contains(name, "/") {
			continue
		}
		kind := "file"
		if item.PathType == "common_prefix" {
			kind = "directory"
		}
		out.Entries = append(out.Entries, snapshot.DirectoryEntry{Name: name, Kind: kind})
	}
	sort.Slice(out.Entries, func(i, j int) bool { return out.Entries[i].Name < out.Entries[j].Name })
	return out, nil
}

func (r *Repository) nextWIP(cs snapshot.TreeChangeSet) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wip++
	seed := string(r.id) + "\x00" + cs.RequestID + "\x00" + string(cs.BaseCommit) + "\x00" + strconv.FormatUint(r.wip, 10)
	return "kc-wip-" + string(kernel.CanonicalDigest(seed))[:24]
}

func (r *Repository) createBranch(name string, source kernel.CommitID, hidden bool) error {
	body := struct {
		Name   string `json:"name"`
		Source string `json:"source"`
		Hidden bool   `json:"hidden,omitempty"`
	}{Name: name, Source: string(source), Hidden: hidden}
	_, _, err := r.client.doJSON(http.MethodPost, r.client.repositoryPath("branches"), body, nil)
	return err
}

func (r *Repository) deleteBranch(name string) {
	_, _, _ = r.client.doJSON(http.MethodDelete,
		r.client.repositoryPath("branches/"+url.PathEscape(name))+"?force=true", nil, nil)
}

func (r *Repository) deleteObject(branch, objectPath string) error {
	endpoint := r.client.repositoryPath("branches/"+url.PathEscape(branch)+"/objects") +
		"?path=" + url.QueryEscape(objectPath)
	_, _, err := r.client.doJSON(http.MethodDelete, endpoint, nil, nil)
	return err
}

func (r *Repository) commit(branch string, cs snapshot.TreeChangeSet) (kernel.CommitID, error) {
	message := strings.TrimSpace(cs.Message)
	if message == "" {
		message = "Knowledge Catalog update"
	}
	metadata := map[string]string{}
	if cs.RequestID != "" {
		metadata["kc.command_id"] = cs.RequestID
	}
	if cs.RuleID != "" {
		metadata["kc.rule_id"] = cs.RuleID
	}
	body := struct {
		Message  string            `json:"message"`
		Metadata map[string]string `json:"metadata,omitempty"`
	}{Message: message, Metadata: metadata}
	var out struct {
		ID string `json:"id"`
	}
	if _, _, err := r.client.doJSON(http.MethodPost,
		r.client.repositoryPath("branches/"+url.PathEscape(branch)+"/commits"), body, &out); err != nil {
		return "", err
	}
	if out.ID == "" {
		return "", kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs commit omitted id")
	}
	return kernel.CommitID(out.ID), nil
}

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
	target := cs.TargetRef
	if target == "" || target == "HEAD" {
		target = snapshot.DefaultRef
	}
	targetBranch, ok := r.branchName(target)
	if !ok {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "lakefs write target must be a full branch ref")
	}
	current, found, err := r.getBranch(targetBranch)
	if err != nil {
		return "", err
	}
	if !found {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "ref %s does not exist", target)
	}
	if current != cs.ExpectedTargetCommit {
		return "", kernel.Fail(kernel.ErrNonFastForward,
			"ref %s moved: expected commit %s, actual %s", target, cs.ExpectedTargetCommit, current)
	}
	wip := r.nextWIP(cs)
	if err := r.createBranch(wip, cs.BaseCommit, true); err != nil {
		return "", err
	}
	defer r.deleteBranch(wip)
	for _, change := range cs.Changes {
		clean, err := treepath.Clean(change.Path)
		if err != nil {
			return "", err
		}
		if change.Remove {
			if err := r.deleteObject(wip, clean); err != nil {
				return "", err
			}
			continue
		}
		if err := r.client.stage(wip, clean, change.Content); err != nil {
			return "", err
		}
	}
	candidate, err := r.commit(wip, cs)
	if err != nil {
		return "", err
	}
	if err := r.publishBranch(targetBranch, candidate, cs.ExpectedTargetCommit); err != nil {
		return "", err
	}
	return candidate, nil
}

func (r *Repository) CommitHistory(commit kernel.CommitID, limit int) ([]kernel.CommitID, error) {
	if !r.HasCommit(commit) {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	query := url.Values{"first_parent": {"true"}, "limit": {"true"}}
	addPage(query, limit, "")
	var page struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	endpoint := r.client.repositoryPath("refs/"+url.PathEscape(string(commit))+"/commits") + "?" + query.Encode()
	if _, _, err := r.client.doJSON(http.MethodGet, endpoint, nil, &page); err != nil {
		return nil, err
	}
	out := make([]kernel.CommitID, 0, len(page.Results))
	for _, item := range page.Results {
		out = append(out, kernel.CommitID(item.ID))
	}
	return out, nil
}

func (r *Repository) ChangedPaths(from, to kernel.CommitID) ([]string, error) {
	if from == "" || to == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "changed paths require from and to commits")
	}
	if from == to {
		return []string{}, nil
	}
	seen := map[string]struct{}{}
	continuation := ""
	for {
		query := url.Values{"type": {"two_dot"}}
		addPage(query, maxPageSize, continuation)
		var page struct {
			Pagination pagination `json:"pagination"`
			Results    []struct {
				Path string `json:"path"`
			} `json:"results"`
		}
		endpoint := r.client.repositoryPath("refs/"+url.PathEscape(string(from))+"/diff/"+
			url.PathEscape(string(to))) + "?" + query.Encode()
		if _, _, err := r.client.doJSON(http.MethodGet, endpoint, nil, &page); err != nil {
			if statusOf(err) == http.StatusNotFound {
				return nil, kernel.Fail(kernel.ErrVersionUnresolved, "changed-path basis does not exist")
			}
			return nil, err
		}
		for _, item := range page.Results {
			if item.Path != "" {
				seen[path.Clean(item.Path)] = struct{}{}
			}
		}
		if !page.Pagination.HasMore {
			break
		}
		if page.Pagination.NextOffset == "" || page.Pagination.NextOffset == continuation {
			return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs diff returned an invalid continuation")
		}
		continuation = page.Pagination.NextOffset
	}
	out := make([]string, 0, len(seen))
	for item := range seen {
		out = append(out, item)
	}
	sort.Strings(out)
	return out, nil
}

func (r *Repository) String() string {
	return fmt.Sprintf("lakefs repository %s at %s", r.id, r.ep.Origin)
}
