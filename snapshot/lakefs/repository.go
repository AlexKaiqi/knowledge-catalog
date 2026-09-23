package lakefs

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
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
	// defaultStageConcurrency bounds concurrent object staging inside one
	// ApplyTreeCommit. Each worker drives the same presigned staging dance the
	// serial path used; commits and publication stay single-threaded.
	defaultStageConcurrency = 32
	// maxTrackedCommits bounds the positive commit-existence cache per
	// repository handle.
	maxTrackedCommits = 4096
)

// stageConcurrency reads the optional staging worker override. Invalid values
// fall back to the default; the effective worker count is capped by the number
// of changes.
func stageConcurrency() int {
	raw := strings.TrimSpace(os.Getenv("KC_LAKEFS_STAGE_CONCURRENCY"))
	if raw == "" {
		return defaultStageConcurrency
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 1 {
		return defaultStageConcurrency
	}
	return parsed
}

// Repository is a lakeFS Snapshot authority. Metadata and refs go through
// lakeFS; object bytes use lakeFS-issued presigned URLs from inside KC Server.
type Repository struct {
	id     kernel.RepositoryID
	ep     Endpoint
	client *apiClient
	branch string
	mu     sync.Mutex
	wip    uint64
	// Positive commit-existence cache. lakeFS commits are immutable and never
	// deleted, so a known commit never needs re-probing; negative results are
	// never cached because a commit may be created concurrently. This holds
	// coordinate identity only — a transport cache, not knowledge semantics.
	verifiedMu   sync.Mutex
	verified     map[kernel.CommitID]struct{}
	verifiedFIFO []kernel.CommitID
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

func (r *Repository) verifiedCommit(commitID kernel.CommitID) bool {
	r.verifiedMu.Lock()
	defer r.verifiedMu.Unlock()
	_, ok := r.verified[commitID]
	return ok
}

func (r *Repository) rememberCommit(commitID kernel.CommitID) {
	r.verifiedMu.Lock()
	defer r.verifiedMu.Unlock()
	if r.verified == nil {
		r.verified = map[kernel.CommitID]struct{}{}
	}
	if _, ok := r.verified[commitID]; ok {
		return
	}
	if len(r.verifiedFIFO) >= maxTrackedCommits {
		evicted := r.verifiedFIFO[0]
		r.verifiedFIFO = r.verifiedFIFO[1:]
		delete(r.verified, evicted)
	}
	r.verified[commitID] = struct{}{}
	r.verifiedFIFO = append(r.verifiedFIFO, commitID)
}

func (r *Repository) HasCommit(commitID kernel.CommitID) bool {
	if commitID == "" {
		return false
	}
	if r.verifiedCommit(commitID) {
		return true
	}
	var commit struct {
		ID string `json:"id"`
	}
	status, _, err := r.client.doJSON(http.MethodGet,
		r.client.repositoryPath("commits/"+url.PathEscape(string(commitID))), nil, &commit)
	if err != nil || status != http.StatusOK || commit.ID != string(commitID) {
		return false
	}
	r.rememberCommit(commitID)
	return true
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

// A directory continuation is bound to the directory that issued it: the
// token carries the issuing directory plus the provider after-key, and a
// replay against any other directory fails closed instead of paging a
// different subtree (docs/reviewed/dataset.md, fixed-range reads).
type directoryContinuation struct {
	Directory string `json:"directory"`
	After     string `json:"after"`
}

func encodeDirectoryContinuation(directory, after string) string {
	raw, err := json.Marshal(directoryContinuation{Directory: directory, After: after})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeDirectoryContinuation(token string) (directoryContinuation, error) {
	var out directoryContinuation
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || json.Unmarshal(raw, &out) != nil {
		return out, kernel.Fail(kernel.ErrPreconditionFailed, "directory continuation is invalid")
	}
	return out, nil
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
	after := ""
	if request.Continuation != "" {
		token, err := decodeDirectoryContinuation(request.Continuation)
		if err != nil {
			return snapshot.DirectoryPage{}, err
		}
		if token.Directory != directory {
			return snapshot.DirectoryPage{}, kernel.Fail(kernel.ErrPreconditionFailed,
				"directory continuation belongs to directory %q, not %q", token.Directory, directory)
		}
		after = token.After
	}
	page, err := r.listObjects(request.Commit, prefix, "/", request.Limit, after)
	if err != nil {
		return snapshot.DirectoryPage{}, err
	}
	out := snapshot.DirectoryPage{
		Continuation: "",
		Exhausted:    !page.Pagination.HasMore,
		Generation:   string(request.Commit),
	}
	if next := page.Pagination.NextOffset; next != "" {
		out.Continuation = encodeDirectoryContinuation(directory, next)
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

func (r *Repository) Origin() string { return r.ep.Origin }

func (r *Repository) String() string {
	return fmt.Sprintf("lakefs repository %s at %s", r.id, r.ep.Origin)
}
