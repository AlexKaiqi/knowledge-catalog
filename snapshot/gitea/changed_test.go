package gitea

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"kc/kernel"
)

func TestChangedPathsUsesCompareAndIncludesBothSidesOfRename(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/core/compare/older...newer" ||
			r.URL.Query().Get("limit") != "1000" || r.URL.Query().Get("files") != "true" {
			t.Fatalf("compare request = %s", r.URL.String())
		}
		_, _ = w.Write([]byte(`{"commits":[{"files":[{"filename":"b.txt"},{"filename":"new.txt","previous_filename":"old.txt"}]},{"files":[{"filename":"b.txt"}]}]}`))
	}))
	t.Cleanup(server.Close)
	repo := &Repository{
		id:  "kr://acme/core",
		ep:  Endpoint{API: server.URL, Owner: "acme", Name: "core"},
		cli: newClient(server.URL, ""),
	}
	got, err := repo.ChangedPaths(kernel.CommitID("older"), kernel.CommitID("newer"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"b.txt", "new.txt", "old.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("changed paths = %v, want %v", got, want)
	}
}

func TestChangedPathsEqualBasisDoesNotCallProvider(t *testing.T) {
	repo := &Repository{}
	got, err := repo.ChangedPaths("same", "same")
	if err != nil || len(got) != 0 {
		t.Fatalf("equal basis changed paths = %v, %v", got, err)
	}
}
