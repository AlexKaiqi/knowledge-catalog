package home

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"kc/kernel"
	"kc/snapshot"
)

func TestConnectionReadOnlyRotationRecoveryAndAuthorityBinding(t *testing.T) {
	var mu sync.Mutex
	token, backend, requests := "original-secret", 42, 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests++
		if r.Method != "GET" {
			t.Errorf("connection wrote external source: %s", r.Method)
			w.WriteHeader(405)
			return
		}
		if r.Header.Get("Authorization") != "token "+token {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(token))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/repos/alice/existing":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": backend, "default_branch": "main", "empty": false})
		case "/api/v1/repos/alice/existing/branches/main":
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "main", "commit": map[string]any{"id": "initial"}})
		case "/api/v1/repos/alice/existing/git/commits/initial":
			_ = json.NewEncoder(w).Encode(map[string]any{"sha": "initial"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer provider.Close()
	cfg := deploymentFixture(t)
	cfg.Connections = &ConnectionPolicy{AllowedOrigins: []string{provider.URL}, CreatorActions: []string{"repository.connections.manage", "repository.metadata.read", "knowledge.read"}}
	if err := InitializeDeployment(cfg, func(dir, _ string) error {
		return os.WriteFile(filepath.Join(dir, "allow.json"), []byte(`{"version":2,"rules":[]}`), 0600)
	}); err != nil {
		t.Fatal(err)
	}
	ws, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	req := ConnectionRequest{Catalog: cfg.Catalogs[0].ID, Repository: "kr://alice/existing", Driver: "gitea", URL: provider.URL + "/alice/existing", Principal: "alice", Credential: token}
	grants := 0
	grant := func(g RepositoryInitialGrant) error {
		grants++
		if g.Principal != "alice" || len(g.Actions) != 3 {
			t.Fatalf("grant %#v", g)
		}
		return nil
	}
	connected, err := ws.ConnectRepository(req, grant)
	if err != nil || connected.Head != "initial" || connected.Status != "READY" {
		t.Fatalf("connect %#v %v", connected, err)
	}
	if !ws.Catalog.HasRepository(kernel.RepositoryID(req.Repository)) {
		t.Fatal("connection not admitted")
	}
	raw, _ := json.Marshal(connected)
	if strings.Contains(string(raw), "secret") {
		t.Fatal("secret in result")
	}
	info, err := os.Stat(filepath.Join(cfg.StateDir, connectionLedgerFile))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private ledger %v %v", info, err)
	}
	if _, err := ws.ConnectRepository(req, grant); err != nil || grants != 1 {
		t.Fatalf("replay regranted: %d %v", grants, err)
	}
	if _, err := ws.GetConnection("bob", req.Repository); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("another owner %v", err)
	}
	if _, err := ws.RotateConnectionCredential("alice", req.Repository, "bad-secret"); err == nil || strings.Contains(err.Error(), token) {
		t.Fatalf("bad credential unsafe result %v", err)
	}
	if _, err := ws.CheckConnection("alice", req.Repository); err != nil {
		t.Fatalf("failed rotation lost old secret: %v", err)
	}
	_ = ws.Close()
	mu.Lock()
	token = "replacement-secret"
	before := requests
	mu.Unlock()
	cfg.CacheDir = filepath.Join(t.TempDir(), "replacement-cache")
	recovered, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatalf("expired credential blocked management recovery: %v", err)
	}
	defer recovered.Close()
	mu.Lock()
	if requests != before {
		t.Error("recovery required available provider")
	}
	mu.Unlock()
	if _, err := recovered.GetConnection("alice", req.Repository); err != nil {
		t.Fatal(err)
	}
	source, _ := recovered.Store.Get(kernel.RepositoryID(req.Repository))
	if _, ok := source.(snapshot.TreeStore); !ok {
		t.Fatal("Gitea literal tree capability lost")
	}
	if _, err := source.Head(snapshot.DefaultRef); err == nil {
		t.Fatal("expired credential still reads")
	}
	if _, err := recovered.RotateConnectionCredential("alice", req.Repository, token); err != nil {
		t.Fatalf("recovery rotation %v", err)
	}
	if head, err := source.Head(snapshot.DefaultRef); err != nil || head != "initial" {
		t.Fatalf("existing handle did not use rotated credential %s %v", head, err)
	}
	mu.Lock()
	backend = 99
	mu.Unlock()
	if _, err := recovered.CheckConnection("alice", req.Repository); err == nil {
		t.Fatal("same URL different authority accepted")
	}
	if _, err := recovered.RotateConnectionCredential("alice", req.Repository, token); err == nil {
		t.Fatal("rotation accepted replaced authority")
	}
	if err := os.Remove(filepath.Join(cfg.StateDir, connectionLedgerFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenDeployment(cfg); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("lost ledger opened: %v", err)
	}
}

func TestConnectionPolicyRejectsUnapprovedOriginsAndPrivileges(t *testing.T) {
	cfg := deploymentFixture(t)
	cfg.Connections = &ConnectionPolicy{AllowedOrigins: []string{"https://gitea.example"}, CreatorActions: []string{"repository.connections.manage"}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"file:///etc", "https://user:secret@gitea.example", "https://gitea.example?q=secret"} {
		cfg.Connections.AllowedOrigins = []string{origin}
		if err := cfg.Validate(); err == nil {
			t.Errorf("accepted origin %s", origin)
		}
	}
	cfg.Connections.AllowedOrigins = []string{"https://gitea.example"}
	cfg.Connections.CreatorActions = []string{"admin.grants.manage"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("privilege escalation accepted")
	}
}
