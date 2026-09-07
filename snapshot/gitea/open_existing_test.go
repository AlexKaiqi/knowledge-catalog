package gitea_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/gitea"
)

func TestOpenExistingGiteaNeverCreatesOrRepairsAuthority(t *testing.T) {
	for _, tc := range []struct {
		name         string
		repoStatus   int
		empty        bool
		branchStatus int
		commitStatus int
		wantError    bool
	}{
		{name: "existing ordinary repository", repoStatus: 200, branchStatus: 200, commitStatus: 200},
		{name: "missing repository", repoStatus: 404, wantError: true},
		{name: "empty repository", repoStatus: 200, empty: true, wantError: true},
		{name: "missing published ref", repoStatus: 200, branchStatus: 404, wantError: true},
		{name: "inaccessible commit", repoStatus: 200, branchStatus: 200, commitStatus: 404, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("opening existing authority attempted %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v1/repos/acme/source":
					w.WriteHeader(tc.repoStatus)
					_ = json.NewEncoder(w).Encode(map[string]any{"default_branch": "main", "empty": tc.empty})
				case "/api/v1/repos/acme/source/branches/main":
					w.WriteHeader(tc.branchStatus)
					_ = json.NewEncoder(w).Encode(map[string]any{"name": "main", "commit": map[string]any{"id": "published-commit"}})
				case "/api/v1/repos/acme/source/commits/published-commit", "/api/v1/repos/acme/source/git/commits/published-commit":
					w.WriteHeader(tc.commitStatus)
					_ = json.NewEncoder(w).Encode(map[string]any{"sha": "published-commit"})
				default:
					t.Errorf("unexpected authority probe %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			repo, err := gitea.OpenExisting("kr://acme/source", server.URL+"/acme/source", "test-token")
			if (err != nil) != tc.wantError {
				t.Fatalf("OpenExisting error = %v, want error %v", err, tc.wantError)
			}
			if tc.wantError {
				return
			}
			if repo.ID() != kernel.RepositoryID("kr://acme/source") {
				t.Fatalf("identity = %s", repo.ID())
			}
			if commit, err := repo.Head(snapshot.DefaultRef); err != nil || commit != "published-commit" {
				t.Fatalf("published head = %s, %v", commit, err)
			}
		})
	}
}

func TestOpenExistingLiveGiteaPreservesPublishedAuthority(t *testing.T) {
	base, token, run := testkit.GiteaEndpoint(t)
	id := kernel.RepositoryID("kr://acme/existing-gitea")
	sum := sha256.Sum256([]byte(string(id) + run))
	dsn := base + "/kc/kc-existing-" + hex.EncodeToString(sum[:8])
	// Only the fixture creation path is allowed to create or publish.
	created, err := gitea.Open(id, dsn, token)
	if err != nil {
		t.Fatal(err)
	}
	head := testkit.MustHead(t, created, snapshot.DefaultRef)
	published, err := created.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: id, TargetRef: snapshot.DefaultRef,
		BaseCommit: head, ExpectedTargetCommit: head,
		Changes: []snapshot.TreeChange{{Path: "ordinary.txt", Content: []byte("ordinary Git content\n")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := gitea.OpenExisting(id, dsn, token)
	if err != nil {
		t.Fatal(err)
	}
	if after := testkit.MustHead(t, opened, snapshot.DefaultRef); after != published {
		t.Fatalf("restoration changed HEAD: %s -> %s", published, after)
	}
	if content, err := opened.ReadFile("ordinary.txt", published); err != nil || string(content) != "ordinary Git content\n" {
		t.Fatalf("ordinary Snapshot changed or was rejected: %q, %v", content, err)
	}
}
