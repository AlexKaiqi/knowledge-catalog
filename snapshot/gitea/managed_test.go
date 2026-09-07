package gitea_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/gitea"
)

func TestManagedGiteaNeverAdoptsExistingRepository(t *testing.T) {
	for _, empty := range []bool{true, false} {
		t.Run(map[bool]string{true: "empty", false: "published"}[empty], func(t *testing.T) {
			mutations := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					mutations++
					w.WriteHeader(405)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"id": 42, "empty": empty, "default_branch": "main", "description": "external"})
			}))
			defer server.Close()
			if _, _, err := gitea.CreateManaged("kr://managed/repo", server.URL+"/kc/source", "token", "allocation"); err == nil {
				t.Fatal("managed create adopted an external repository")
			}
			if mutations != 0 {
				t.Fatal("managed create modified an external repository")
			}
		})
	}
}

func TestManagedGiteaDoesNotRepairUncertainBranch(t *testing.T) {
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			mutations := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					mutations++
					w.WriteHeader(405)
					return
				}
				if strings.HasSuffix(r.URL.Path, "/branches/main") {
					w.WriteHeader(status)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"id": 41, "description": "kc-managed:allocation:kr://managed/repo", "default_branch": "main", "empty": false})
			}))
			defer server.Close()
			if _, _, err := gitea.CreateManaged("kr://managed/repo", server.URL+"/kc/source", "token", "allocation"); err == nil {
				t.Fatal("uncertain branch was accepted")
			}
			if mutations != 0 {
				t.Fatalf("uncertain branch triggered %d source mutations", mutations)
			}
		})
	}
}

func TestManagedLiveGiteaPreservesAllocationAndInitialHead(t *testing.T) {
	base, token, run := testkit.GiteaEndpoint(t)
	sum := sha256.Sum256([]byte("managed-" + run))
	allocation := hex.EncodeToString(sum[:16])
	dsn := base + "/kc/kc-owned-" + allocation
	id := kernel.RepositoryID("kr://managed/live-gitea")
	repo, backend, err := gitea.CreateManaged(id, dsn, token, allocation)
	if err != nil || backend <= 0 {
		t.Fatalf("owned create: backend=%d %v", backend, err)
	}
	head := testkit.MustHead(t, repo, snapshot.DefaultRef)
	replayed, again, err := gitea.CreateManaged(id, dsn, token, allocation)
	if err != nil || again != backend || testkit.MustHead(t, replayed, snapshot.DefaultRef) != head {
		t.Fatalf("owned retry changed source: backend=%d %v", again, err)
	}
	if _, _, err := gitea.CreateManaged(id, dsn, token, "another-allocation"); err == nil {
		t.Fatal("different allocation adopted existing authority")
	}
	if _, err := gitea.OpenManaged(id, dsn, token, allocation, backend+1); err == nil {
		t.Fatal("wrong backend identity accepted")
	}
	if testkit.MustHead(t, repo, snapshot.DefaultRef) != head {
		t.Fatal("rejected adoption changed published head")
	}
}

func TestManagedGiteaRecoversLostCreateResponseAndGuardsOpenedHandle(t *testing.T) {
	var mu sync.Mutex
	backend := int64(0)
	description := ""
	creates := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/user":
			_ = json.NewEncoder(w).Encode(map[string]any{"login": "kc"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/user/repos":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			creates++
			backend = 41
			description = body["description"].(string)
			w.WriteHeader(201)
			_, _ = w.Write([]byte("{")) // Accepted, but the response cannot be decoded.
		case r.URL.Path == "/api/v1/repos/kc/source":
			if backend == 0 {
				w.WriteHeader(404)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": backend, "description": description, "default_branch": "main", "empty": false})
		case strings.HasSuffix(r.URL.Path, "/branches/main"):
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "main", "commit": map[string]any{"id": "initial"}})
		case strings.Contains(r.URL.Path, "/commits/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"sha": "initial"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	created, id, err := gitea.CreateManaged("kr://managed/repo", server.URL+"/kc/source", "token", "allocation")
	if err != nil || id != 41 {
		t.Fatalf("ambiguous create recovery: backend=%d %v", id, err)
	}
	if _, _, err := gitea.CreateManaged("kr://managed/repo", server.URL+"/kc/source", "token", "allocation"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	count := creates
	backend = 42
	mu.Unlock()
	if count != 1 {
		t.Fatal("retry created a second repository")
	}
	if _, err := gitea.OpenManaged("kr://managed/repo", server.URL+"/kc/source", "token", "allocation", 41); err == nil {
		t.Fatal("recovery accepted replacement backend")
	}
	if _, err := created.Head(snapshot.DefaultRef); err == nil {
		t.Fatal("already opened managed handle followed replacement backend")
	}
}
