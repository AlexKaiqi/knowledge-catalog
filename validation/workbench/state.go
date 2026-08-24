package scenario

import (
	"encoding/json"
	"reflect"
	"testing"

	"kc/catalog"
	"kc/controlplane"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/reader"
	"kc/repository"
)

type catalogWant struct {
	workspaces []workspaceWant
	archived   bool
}

type workspaceWant struct {
	id      string
	rev     int
	retired bool
	repos   []kernel.RepositoryID
}

func (wb *workbench) expectCatalog(t *testing.T, want catalogWant) {
	t.Helper()
	got := wb.snapshot()
	if got.CatalogID != CatalogID {
		t.Fatalf("catalog id %s", got.CatalogID)
	}
	if got.Archived != want.archived {
		t.Fatalf("archived=%v want %v", got.Archived, want.archived)
	}
	wantRepos := []string{string(Semantics), string(Personal), string(Metadata)}
	if !reflect.DeepEqual(got.Repositories, wantRepos) {
		t.Fatalf("repositories %#v", got.Repositories)
	}
	if len(got.Workspaces) != len(want.workspaces) {
		t.Fatalf("workspaces got %d want %d: %s", len(got.Workspaces), len(want.workspaces), pretty(got.Workspaces))
	}
	for _, v := range want.workspaces {
		found := false
		for _, gotView := range got.Workspaces {
			if gotView.WorkspaceID != v.id {
				continue
			}
			found = true
			if gotView.Revision != v.rev || gotView.Retired != v.retired {
				t.Fatalf("workspace %s rev=%d retired=%v", v.id, gotView.Revision, gotView.Retired)
			}
			if len(gotView.Sources) != len(v.repos) {
				t.Fatalf("workspace %s sources %#v", v.id, gotView.Sources)
			}
			for i, repo := range v.repos {
				if gotView.Sources[i].Repository != repo || gotView.Sources[i].Selector != MainRef {
					t.Fatalf("workspace %s source[%d]=%#v", v.id, i, gotView.Sources[i])
				}
			}
		}
		if !found {
			t.Fatalf("missing workspace %s in %s", v.id, pretty(got.Workspaces))
		}
	}
}

func (wb *workbench) expectUnchanged(t *testing.T, before catalog.CatalogState) {
	t.Helper()
	after := wb.snapshot()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("catalog mutated\nbefore %s\nafter %s", pretty(before), pretty(after))
	}
}

func expectCode(t *testing.T, err error, code kernel.ErrorCode) {
	t.Helper()
	testkit.ExpectCode(t, err, code)
}

func pretty(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func findFederated(values []reader.FederatedValue, repo kernel.RepositoryID) (reader.FederatedValue, bool) {
	for _, v := range values {
		if v.Repository == repo {
			return v, true
		}
	}
	return reader.FederatedValue{}, false
}

func mustResolve(t *testing.T, repo repository.Repository, object kernel.ObjectID, commit kernel.CommitID, status repository.ResolutionStatus) repository.Resolution {
	t.Helper()
	res, err := repo.Resolve(object, commit)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != status {
		t.Fatalf("%s at %s status %s", object, commit, res.Status)
	}
	return res
}

func readPreview(t *testing.T, wb *workbench, preview controlplane.Preview, object kernel.ObjectID) []reader.FederatedValue {
	t.Helper()
	got, err := reader.Open(wb.catalog.RequireKnowledge, reader.WorkspacePin{
		WorkspaceID:  preview.WorkspaceID,
		Repositories: preview.Repositories,
	}).Read(object, nil)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
