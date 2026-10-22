package lakefsimpl

// Write-path machinery for the lakeFS authority: WIP branches, tree apply,
// commit publication and history/diff reads. Read-side plumbing stays in
// repository.go; both files share one Repository type.

import (
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"kc/internal/treepath"
	"kc/kernel"
	"kc/snapshot/internal/storecontract"
)

func (r *Repository) nextWIP(cs storecontract.TreeChangeSet) string {
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

func (r *Repository) commit(branch string, cs storecontract.TreeChangeSet) (kernel.CommitID, error) {
	message := strings.TrimSpace(cs.Message)
	if message == "" {
		message = "Knowledge Catalog update"
	}
	metadata := map[string]string{}
	if cs.Author != "" {
		metadata["kc.author"] = cs.Author
	}
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

// objectWriter is the per-object transport primitive used while applying one
// changeset: presigned staging for the default path, direct object upload for
// the bulk path.
type objectWriter func(branch, objectPath string, content []byte) error

func (r *Repository) ApplyTreeCommit(cs storecontract.TreeChangeSet) (kernel.CommitID, error) {
	return r.applyTreeCommit(cs, r.client.stage)
}

// ApplyTreeCommitBulk is the BulkTreeIngester capability: one apply through
// the lakeFS object-upload data plane (single round trip per object, bytes
// cross the lakeFS server) for many-small-object commits. CAS, wip isolation,
// commit and publication semantics equal ApplyTreeCommit.
func (r *Repository) ApplyTreeCommitBulk(cs storecontract.TreeChangeSet) (kernel.CommitID, error) {
	return r.applyTreeCommit(cs, r.client.uploadObject)
}

func (r *Repository) applyTreeCommit(cs storecontract.TreeChangeSet, write objectWriter) (_ kernel.CommitID, err error) {
	publicationStarted := false
	defer func() {
		if err != nil && !publicationStarted {
			err = storecontract.WriteNotApplied(err)
		}
	}()
	targetBranch, err := r.checkTreeWriteTarget(cs)
	if err != nil {
		return "", err
	}
	wip := r.nextWIP(cs)
	if err := r.createBranch(wip, cs.BaseCommit, true); err != nil {
		return "", err
	}
	defer r.deleteBranch(wip)
	if err := r.applyChanges(wip, cs.Changes, write); err != nil {
		return "", err
	}
	candidate, err := r.commit(wip, cs)
	if err != nil {
		return "", err
	}
	publicationStarted = true
	if err := r.publishBranch(targetBranch, candidate, cs.ExpectedTargetCommit); err != nil {
		return "", err
	}
	return candidate, nil
}

func (r *Repository) checkTreeWriteTarget(cs storecontract.TreeChangeSet) (string, error) {
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
		target = storecontract.DefaultRef
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
	return targetBranch, nil
}

// applyChanges writes the literal tree changes onto the wip branch through the
// changeset's transport primitive. Object staging is independent per path, so
// it fans out over a bounded worker pool; the first failure stops new work and
// is returned after in-flight writes finish. Commit and publication stay
// single-threaded: lakeFS serializes commits per branch, and the publication
// lock protocol relies on that.
func (r *Repository) applyChanges(branch string, changes []storecontract.TreeChange, write objectWriter) error {
	workers := stageConcurrency()
	if workers > len(changes) {
		workers = len(changes)
	}
	if workers <= 1 {
		for _, change := range changes {
			if err := r.applyChangeWith(write, branch, change); err != nil {
				return err
			}
		}
		return nil
	}
	var (
		next     int64
		mu       sync.Mutex
		firstErr error
		wg       sync.WaitGroup
	)
	wg.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wg.Done()
			for {
				index := int(atomic.AddInt64(&next, 1)) - 1
				if index >= len(changes) {
					return
				}
				mu.Lock()
				abort := firstErr != nil
				mu.Unlock()
				if abort {
					return
				}
				if err := r.applyChangeWith(write, branch, changes[index]); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					return
				}
			}
		}()
	}
	wg.Wait()
	return firstErr
}

func (r *Repository) applyChangeWith(write objectWriter, branch string, change storecontract.TreeChange) error {
	clean, err := treepath.Clean(change.Path)
	if err != nil {
		return err
	}
	if change.Remove {
		return r.deleteObject(branch, clean)
	}
	return write(branch, clean, change.Content)
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

// Origin reports the authority-instance coordinate of this repository (the
// lakeFS endpoint this handle is bound to, without credentials).
