package testkit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// LakeFSFakeCredential is the access-key:secret the in-process fake accepts.
const LakeFSFakeCredential = "access:secret"

// LakeFSFake is a protocol-faithful lakeFS HTTP stand-in for scene isolation.
// It is not a selectable Snapshot adapter; production still talks Graveler.
type LakeFSFake struct {
	mu     sync.Mutex
	server *httptest.Server
	next   int
	seq    atomic.Uint64
	repos  map[string]*lakefsFakeRepo
}

type lakefsFakeRepo struct {
	commits  map[string]lakefsFakeCommit
	branches map[string]string
	staged   map[string]map[string]*[]byte
	uploads  map[string][]byte
}

type lakefsFakeCommit struct {
	parent string
	files  map[string][]byte
}

// NewLakeFSFake starts one shared fake for a test (or scene cache).
func NewLakeFSFake(t *testing.T) *LakeFSFake {
	t.Helper()
	f := &LakeFSFake{repos: map[string]*lakefsFakeRepo{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serveHTTP))
	t.Cleanup(f.server.Close)
	return f
}

// NewRepo creates an empty lakeFS repository named for this process and returns
// the physical repository id used in the DSN.
func (f *LakeFSFake) NewRepo() string {
	name := fmt.Sprintf("scn%010d", f.seq.Add(1))
	f.mu.Lock()
	defer f.mu.Unlock()
	f.repos[name] = newEmptyLakeFSRepo()
	return name
}

// Credential is the access-key:secret the fake accepts.
func (f *LakeFSFake) Credential() string {
	return LakeFSFakeCredential
}

// DSN is the KC lakeFS DSN for a physical repository on this fake.
func (f *LakeFSFake) DSN(name string) string {
	return strings.TrimRight(f.server.URL, "/") + "/" + name
}

// OwnsDSN reports whether dsn names a repository on this fake.
func (f *LakeFSFake) OwnsDSN(dsn string) bool {
	origin := strings.TrimRight(f.server.URL, "/")
	dsn = strings.TrimRight(strings.TrimSpace(dsn), "/")
	return strings.HasPrefix(dsn, origin+"/")
}

