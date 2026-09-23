package lakefs_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/lakefs"
)

type fakeCommit struct {
	parent string
	files  map[string][]byte
}

type fakeLakeFS struct {
	mu           sync.Mutex
	server       *httptest.Server
	next         int
	commits      map[string]fakeCommit
	branches     map[string]string
	staged       map[string]map[string]*[]byte
	uploads      map[string][]byte
	directWrites int
	directReads  int
	// blobInFlight/blobMaxInFlight track concurrent presigned PUTs without
	// holding mu; blobSleep lets a test widen the overlap window.
	blobSleep       time.Duration
	blobInFlight    int32
	blobMaxInFlight int32
	// commitLookups counts GET /commits/{id} probes (existence checks).
	commitLookups int
}

func newFakeLakeFS(t *testing.T) *fakeLakeFS {
	t.Helper()
	f := &fakeLakeFS{
		commits:  map[string]fakeCommit{"c0": {files: map[string][]byte{}}},
		branches: map[string]string{"main": "c0"},
		staged:   map[string]map[string]*[]byte{},
		uploads:  map[string][]byte{},
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.serveHTTP))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeLakeFS) open(t *testing.T, id string) *lakefs.Repository {
	t.Helper()
	repo, err := lakefs.OpenExisting(kernel.RepositoryID(id), f.server.URL+"/knowledge", "access:secret")
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestLakeFSRepositoryContract(t *testing.T) {
	testkit.RepositoryContract(t, func(t *testing.T, id string) *lakefs.Repository {
		return newFakeLakeFS(t).open(t, id)
	})
}

func TestLakeFSWriterContract(t *testing.T) {
	testkit.WriterContract(t, func(t *testing.T, id string) *lakefs.Repository {
		return newFakeLakeFS(t).open(t, id)
	})
}

func TestLakeFSMatchesTreeProviderByOperationStep(t *testing.T) {
	testkit.ProviderParityContract(t,
		func(t *testing.T, id string) snapshot.Store { return testkit.MakeRepository(t, id) },
		func(t *testing.T, id string) snapshot.Store { return newFakeLakeFS(t).open(t, id) },
	)
}

func TestLakeFSObjectBytesUsePresignedDataPlane(t *testing.T) {
	fake := newFakeLakeFS(t)
	repo := fake.open(t, "kr://conformance/lakefs-data-plane")
	root, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("large-object-body")
	commit, err := repo.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root, RequestID: "direct-write",
		Changes: []snapshot.TreeChange{{Path: "objects/value.bin", Content: body}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fake.directWrites != 1 {
		t.Fatalf("object bytes did not use exactly one presigned PUT: %d", fake.directWrites)
	}
	got, err := repo.ReadFile("objects/value.bin", commit)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("fixed-commit direct read=%q err=%v", got, err)
	}
	if fake.directReads != 1 {
		t.Fatalf("object bytes did not use exactly one presigned GET: %d", fake.directReads)
	}
}

func TestLakeFSPublicationLockPreventsConcurrentLostUpdate(t *testing.T) {
	fake := newFakeLakeFS(t)
	repo := fake.open(t, "kr://conformance/lakefs-cas")
	root, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for n := 1; n <= 2; n++ {
		n := n
		go func() {
			<-start
			_, err := repo.ApplyTreeCommit(snapshot.TreeChangeSet{
				TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
				BaseCommit: root, ExpectedTargetCommit: root,
				RequestID: fmt.Sprintf("concurrent-%d", n),
				Changes: []snapshot.TreeChange{{
					Path: fmt.Sprintf("objects/%d.bin", n), Content: []byte(strconv.Itoa(n)),
				}},
			})
			results <- err
		}()
	}
	close(start)
	successes := 0
	for range 2 {
		err := <-results
		if err == nil {
			successes++
			continue
		}
		if code := kernel.CodeOf(err); code != kernel.ErrNonFastForward && code != kernel.ErrTemporaryUnavailable {
			t.Fatalf("concurrent loser returned %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent expected-old writes succeeded %d times", successes)
	}
	head, err := repo.Head(snapshot.DefaultRef)
	if err != nil || head == root {
		t.Fatalf("published head=%s err=%v", head, err)
	}
}

func (f *fakeLakeFS) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/blob/") {
		// Presigned data-plane writes run without the metadata lock so that
		// concurrency tests can observe overlapping uploads; state updates
		// inside serveBlob still take mu.
		f.serveBlob(w, r)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.HasPrefix(r.URL.Path, "/read/") {
		f.serveDirectRead(w, r)
		return
	}
	const root = "/api/v1/repositories/knowledge"
	escapedPath := r.URL.EscapedPath()
	if !strings.HasPrefix(escapedPath, root) {
		http.NotFound(w, r)
		return
	}
	suffix := strings.TrimPrefix(escapedPath, root)
	switch {
	case suffix == "" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"id": "knowledge", "default_branch": "main", "storage_namespace": "s3://bucket/knowledge"})
	case suffix == "/branches" && r.Method == http.MethodPost:
		f.createBranch(w, r)
	case strings.HasPrefix(suffix, "/branches/"):
		f.serveBranch(w, r, strings.TrimPrefix(suffix, "/branches/"))
	case strings.HasPrefix(suffix, "/commits/") && r.Method == http.MethodGet:
		f.commitLookups++
		f.getCommit(w, strings.TrimPrefix(suffix, "/commits/"))
	case strings.HasPrefix(suffix, "/refs/"):
		f.serveRef(w, r, strings.TrimPrefix(suffix, "/refs/"))
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeLakeFS) createBranch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Name == "" {
		http.Error(w, "invalid branch", http.StatusBadRequest)
		return
	}
	if _, exists := f.branches[body.Name]; exists {
		http.Error(w, "branch exists", http.StatusConflict)
		return
	}
	source := f.resolve(body.Source)
	if source == "" {
		http.NotFound(w, r)
		return
	}
	f.branches[body.Name] = source
	w.WriteHeader(http.StatusCreated)
}

