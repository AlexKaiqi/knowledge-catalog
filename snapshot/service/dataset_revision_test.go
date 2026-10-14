package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"testing"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
)

func TestNamedDatasetResolveSelectsAcceptedRevisionWithoutInventory(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	h := f.handler()
	base := "/snapshot/v1/datasets/files"
	datasetHTTPCall(t, h, http.MethodPost, "/snapshot/v1/datasets", datasetHTTPDefinition(1, f.source.ID()), http.StatusOK)
	f.source.head = "c2"
	datasetHTTPCall(t, h, http.MethodPost, "/snapshot/v1/datasets", datasetHTTPDefinition(3, f.source.ID()), http.StatusOK)
	f.rules.rules = []access.Rule{{ID: "consume", Principal: "alice", Dataset: "files", Actions: []string{"dataset.resolve", "file.read"}}}

	old := datasetHTTPCall(t, h, http.MethodPost, base+"/resolve", map[string]any{"revision": 1}, http.StatusOK)
	if old["revision"] != float64(1) || old["ref"] != "v1" || old["repositories"].(map[string]any)[string(f.source.ID())] != "c1" {
		t.Fatalf("accepted historical basis changed: %#v", old)
	}
	current := datasetHTTPCall(t, h, http.MethodPost, base+"/resolve", map[string]any{}, http.StatusOK)
	if current["revision"] != float64(3) || current["repositories"].(map[string]any)[string(f.source.ID())] != "c2" {
		t.Fatalf("omitted revision did not select current: %#v", current)
	}
	datasetHTTPCall(t, h, http.MethodPost, base+"/check", map[string]any{"revision": 1}, http.StatusBadRequest)
	datasetHTTPCall(t, h, http.MethodPost, base+"/check", map[string]any{"pin": old}, http.StatusOK)
	datasetHTTPCall(t, h, http.MethodGet, base+"?revision=1", nil, http.StatusForbidden)
	datasetHTTPCall(t, h, http.MethodPost, base+"/resolve", map[string]any{"revision": 2}, http.StatusNotFound)
	adoptions := 0
	f.resources.EnsureDataset = func(context.Context, identity.HTTPIdentity, dataset.Service, string) error {
		adoptions++
		return nil
	}
	for _, revision := range []any{0, -1, 1.5, "1", nil} {
		t.Run("invalid-"+string(mustRevisionJSON(t, revision)), func(t *testing.T) {
			result := datasetHTTPCall(t, h, http.MethodPost, base+"/resolve", map[string]any{"revision": revision}, http.StatusBadRequest)
			if result["error"].(map[string]any)["code"] != string(kernel.ErrUsageInvalid) {
				t.Fatalf("invalid revision accepted: %#v", result)
			}
		})
	}
	datasetHTTPCall(t, h, http.MethodPost, base+"/resolve", map[string]any{"revision": 1, "pin": old}, http.StatusBadRequest)
	if adoptions != 0 {
		t.Fatalf("invalid selectors invoked recipe adoption %d times", adoptions)
	}
	f.rules.rules = nil
	datasetHTTPCall(t, h, http.MethodPost, base+"/resolve", map[string]any{"revision": 1}, http.StatusForbidden)
}

func mustRevisionJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestNamedDatasetHistoricalResolveHonorsCurrentLifecycle(t *testing.T) {
	for _, state := range []string{"retired", "unregistered", "unavailable"} {
		t.Run(state, func(t *testing.T) {
			f := newDatasetHTTPFixture(t)
			h := f.handler()
			datasetHTTPCall(t, h, http.MethodPost, "/snapshot/v1/datasets", datasetHTTPDefinition(1, f.source.ID()), http.StatusOK)
			pin := datasetHTTPPin(t, datasetHTTPCall(t, h, http.MethodPost, "/snapshot/v1/datasets/files/resolve", map[string]any{"revision": 1}, http.StatusOK))
			if pin.Revision != 1 || pin.Repositories[f.source.ID()] != "c1" {
				t.Fatalf("historical lifecycle fixture did not start from an accepted basis: %#v", pin)
			}
			if state == "retired" {
				if err := f.catalog.RetireKnowledgeSet("files"); err != nil {
					t.Fatal(err)
				}
			} else if state == "unregistered" {
				if err := f.catalog.UnregisterRepository(f.source.ID()); err != nil {
					t.Fatal(err)
				}
			} else {
				// The accepted release's backing Store becomes unavailable, while
				// the published registry record and caller's grants remain intact.
				f.resources.Sources.(snapshot.Registry).Delete(f.source.ID())
			}
			before := dataset.NormalizeState(f.catalog.DumpState())
			result := datasetHTTPCall(t, h, http.MethodPost, "/snapshot/v1/datasets/files/resolve", map[string]any{"revision": 1}, http.StatusBadRequest)
			if result["error"].(map[string]any)["code"] != string(kernel.ErrKnowledgeSetInvalid) {
				t.Fatalf("historical resolve replaced its current lifecycle error: %#v", result)
			}
			assertDatasetHTTPState(t, f, before)
		})
	}
}

func TestHistoricalDatasetDoesNotResolveAnUnfixedAcceptedSource(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	h := f.handler()
	datasetHTTPCall(t, h, http.MethodPost, "/snapshot/v1/datasets", datasetHTTPDefinition(1, f.source.ID()), http.StatusOK)
	f.source.head = "c2"
	datasetHTTPCall(t, h, http.MethodPost, "/snapshot/v1/datasets", datasetHTTPDefinition(3, f.source.ID()), http.StatusOK)
	state := f.catalog.DumpState()
	for i := range state.DatasetVersions {
		if state.DatasetVersions[i].Revision == 1 {
			state.DatasetVersions[i].Sources[0].Commit = ""
		}
	}
	f.catalog.LoadState(state)
	datasetHTTPCall(t, h, http.MethodPost, "/snapshot/v1/datasets/files/resolve", map[string]any{"revision": 1}, http.StatusBadRequest)
}

func TestExplicitDatasetRevisionDoesNotAdoptARecipe(t *testing.T) {
	for _, revision := range []int{1, 2} {
		t.Run(strconv.Itoa(revision), func(t *testing.T) {
			f := newDatasetHTTPFixture(t)
			f.source.head = "c2"
			adoptions := 0
			f.resources.EnsureDataset = func(_ context.Context, _ identity.HTTPIdentity, cat dataset.Service, id string) error {
				adoptions++
				_, err := cat.DefineKnowledgeSet(id, 1, []dataset.KnowledgeSetSource{{Repository: f.source.ID(), Selector: snapshot.DefaultRef, File: "plain.txt", Target: "docs/plain.txt"}})
				return err
			}
			before := dataset.NormalizeState(f.catalog.DumpState())
			h := f.handler()
			base := "/snapshot/v1/datasets/files/resolve"
			result := datasetHTTPCall(t, h, http.MethodPost, base, map[string]any{"revision": revision}, http.StatusBadRequest)
			if result["error"].(map[string]any)["code"] != string(kernel.ErrKnowledgeSetInvalid) {
				t.Fatalf("missing Dataset did not retain its lifecycle error: %#v", result)
			}
			if adoptions != 0 || !reflect.DeepEqual(dataset.NormalizeState(f.catalog.DumpState()), before) {
				t.Fatalf("explicit revision adopted a recipe: calls=%d state=%#v", adoptions, f.catalog.DumpState())
			}
			current := datasetHTTPCall(t, h, http.MethodPost, base, map[string]any{}, http.StatusOK)
			if adoptions != 1 || current["revision"] != float64(1) || current["repositories"].(map[string]any)[string(f.source.ID())] != "c2" {
				t.Fatalf("default resolve lost current recipe adoption: calls=%d result=%#v", adoptions, current)
			}
		})
	}
}