// ForkStamps copies each lakefs remote.yaml in dst onto a new physical repo so
// a copied scene home does not share Snapshot authority with its parent.
func (f *LakeFSFake) ForkStamps(dst string) error {
	var stamps []string
	err := filepath.WalkDir(dst, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || d.Name() != "remote.yaml" {
			return nil
		}
		stamps = append(stamps, path)
		return nil
	})
	if err != nil {
		return err
	}
	for _, path := range stamps {
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		id, dsn, ok := parseLakeFSStamp(raw)
		if !ok {
			continue
		}
		srcName := lakefsRepoFromDSN(dsn)
		dstName := fmt.Sprintf("scn%010d", f.seq.Add(1))
		if err := f.cloneRepo(srcName, dstName); err != nil {
			return err
		}
		rewritten := fmt.Sprintf("id: %s\ndriver: lakefs\ndsn: %s\n", id, f.DSN(dstName))
		if err := os.WriteFile(path, []byte(rewritten), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (f *LakeFSFake) cloneRepo(srcName, dstName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	src := f.repos[srcName]
	if src == nil {
		return fmt.Errorf("lakefs fake: source repo %s missing", srcName)
	}
	dst := &lakefsFakeRepo{
		commits:  map[string]lakefsFakeCommit{},
		branches: map[string]string{},
		staged:   map[string]map[string]*[]byte{},
		uploads:  map[string][]byte{},
	}
	for id, commit := range src.commits {
		dst.commits[id] = lakefsFakeCommit{parent: commit.parent, files: cloneByteMap(commit.files)}
	}
	for name, head := range src.branches {
		dst.branches[name] = head
	}
	f.repos[dstName] = dst
	return nil
}

func newEmptyLakeFSRepo() *lakefsFakeRepo {
	return &lakefsFakeRepo{
		commits:  map[string]lakefsFakeCommit{"c0": {files: map[string][]byte{}}},
		branches: map[string]string{"main": "c0"},
		staged:   map[string]map[string]*[]byte{},
		uploads:  map[string][]byte{},
	}
}

func (f *LakeFSFake) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.HasPrefix(r.URL.Path, "/blob/") {
		f.serveBlob(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/read/") {
		f.serveDirectRead(w, r)
		return
	}
	repoName, suffix, ok := splitLakeFSAPIPath(r.URL.EscapedPath())
	if !ok {
		http.NotFound(w, r)
		return
	}
	repo := f.repos[repoName]
	if repo == nil {
		http.NotFound(w, r)
		return
	}
	switch {
	case suffix == "" && r.Method == http.MethodGet:
		writeLakeFSJSON(w, http.StatusOK, map[string]any{"id": repoName, "default_branch": "main", "storage_namespace": "s3://bucket/" + repoName})
	case suffix == "/branches" && r.Method == http.MethodPost:
		f.createBranch(w, r, repo)
	case strings.HasPrefix(suffix, "/branches/"):
		f.serveBranch(w, r, repoName, repo, strings.TrimPrefix(suffix, "/branches/"))
	case strings.HasPrefix(suffix, "/commits/") && r.Method == http.MethodGet:
		f.getCommit(w, repo, strings.TrimPrefix(suffix, "/commits/"))
	case strings.HasPrefix(suffix, "/refs/"):
		f.serveRef(w, r, repoName, repo, strings.TrimPrefix(suffix, "/refs/"))
	default:
		http.NotFound(w, r)
	}
}

func splitLakeFSAPIPath(escaped string) (repo, suffix string, ok bool) {
	const prefix = "/api/v1/repositories/"
	if !strings.HasPrefix(escaped, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(escaped, prefix)
	name, after, found := strings.Cut(rest, "/")
	name, _ = url.PathUnescape(name)
	if name == "" {
		return "", "", false
	}
	if !found {
		return name, "", true
	}
	return name, "/" + after, true
}

func (f *LakeFSFake) createBranch(w http.ResponseWriter, r *http.Request, repo *lakefsFakeRepo) {
	var body struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Name == "" {
		http.Error(w, "invalid branch", http.StatusBadRequest)
		return
	}
	if _, exists := repo.branches[body.Name]; exists {
		http.Error(w, "branch exists", http.StatusConflict)
		return
	}
	source := resolveLakeFSRef(repo, body.Source)
	if source == "" {
		http.NotFound(w, r)
		return
	}
	repo.branches[body.Name] = source
	w.WriteHeader(http.StatusCreated)
}

func (f *LakeFSFake) serveBranch(w http.ResponseWriter, r *http.Request, repoName string, repo *lakefsFakeRepo, tail string) {
	parts := strings.Split(tail, "/")
	name, _ := url.PathUnescape(parts[0])
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			head, ok := repo.branches[name]
			if !ok {
				http.NotFound(w, r)
				return
			}
			writeLakeFSJSON(w, http.StatusOK, map[string]any{"id": name, "commit_id": head})
		case http.MethodDelete:
			if _, ok := repo.branches[name]; !ok {
				http.NotFound(w, r)
				return
			}
			delete(repo.branches, name)
			delete(repo.staged, name)
			w.WriteHeader(http.StatusNoContent)
		}
		return
	}
	action := strings.Join(parts[1:], "/")
	switch {
	case action == "hard_reset" && r.Method == http.MethodPut:
		source := resolveLakeFSRef(repo, r.URL.Query().Get("source"))
		if source == "" {
			source = resolveLakeFSRef(repo, r.URL.Query().Get("ref"))
		}
		if source == "" {
			http.NotFound(w, r)
			return
		}
		repo.branches[name] = source
		w.WriteHeader(http.StatusNoContent)
	case action == "commits" && r.Method == http.MethodPost:
		f.commit(w, r, repo, name)
	case action == "staging/backing":
		f.staging(w, r, repoName, repo, name)
	case action == "objects" && r.Method == http.MethodDelete:
		f.stageDelete(w, r, repo, name)
	default:
		http.NotFound(w, r)
	}
}

func (f *LakeFSFake) staging(w http.ResponseWriter, r *http.Request, repoName string, repo *lakefsFakeRepo, branch string) {
	objectPath := r.URL.Query().Get("path")
	token := repoName + ":" + branch + ":" + objectPath
	switch r.Method {
	case http.MethodGet:
		writeLakeFSJSON(w, http.StatusOK, map[string]any{
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
		content, ok := repo.uploads[token]
		if !ok || int64(len(content)) != body.Size {
			http.Error(w, "missing upload", http.StatusConflict)
			return
		}
		if repo.staged[branch] == nil {
			repo.staged[branch] = map[string]*[]byte{}
		}
		copyBody := append([]byte(nil), content...)
		repo.staged[branch][objectPath] = &copyBody
		writeLakeFSJSON(w, http.StatusOK, map[string]any{
			"path": objectPath, "path_type": "object", "checksum": "etag",
			"physical_address": body.Staging.PhysicalAddress, "mtime": 1,
		})
	}
}

func (f *LakeFSFake) serveBlob(w http.ResponseWriter, r *http.Request) {
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
	repoName, _, ok := strings.Cut(token, ":")
	if !ok {
		http.NotFound(w, r)
		return
	}
	repo := f.repos[repoName]
	if repo == nil {
		http.NotFound(w, r)
		return
	}
	repo.uploads[token] = content
	w.Header().Set("ETag", `"etag"`)
	w.WriteHeader(http.StatusOK)
}

func (f *LakeFSFake) serveDirectRead(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/read/")
	repoName, commit, ok := strings.Cut(rest, "/")
	repoName, _ = url.PathUnescape(repoName)
	commit, _ = url.PathUnescape(commit)
	repo := f.repos[repoName]
	if r.Method != http.MethodGet || !ok || repo == nil {
		http.NotFound(w, r)
		return
	}
	content, exists := repo.commits[commit].files[r.URL.Query().Get("path")]
	if !exists {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (f *LakeFSFake) stageDelete(w http.ResponseWriter, r *http.Request, repo *lakefsFakeRepo, branch string) {
	if _, ok := repo.branches[branch]; !ok {
		http.NotFound(w, r)
		return
	}
	if repo.staged[branch] == nil {
		repo.staged[branch] = map[string]*[]byte{}
	}
	repo.staged[branch][r.URL.Query().Get("path")] = nil
	w.WriteHeader(http.StatusNoContent)
}

func (f *LakeFSFake) commit(w http.ResponseWriter, r *http.Request, repo *lakefsFakeRepo, branch string) {
	parent, ok := repo.branches[branch]
	if !ok {
		http.NotFound(w, r)
		return
	}
	files := cloneByteMap(repo.commits[parent].files)
	for name, content := range repo.staged[branch] {
		if content == nil {
			delete(files, name)
		} else {
			files[name] = append([]byte(nil), (*content)...)
		}
	}
	f.next++
	id := "c" + strconv.Itoa(f.next)
	repo.commits[id] = lakefsFakeCommit{parent: parent, files: files}
	repo.branches[branch] = id
	delete(repo.staged, branch)
	writeLakeFSJSON(w, http.StatusCreated, map[string]any{
		"id": id, "parents": []string{parent}, "committer": "kc", "message": "update",
		"creation_date": f.next, "meta_range_id": id,
	})
}

func (f *LakeFSFake) getCommit(w http.ResponseWriter, repo *lakefsFakeRepo, raw string) {
	id, _ := url.PathUnescape(raw)
	commit, ok := repo.commits[id]
	if !ok {
		http.Error(w, "missing commit", http.StatusNotFound)
		return
	}
	writeLakeFSJSON(w, http.StatusOK, map[string]any{
		"id": id, "parents": []string{commit.parent}, "committer": "kc", "message": "update",
		"creation_date": 1, "meta_range_id": id,
	})
}

func (f *LakeFSFake) serveRef(w http.ResponseWriter, r *http.Request, repoName string, repo *lakefsFakeRepo, tail string) {
	switch {
	case strings.Contains(tail, "/objects/ls") && r.Method == http.MethodGet:
		f.listObjects(w, r, repo, strings.TrimSuffix(tail, "/objects/ls"))
	case strings.Contains(tail, "/objects") && r.Method == http.MethodGet:
		f.getObject(w, r, repoName, repo, strings.TrimSuffix(tail, "/objects"))
	case strings.Contains(tail, "/diff/") && r.Method == http.MethodGet:
		parts := strings.SplitN(tail, "/diff/", 2)
		f.diff(w, r, repo, parts[0], parts[1])
	case strings.HasSuffix(tail, "/commits") && r.Method == http.MethodGet:
		f.log(w, r, repo, strings.TrimSuffix(tail, "/commits"))
	default:
		http.NotFound(w, r)
	}
}

func resolveLakeFSRef(repo *lakefsFakeRepo, raw string) string {
	value, _ := url.PathUnescape(raw)
	if head := repo.branches[value]; head != "" {
		return head
	}
	if _, ok := repo.commits[value]; ok {
		return value
	}
	return ""
}

func (f *LakeFSFake) getObject(w http.ResponseWriter, r *http.Request, repoName string, repo *lakefsFakeRepo, ref string) {
	commit := resolveLakeFSRef(repo, ref)
	_, ok := repo.commits[commit].files[r.URL.Query().Get("path")]
	if commit == "" || !ok {
		http.NotFound(w, r)
		return
	}
	location := f.server.URL + "/read/" + url.PathEscape(repoName) + "/" + url.PathEscape(commit) +
		"?path=" + url.QueryEscape(r.URL.Query().Get("path"))
	http.Redirect(w, r, location, http.StatusTemporaryRedirect)
}

func (f *LakeFSFake) listObjects(w http.ResponseWriter, r *http.Request, repo *lakefsFakeRepo, ref string) {
	commit := resolveLakeFSRef(repo, ref)
	if commit == "" {
		http.NotFound(w, r)
		return
	}
	prefix, delimiter := r.URL.Query().Get("prefix"), r.URL.Query().Get("delimiter")
	kind := map[string]string{}
	for name := range repo.commits[commit].files {
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
	names := sortedLakeFSKeys(kind)
	names, more, next := pageLakeFSStrings(names, r.URL.Query())
	results := make([]map[string]any, 0, len(names))
	for _, name := range names {
		results = append(results, map[string]any{
			"path": name, "path_type": kind[name], "checksum": "etag",
			"physical_address": "s3://bucket/" + name, "mtime": 1,
		})
	}
	writeLakeFSJSON(w, http.StatusOK, map[string]any{
		"pagination": map[string]any{"has_more": more, "next_offset": next, "results": len(results), "max_per_page": lakeFSQueryAmount(r.URL.Query())},
		"results":    results,
	})
}

func (f *LakeFSFake) log(w http.ResponseWriter, r *http.Request, repo *lakefsFakeRepo, ref string) {
	current := resolveLakeFSRef(repo, ref)
	if current == "" {
		http.NotFound(w, r)
		return
	}
	var ids []string
	for current != "" {
		ids = append(ids, current)
		current = repo.commits[current].parent
	}
	ids, more, next := pageLakeFSStrings(ids, r.URL.Query())
	results := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		results = append(results, map[string]any{
			"id": id, "parents": []string{repo.commits[id].parent}, "committer": "kc", "message": "update",
			"creation_date": 1, "meta_range_id": id,
		})
	}
	writeLakeFSJSON(w, http.StatusOK, map[string]any{
		"pagination": map[string]any{"has_more": more, "next_offset": next, "results": len(results), "max_per_page": lakeFSQueryAmount(r.URL.Query())},
		"results":    results,
	})
}

func (f *LakeFSFake) diff(w http.ResponseWriter, r *http.Request, repo *lakefsFakeRepo, leftRef, rightRef string) {
	if r.URL.Query().Get("type") != "two_dot" {
		http.Error(w, "two_dot required", http.StatusBadRequest)
		return
	}
	left, right := resolveLakeFSRef(repo, leftRef), resolveLakeFSRef(repo, rightRef)
	if left == "" || right == "" {
		http.NotFound(w, r)
		return
	}
	changed := map[string]bool{}
	for name, body := range repo.commits[left].files {
		if other, ok := repo.commits[right].files[name]; !ok || !bytes.Equal(body, other) {
			changed[name] = true
		}
	}
	for name, body := range repo.commits[right].files {
		if other, ok := repo.commits[left].files[name]; !ok || !bytes.Equal(body, other) {
			changed[name] = true
		}
	}
	names := sortedLakeFSKeys(changed)
	names, more, next := pageLakeFSStrings(names, r.URL.Query())
	results := make([]map[string]any, 0, len(names))
	for _, name := range names {
		results = append(results, map[string]any{"path": name, "path_type": "object", "type": "changed"})
	}
	writeLakeFSJSON(w, http.StatusOK, map[string]any{
		"pagination": map[string]any{"has_more": more, "next_offset": next, "results": len(results), "max_per_page": lakeFSQueryAmount(r.URL.Query())},
		"results":    results,
	})
}

func parseLakeFSStamp(raw []byte) (id, dsn string, ok bool) {
	driver := ""
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key, value = strings.TrimSpace(key), strings.Trim(strings.TrimSpace(value), `"'`)
		switch key {
		case "id":
			id = value
		case "driver":
			driver = value
		case "dsn":
			dsn = value
		}
	}
	return id, dsn, driver == "lakefs" && id != "" && dsn != ""
}

func lakefsRepoFromDSN(dsn string) string {
	dsn = strings.TrimRight(strings.TrimSpace(dsn), "/")
	if i := strings.LastIndex(dsn, "/"); i >= 0 {
		return dsn[i+1:]
	}
	return dsn
}

func cloneByteMap(values map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(values))
	for name, body := range values {
		out[name] = append([]byte(nil), body...)
	}
	return out
}

func lakeFSQueryAmount(query url.Values) int {
	amount, _ := strconv.Atoi(query.Get("amount"))
	if amount <= 0 {
		return 100
	}
	return amount
}

func pageLakeFSStrings(values []string, query url.Values) ([]string, bool, string) {
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
	end := start + lakeFSQueryAmount(query)
	if end > len(values) {
		end = len(values)
	}
	more, next := end < len(values), ""
	if more && end > start {
		next = values[end-1]
	}
	return values[start:end], more, next
}

func sortedLakeFSKeys[V any](values map[string]V) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func writeLakeFSJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		panic(fmt.Sprintf("encode fake lakeFS response: %v", err))
	}
}
