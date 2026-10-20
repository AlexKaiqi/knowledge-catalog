package lakefsimpl

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/kernel"
)

func TestCommitMetadataRejectsAnIncorrectImmutableBasis(t *testing.T) {
	for _, body := range []string{
		`{"id":"other","committer":"service","message":"wrong coordinate"}`,
		`{"committer":"service","message":"missing coordinate"}`,
		`{"id":"requested","metadata":{"kc.author":false}}`,
	} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/repositories/files/commits/requested" {
					t.Errorf("unexpected metadata read %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			endpoint, err := ParseDSN(server.URL + "/files")
			if err != nil {
				t.Fatal(err)
			}
			client, err := newAPIClient(endpoint, "access:secret")
			if err != nil {
				t.Fatal(err)
			}
			repo := &Repository{client: client, repoState: &repoState{}}
			if metadata, err := repo.ReadCommitMetadata("requested"); err == nil || metadata.Commit != "" {
				t.Fatalf("accepted unreliable commit basis: %#v err=%v", metadata, err)
			}
			if repo.verifiedCommit("requested") {
				t.Fatal("incorrect response marked the requested commit verified")
			}
		})
	}
}

func TestCommitMetadataRejectsEmptyCoordinateBeforeTransport(t *testing.T) {
	repo := &Repository{repoState: &repoState{}}
	if _, err := repo.ReadCommitMetadata(""); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		t.Fatalf("empty coordinate: %v", err)
	}
}