func (f *fakeLakeFS) serveBranch(w http.ResponseWriter, r *http.Request, tail string) {
	parts := strings.Split(tail, "/")
	name, _ := url.PathUnescape(parts[0])
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			head, ok := f.branches[name]
			if !ok {
				http.NotFound(w, r)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"id": name, "commit_id": head})
		case http.MethodDelete:
			if _, ok := f.branches[name]; !ok {
				http.NotFound(w, r)
				return
			}
			delete(f.branches, name)
			delete(f.staged, name)
			w.WriteHeader(http.StatusNoContent)
		}
		return
	}
	action := strings.Join(parts[1:], "/")
	switch {
	case action == "hard_reset" && r.Method == http.MethodPut:
		source := f.resolve(r.URL.Query().Get("source"))
		if source == "" {
			source = f.resolve(r.URL.Query().Get("ref"))
		}
		if source == "" {
			http.NotFound(w, r)
			return
		}
		f.branches[name] = source
		w.WriteHeader(http.StatusNoContent)
	case action == "commits" && r.Method == http.MethodPost:
		f.commit(w, r, name)
	case action == "staging/backing":
		f.staging(w, r, name)
	case action == "objects" && r.Method == http.MethodDelete:
		f.stageDelete(w, r, name)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeLakeFS) staging(w http.ResponseWriter, r *http.Request, branch string) {
	objectPath := r.URL.Query().Get("path")
	token := branch + ":" + objectPath
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"physical_address": "s3://bucket/staging/" + token,
			"presigned_url":    f.server.URL + "/blob/" + url.PathEscape(token),
		})
	case http.MethodPut:
		var body struct {
			Staging struct {
				PhysicalAddress string `json:"physical_address"`
			} `json:"staging"`
			Size int64 `json:"size_bytes"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "invalid staging metadata", http.StatusBadRequest)
			return
		}
		content, ok := f.uploads[token]
		if !ok || int64(len(content)) != body.Size {
			http.Error(w, "missing upload", http.StatusConflict)
			return
		}
		if f.staged[branch] == nil {
			f.staged[branch] = map[string]*[]byte{}
		}
		copyBody := append([]byte(nil), content...)
		f.staged[branch][objectPath] = &copyBody
		writeJSON(w, http.StatusOK, map[string]any{"path": objectPath, "path_type": "object", "checksum": "etag", "physical_address": body.Staging.PhysicalAddress, "mtime": 1})
	}
}

func (f *fakeLakeFS) serveBlob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.NotFound(w, r)
		return
	}
	token, _ := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/blob/"))
	content, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	current := atomic.AddInt32(&f.blobInFlight, 1)
	for {
		max := atomic.LoadInt32(&f.blobMaxInFlight)
		if current <= max || atomic.CompareAndSwapInt32(&f.blobMaxInFlight, max, current) {
			break
		}
	}
	if sleep := f.blobSleep; sleep > 0 {
		time.Sleep(sleep)
	}
	f.mu.Lock()
	f.uploads[token] = content
	f.directWrites++
	f.mu.Unlock()
	atomic.AddInt32(&f.blobInFlight, -1)
	w.Header().Set("ETag", `"etag"`)
	w.WriteHeader(http.StatusOK)
}

func (f *fakeLakeFS) serveDirectRead(w http.ResponseWriter, r *http.Request) {
	commit := strings.TrimPrefix(r.URL.Path, "/read/")
	content, ok := f.commits[commit].files[r.URL.Query().Get("path")]
	if r.Method != http.MethodGet || !ok {
		http.NotFound(w, r)
		return
	}
	f.directReads++
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (f *fakeLakeFS) stageDelete(w http.ResponseWriter, r *http.Request, branch string) {
	if _, ok := f.branches[branch]; !ok {
		http.NotFound(w, r)
		return
	}
	if f.staged[branch] == nil {
		f.staged[branch] = map[string]*[]byte{}
	}
	f.staged[branch][r.URL.Query().Get("path")] = nil
	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeLakeFS) commit(w http.ResponseWriter, r *http.Request, branch string) {
	parent, ok := f.branches[branch]
	if !ok {
		http.NotFound(w, r)
		return
	}
	files := cloneFiles(f.commits[parent].files)
	for name, content := range f.staged[branch] {
		if content == nil {
			delete(files, name)
		} else {
			files[name] = append([]byte(nil), (*content)...)
		}
	}
	f.next++
	id := "c" + strconv.Itoa(f.next)
	f.commits[id] = fakeCommit{parent: parent, files: files}
	f.branches[branch] = id
	delete(f.staged, branch)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "parents": []string{parent}, "committer": "kc", "message": "update", "creation_date": f.next, "meta_range_id": id})
}

func (f *fakeLakeFS) getCommit(w http.ResponseWriter, raw string) {
	id, _ := url.PathUnescape(raw)
	commit, ok := f.commits[id]
	if !ok {
		http.Error(w, "missing commit", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "parents": []string{commit.parent}, "committer": "kc", "message": "update", "creation_date": 1, "meta_range_id": id})
}

func (f *fakeLakeFS) serveRef(w http.ResponseWriter, r *http.Request, tail string) {
	switch {
	case strings.Contains(tail, "/objects/ls") && r.Method == http.MethodGet:
		ref := strings.TrimSuffix(tail, "/objects/ls")
		f.listObjects(w, r, ref)
	case strings.Contains(tail, "/objects") && r.Method == http.MethodGet:
		ref := strings.TrimSuffix(tail, "/objects")
		f.getObject(w, r, ref)
	case strings.Contains(tail, "/diff/") && r.Method == http.MethodGet:
		parts := strings.SplitN(tail, "/diff/", 2)
		f.diff(w, r, parts[0], parts[1])
	case strings.HasSuffix(tail, "/commits") && r.Method == http.MethodGet:
		f.log(w, r, strings.TrimSuffix(tail, "/commits"))
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeLakeFS) resolve(raw string) string {
	value, _ := url.PathUnescape(raw)
	if head := f.branches[value]; head != "" {
		return head
	}
	if _, ok := f.commits[value]; ok {
		return value
	}
	return ""
}

func (f *fakeLakeFS) getObject(w http.ResponseWriter, r *http.Request, ref string) {
	commit := f.resolve(ref)
	_, ok := f.commits[commit].files[r.URL.Query().Get("path")]
	if commit == "" || !ok {
		http.NotFound(w, r)
		return
	}
	location := f.server.URL + "/read/" + url.PathEscape(commit) + "?path=" +
		url.QueryEscape(r.URL.Query().Get("path"))
	http.Redirect(w, r, location, http.StatusTemporaryRedirect)
}

func (f *fakeLakeFS) listObjects(w http.ResponseWriter, r *http.Request, ref string) {
	commit := f.resolve(ref)
	if commit == "" {
		http.NotFound(w, r)
		return
	}
	prefix, delimiter := r.URL.Query().Get("prefix"), r.URL.Query().Get("delimiter")
	kind := map[string]string{}
	for name := range f.commits[commit].files {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		rest := strings.TrimPrefix(name, prefix)
		if delimiter != "" && strings.Contains(rest, delimiter) {
			first := strings.SplitN(rest, delimiter, 2)[0]
			kind[prefix+first+delimiter] = "common_prefix"
		} else {
			kind[name] = "object"
		}
	}
	names := sortedKeys(kind)
	names, more, next := pageStrings(names, r.URL.Query())
	results := make([]map[string]any, 0, len(names))
	for _, name := range names {
		results = append(results, map[string]any{"path": name, "path_type": kind[name], "checksum": "etag", "physical_address": "s3://bucket/" + name, "mtime": 1})
	}
	writeJSON(w, http.StatusOK, map[string]any{"pagination": map[string]any{"has_more": more, "next_offset": next, "results": len(results), "max_per_page": queryAmount(r.URL.Query())}, "results": results})
}

func (f *fakeLakeFS) log(w http.ResponseWriter, r *http.Request, ref string) {
	current := f.resolve(ref)
	if current == "" {
		http.NotFound(w, r)
		return
	}
	var ids []string
	for current != "" {
		ids = append(ids, current)
		current = f.commits[current].parent
	}
	ids, more, next := pageStrings(ids, r.URL.Query())
	results := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		results = append(results, map[string]any{"id": id, "parents": []string{f.commits[id].parent}, "committer": "kc", "message": "update", "creation_date": 1, "meta_range_id": id})
	}
	writeJSON(w, http.StatusOK, map[string]any{"pagination": map[string]any{"has_more": more, "next_offset": next, "results": len(results), "max_per_page": queryAmount(r.URL.Query())}, "results": results})
}

func (f *fakeLakeFS) diff(w http.ResponseWriter, r *http.Request, leftRef, rightRef string) {
	if r.URL.Query().Get("type") != "two_dot" {
		http.Error(w, "two_dot required", http.StatusBadRequest)
		return
	}
	left, right := f.resolve(leftRef), f.resolve(rightRef)
	if left == "" || right == "" {
		http.NotFound(w, r)
		return
	}
	changed := map[string]bool{}
	for name, body := range f.commits[left].files {
		if other, ok := f.commits[right].files[name]; !ok || !bytes.Equal(body, other) {
			changed[name] = true
		}
	}
	for name, body := range f.commits[right].files {
		if other, ok := f.commits[left].files[name]; !ok || !bytes.Equal(body, other) {
			changed[name] = true
		}
	}
	names := sortedKeys(changed)
	names, more, next := pageStrings(names, r.URL.Query())
	results := make([]map[string]any, 0, len(names))
	for _, name := range names {
		results = append(results, map[string]any{"path": name, "path_type": "object", "type": "changed"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"pagination": map[string]any{"has_more": more, "next_offset": next, "results": len(results), "max_per_page": queryAmount(r.URL.Query())}, "results": results})
}

func queryAmount(query url.Values) int {
	amount, _ := strconv.Atoi(query.Get("amount"))
	if amount <= 0 {
		return 100
	}
	return amount
}

func pageStrings(values []string, query url.Values) ([]string, bool, string) {
	after := query.Get("after")
	start := 0
	if after != "" {
		for start < len(values) && values[start] != after {
			start++
		}
		if start < len(values) {
			start++
		}
	}
	end := start + queryAmount(query)
	if end > len(values) {
		end = len(values)
	}
	more, next := end < len(values), ""
	if more && end > start {
		next = values[end-1]
	}
	return values[start:end], more, next
}

func sortedKeys[V any](values map[string]V) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func cloneFiles(values map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(values))
	for name, body := range values {
		out[name] = append([]byte(nil), body...)
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		panic(fmt.Sprintf("encode fake lakeFS response: %v", err))
	}
}
