package lakefs_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/lakefs"
)

type managedLakeFSFake struct {
	mu    sync.Mutex
	repos map[string]map[string]string
}

func newManagedLakeFSFake(t *testing.T) (*httptest.Server, *managedLakeFSFake) {
	t.Helper()
	fake := &managedLakeFSFake{repos: map[string]map[string]string{}}
	server := httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(server.Close)
	return server, fake
}

func (f *managedLakeFSFake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repositories":
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Name             string `json:"name"`
			StorageNamespace string `json:"storage_namespace"`
			DefaultBranch    string `json:"default_branch"`
		}
		if json.Unmarshal(raw, &body) != nil || body.Name == "" || body.StorageNamespace == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, exists := f.repos[body.Name]; exists {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if body.DefaultBranch == "" {
			body.DefaultBranch = "main"
		}
		f.repos[body.Name] = map[string]string{
			"id":                body.Name,
			"default_branch":    body.DefaultBranch,
			"storage_namespace": body.StorageNamespace,
			"commit":            "c0",
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(f.repos[body.Name])
		return
	case strings.HasPrefix(r.URL.Path, "/api/v1/repositories/"):
		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/repositories/")
		name, suffix, _ := strings.Cut(rest, "/")
		repo := f.repos[name]
		if repo == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch {
		case suffix == "" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(repo)
		case suffix == "branches/main" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "main", "commit_id": repo["commit"]})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
		return
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestManagedLakeFSCreatesAndResumesAllocation(t *testing.T) {
	server, fake := newManagedLakeFSFake(t)
	id := kernel.RepositoryID("kr://kaiqidong/notes")
	allocation := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	name := "kaiqidong-notes"
	dsn := server.URL + "/" + name
	prefix := "s3://kc-authority"
	repo, backend, err := lakefs.CreateManaged(id, dsn, "access:secret", allocation, prefix)
	if err != nil || backend != prefix+"/"+name {
		t.Fatalf("create: backend=%s err=%v", backend, err)
	}
	head := mustHead(t, repo)
	if fake.repos[name]["storage_namespace"] != prefix+"/"+name {
		t.Fatalf("storage namespace: %#v", fake.repos)
	}
	replayed, again, err := lakefs.CreateManaged(id, dsn, "access:secret", allocation, prefix)
	if err != nil || again != backend {
		t.Fatalf("resume: backend=%s err=%v", again, err)
	}
	if mustHead(t, replayed) != head {
		t.Fatal("resume moved the initial commit")
	}
	opened, err := lakefs.OpenManaged(id, dsn, "access:secret", allocation, backend)
	if err != nil {
		t.Fatal(err)
	}
	if mustHead(t, opened) != head {
		t.Fatal("open lost the initial commit")
	}
}

func TestManagedLakeFSStorageNamespaceUsesPrefixAndName(t *testing.T) {
	server, fake := newManagedLakeFSFake(t)
	allocation := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	prefix := "s3://kc-authority/tianqiong"
	_, backend, err := lakefs.CreateManaged("kr://admin/table-meta", server.URL+"/table-meta", "access:secret", allocation, prefix)
	if err != nil || backend != prefix+"/table-meta" {
		t.Fatalf("backend=%s err=%v", backend, err)
	}
	if fake.repos["table-meta"]["storage_namespace"] != "s3://kc-authority/tianqiong/table-meta" {
		t.Fatalf("storage namespace: %#v", fake.repos)
	}
}

func TestManagedLakeFSNeverAdoptsForeignRepository(t *testing.T) {
	server, fake := newManagedLakeFSFake(t)
	allocation := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fake.repos["kaiqidong-notes"] = map[string]string{
		"id": "kaiqidong-notes", "default_branch": "main",
		"storage_namespace": "s3://other/kaiqidong-notes", "commit": "c0",
	}
	if _, _, err := lakefs.CreateManaged("kr://managed/repo", server.URL+"/kaiqidong-notes", "access:secret", allocation, "s3://kc-authority"); err == nil {
		t.Fatal("managed create adopted a foreign storage namespace")
	}
}

func mustHead(t *testing.T, repo *lakefs.Repository) kernel.CommitID {
	t.Helper()
	head, err := repo.Head(snapshot.DefaultRef)
	if err != nil || head == "" {
		t.Fatalf("head: %s %v", head, err)
	}
	return head
}
