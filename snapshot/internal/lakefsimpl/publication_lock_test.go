package lakefsimpl

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"kc/kernel"
)

func TestContenderNeverDeletesAnotherPublishersLock(t *testing.T) {
	for _, row := range []struct {
		name, candidate string
		code            kernel.ErrorCode
	}{{"same published candidate", "c1", ""}, {"different candidate", "c2", kernel.ErrNonFastForward}} {
		t.Run(row.name, func(t *testing.T) {
			var deletes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodPost:
					w.WriteHeader(http.StatusConflict)
				case http.MethodDelete:
					deletes.Add(1)
					w.WriteHeader(http.StatusNoContent)
				case http.MethodGet:
					if !strings.Contains(r.URL.Path, "/branches/") {
						t.Errorf("unexpected read %s", r.URL.Path)
					}
					_ = json.NewEncoder(w).Encode(map[string]string{"commit_id": "c1"})
				default:
					t.Errorf("contender mutated target with %s", r.Method)
				}
			}))
			t.Cleanup(server.Close)
			client, err := newAPIClient(Endpoint{API: server.URL, Repository: "source"}, "access:secret")
			if err != nil {
				t.Fatal(err)
			}
			repo := &Repository{id: "kr://conformance/publisher-owner", client: client}
			err = repo.publishBranch("main", kernel.CommitID(row.candidate), "c0")
			if kernel.CodeOf(err) != row.code {
				t.Fatalf("contender result %v, want %s", err, row.code)
			}
			if deletes.Load() != 0 {
				t.Fatal("observing a published target does not prove its publisher has stopped; contender deleted the active owner's lock")
			}
		})
	}
}
