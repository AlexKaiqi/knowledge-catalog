package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	localauth "kc/identity/local"
	"kc/internal/testkit"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
	"kc/snapshot/service"
)

type datasetHTTPSource struct {
	id          kernel.RepositoryID
	head        kernel.CommitID
	inspections int
	files       map[kernel.CommitID]map[string][]byte
}

func (s *datasetHTTPSource) ID() kernel.RepositoryID { return s.id }
func (s *datasetHTTPSource) Head(string) (kernel.CommitID, error) {
	s.inspections++
	return s.head, nil
}
func (s *datasetHTTPSource) GetRef(string) (kernel.CommitID, bool) {
	s.inspections++
	return s.head, true
}
func (s *datasetHTTPSource) HasCommit(id kernel.CommitID) bool {
	s.inspections++
	return id == "c1" || id == "c2"
}
func (s *datasetHTTPSource) CreateRef(string, kernel.CommitID) error { return nil }
func (s *datasetHTTPSource) Merge(string, kernel.CommitID, kernel.CommitID) (kernel.CommitID, error) {
	return "", fmt.Errorf("unused")
}
func (s *datasetHTTPSource) Archived() bool {
	s.inspections++
	return false
}
func (s *datasetHTTPSource) Archive() error { return nil }

func (s *datasetHTTPSource) ReadFile(file string, commit kernel.CommitID) ([]byte, error) {
	body, ok := s.files[commit][file]
	if !ok {
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "file %s is missing from %s", file, commit)
	}
	return append([]byte{}, body...), nil
}

func (s *datasetHTTPSource) ListFiles(commit kernel.CommitID) ([]string, error) {
	files := []string{}
	for file := range s.files[commit] {
		files = append(files, file)
	}
	return files, nil
}

type datasetHTTPRules struct{ rules []access.Rule }

func (s *datasetHTTPRules) ReadRules(context.Context) ([]access.Rule, error) { return s.rules, nil }
func (s *datasetHTTPRules) UpdateRules(ctx context.Context, update func([]access.Rule) ([]access.Rule, error)) error {
	rules, err := update(s.rules)
	if err == nil {
		s.rules = rules
	}
	return err
}

type datasetHTTPFixture struct {
	catalog   dataset.Service
	source    *datasetHTTPSource
	rules     *datasetHTTPRules
	resources service.Resources
	authority snapshot.Store
}

func newDatasetHTTPFixture(t *testing.T) *datasetHTTPFixture {
	t.Helper()
	store := snapshot.NewRegistry()
	source := &datasetHTTPSource{id: "kr://files/source", head: "c1", files: map[kernel.CommitID]map[string][]byte{"c1": {"plain.txt": []byte("plain version one")}, "c2": {"plain.txt": []byte("plain version two")}}}
	if err := store.Add(source); err != nil {
		t.Fatal(err)
	}
	authority := testkit.MakeTreeStore(t, "kr://fixture/A-authority")
	registry, err := dataset.CreateSnapshotRegistry(authority, "")
	if err != nil {
		t.Fatal(err)
	}
	cat, err := dataset.NewService(store, registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.RegisterRepository(source.ID()); err != nil {
		t.Fatal(err)
	}
	// Ordinary read and Dataset management do not permit publishing a private
	// source. This fixture explicitly permits its one source branch.
	rules := &datasetHTTPRules{rules: []access.Rule{
		{ID: "allowed", Principal: "alice", Actions: []string{"inventory.read", "dataset.manage", "dataset.resolve", "file.read"}},
		{ID: "source-publication", Principal: "alice", Repo: string(source.ID()), Ref: snapshot.DefaultRef, Actions: []string{access.RepositoryDatasetPublish}},
	}}
	return &datasetHTTPFixture{catalog: cat, source: source, rules: rules, authority: authority, resources: service.Resources{Datasets: cat, Sources: store, Access: access.NewService(rules)}}
}

func (f *datasetHTTPFixture) handler() service.Handler {
	return service.NewHandler(service.Config{Identity: service.IdentityOptions{Mode: "local", Authenticator: localauth.New()},
		Acquire: func(context.Context, identity.HTTPIdentity, string) (service.Resources, func(), error) {
			return f.resources, func() {}, nil
		},
	})
}

func datasetHTTPCall(t *testing.T, handler http.Handler, method, target string, body any, want int) map[string]any {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, target, &payload)
	r.Header.Set("X-Kc-As", "alice")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s %s HTTP %d, want %d: %s", method, target, w.Code, want, w.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("not JSON: %s", w.Body.String())
	}
	return result
}

func datasetHTTPDefinition(revision int, source kernel.RepositoryID) map[string]any {
	return map[string]any{"dataset": "files", "revision": revision, "sources": []dataset.DatasetSource{{Repository: source, Selector: snapshot.DefaultRef, File: "plain.txt", Target: "docs/plain.txt"}}}
}

func assertDatasetHTTPState(t *testing.T, f *datasetHTTPFixture, before dataset.State) {
	t.Helper()
	if after := dataset.NormalizeState(f.catalog.DumpState()); !reflect.DeepEqual(after, before) {
		t.Fatalf("request changed Dataset authority: before=%#v after=%#v", before, after)
	}
}

func datasetHTTPPin(t *testing.T, response map[string]any) dataset.ResolvedDataset {
	t.Helper()
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var pin dataset.ResolvedDataset
	if err := dataset.DecodeJSON(raw, &pin); err != nil {
		t.Fatal(err)
	}
	return pin
}

func TestDatasetHTTPStandaloneManagementLifecycle(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	h := f.handler()
	base := "/snapshot/v1/datasets"
	var list map[string]any
	created := datasetHTTPCall(t, h, "POST", base, datasetHTTPDefinition(1, f.source.ID()), 200)
	if created["setId"] != "files" || created["revision"] != float64(1) || created["disposition"] != "APPLIED" || len(created) != 4 {
		t.Fatalf("publication wire changed: %#v", created)
	}
	// A retry after a lost response replays the accepted revision.
	replayed := datasetHTTPCall(t, h, "POST", base, datasetHTTPDefinition(1, f.source.ID()), 200)
	if replayed["disposition"] != "REPLAYED" || replayed["revision"] != float64(1) || len(replayed) != 4 {
		t.Fatalf("retrying the accepted definition must replay it: %#v", replayed)
	}
	list = datasetHTTPCall(t, h, "GET", base, nil, 200)
	rows := list["datasets"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["itemCount"] != float64(1) {
		t.Fatalf("Dataset inventory lost file selection: %#v", list)
	}
	pin := datasetHTTPCall(t, h, "POST", base+"/files/resolve", map[string]any{}, 200)
	if pin["setId"] != "files" || pin["ref"] != "v1" {
		t.Fatalf("pin changed: %#v", pin)
	}
	datasetHTTPCall(t, h, "POST", base+"/files/check", map[string]any{"pin": pin}, 200)
	f.source.head = "c2"
	datasetHTTPCall(t, h, "POST", base, datasetHTTPDefinition(2, f.source.ID()), 200)
	old := datasetHTTPCall(t, h, "GET", base+"/files?revision=1&limit=1", nil, 200)
	if old["revision"] != float64(1) || old["servingRevision"] != float64(2) || old["continuation"] == nil {
		t.Fatalf("history lost its accepted basis: %#v", old)
	}
	if old["sources"].([]any)[0].(map[string]any)["commit"] != "c1" {
		t.Fatalf("historical source followed latest: %#v", old)
	}
	page := datasetHTTPCall(t, h, "GET", base+"/files?continuation="+url.QueryEscape(old["continuation"].(string)), nil, 200)
	if !reflect.DeepEqual(page["versions"], []any{float64(1)}) {
		t.Fatalf("history pagination changed: %#v", page)
	}
	replay := datasetHTTPCall(t, h, "POST", base+"/files/resolve", map[string]any{"pin": pin}, 200)
	if replay["repositories"].(map[string]any)[string(f.source.ID())] != "c1" {
		t.Fatalf("old pin drifted: %#v", replay)
	}
	pin["repositories"].(map[string]any)[string(f.source.ID())] = "c2"
	datasetHTTPCall(t, h, "POST", base+"/files/resolve", map[string]any{"pin": pin}, 400)
	datasetHTTPCall(t, h, "POST", base+"/files/retire", map[string]any{}, 200)
	datasetHTTPCall(t, h, "POST", base+"/files/resolve", map[string]any{}, 400)
	old = datasetHTTPCall(t, h, "GET", base+"/files?revision=1", nil, 200)
	if old["retired"] != true {
		t.Fatalf("history bypassed current retirement: %#v", old)
	}
}

func TestDatasetHTTPTemporaryFileGrantDoesNotAuthorizeResolve(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	before := dataset.NormalizeState(f.catalog.DumpState())
	f.source.inspections = 0
	f.rules.rules = []access.Rule{{ID: "read", Principal: "alice", Repo: string(f.source.ID()), Actions: []string{"file.read"}}}
	datasetHTTPCall(t, f.handler(), "POST", "/snapshot/v1/datasets:resolve", datasetHTTPDefinition(1, f.source.ID()), 403)
	assertDatasetHTTPState(t, f, before)
	if f.source.inspections != 0 {
		t.Fatal("file-only grant bypassed source resolution authorization")
	}
	f.rules.rules[0].Actions = append(f.rules.rules[0].Actions, "dataset.resolve")
	pin := datasetHTTPPin(t, datasetHTTPCall(t, f.handler(), "POST", "/snapshot/v1/datasets:resolve", datasetHTTPDefinition(1, f.source.ID()), 200))
	if pin.SetID != "files" || pin.Revision != 1 || pin.Repositories[f.source.ID()] != "c1" || len(pin.Items) != 1 || pin.Items[0].Target != "docs/plain.txt" {
		t.Fatalf("temporary source grant did not resolve its fixed scope: %#v", pin)
	}
	assertDatasetHTTPState(t, f, before)
}

func TestDatasetHTTPTemporarySourceCheckIsCompleteAndPolicyPluggable(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	f.source.files["c1"]["a"] = []byte("first selected file")
	second := &datasetHTTPSource{id: "kr://files/other", head: "c2", files: map[kernel.CommitID]map[string][]byte{"c2": {"b": []byte("second selected file")}}}
	store := f.resources.Sources.(snapshot.Registry)
	if err := store.Add(second); err != nil {
		t.Fatal(err)
	}
	if err := f.catalog.RegisterRepository(second.ID()); err != nil {
		t.Fatal(err)
	}
	body := datasetHTTPDefinition(1, f.source.ID())
	body["sources"] = []dataset.DatasetSource{{Repository: f.source.ID(), Selector: snapshot.DefaultRef, File: "a", Target: "a"}, {Repository: second.ID(), Selector: snapshot.DefaultRef, File: "b", Target: "b"}}
	before := dataset.NormalizeState(f.catalog.DumpState())
	f.rules.rules = []access.Rule{{ID: "one", Principal: "alice", Repo: string(f.source.ID()), Actions: []string{"dataset.resolve", "file.read"}}}
	datasetHTTPCall(t, f.handler(), "POST", "/snapshot/v1/datasets:resolve", body, 403)
	assertDatasetHTTPState(t, f, before)
	// All sources can resolve, but the second source cannot be consumed.
	// This must fail in the distinct fixed-pin file.read authorization phase.
	f.rules.rules = append(f.rules.rules, access.Rule{ID: "two", Principal: "alice", Repo: string(second.ID()), Actions: []string{"dataset.resolve"}})
	datasetHTTPCall(t, f.handler(), "POST", "/snapshot/v1/datasets:resolve", body, 403)
	assertDatasetHTTPState(t, f, before)
	f.rules.rules[1].Actions = append(f.rules.rules[1].Actions, "file.read")
	wantRepositories := map[kernel.RepositoryID]kernel.CommitID{f.source.ID(): "c1", second.ID(): "c2"}
	wantItems := []dataset.DatasetItem{{Target: "a", Repository: f.source.ID(), Commit: "c1", Kind: dataset.DatasetItemFile, File: "a"}, {Target: "b", Repository: second.ID(), Commit: "c2", Kind: dataset.DatasetItemFile, File: "b"}}
	assertPin := func(pin dataset.ResolvedDataset) {
		t.Helper()
		if pin.SetID != "files" || pin.Revision != 1 || !reflect.DeepEqual(pin.Repositories, wantRepositories) || !reflect.DeepEqual(pin.Items, wantItems) {
			t.Fatalf("temporary policy or response lost exact fixed scope: %#v", pin)
		}
	}
	assertPin(datasetHTTPPin(t, datasetHTTPCall(t, f.handler(), "POST", "/snapshot/v1/datasets:resolve", body, 200)))
	assertDatasetHTTPState(t, f, before)
	checks := []string{}
	f.resources.Authorize = func(_ context.Context, request service.Authorization) error {
		checks = append(checks, request.Query.Action)
		if request.Caller.Principal != "alice" || request.Query.Dataset != "" || request.Definition == nil {
			t.Fatalf("temporary label became a grant selector: %#v", request)
		}
		if request.Query.Action == "file.read" {
			if request.Pin == nil {
				t.Fatalf("policy did not get fixed source coordinates: %#v", request)
			}
			assertPin(*request.Pin)
		}
		return nil
	}
	datasetHTTPCall(t, f.handler(), "POST", "/snapshot/v1/datasets:resolve", body, 200)
	if !reflect.DeepEqual(checks, []string{"dataset.resolve", "file.read"}) {
		t.Fatalf("temporary source phases bypassed configured policy: %v", checks)
	}
	assertDatasetHTTPState(t, f, before)
	f.resources.Authorize = func(_ context.Context, request service.Authorization) error {
		if request.Query.Action == "file.read" {
			return kernel.Fail(kernel.ErrForbidden, "configured temporary consumption policy refused")
		}
		return nil
	}
	datasetHTTPCall(t, f.handler(), "POST", "/snapshot/v1/datasets:resolve", body, 403)
	assertDatasetHTTPState(t, f, before)
}

func TestDatasetHTTPDiscoveryMarkerReachesPolicyOnlyForResolve(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	if _, err := f.catalog.DefineDataset("files", 1, []dataset.DatasetSource{{Repository: f.source.ID(), Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	var discovered bool
	f.resources.Authorize = func(_ context.Context, request service.Authorization) error {
		discovered = request.InventoryDiscovery
		if !discovered {
			return kernel.Fail(kernel.ErrForbidden, "discovery proof required")
		}
		return nil
	}
	datasetHTTPCall(t, f.handler(), "POST", "/snapshot/v1/datasets/files/resolve", map[string]any{"inventoryDiscovery": true}, 200)
	if !discovered {
		t.Fatal("configured discovery policy marker was lost")
	}
	datasetHTTPCall(t, f.handler(), "POST", "/snapshot/v1/datasets/files/check", map[string]any{"inventoryDiscovery": true}, 400)
	f.resources.Authorize = nil
	f.rules.rules = nil
	datasetHTTPCall(t, f.handler(), "POST", "/snapshot/v1/datasets/files/resolve", map[string]any{"inventoryDiscovery": true}, 403)
}

func TestDatasetHTTPPublicationPreparesThenSwitchesOnce(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	var candidate dataset.Dataset
	f.resources.BeforePublish = func(_ context.Context, _ identity.HTTPIdentity, _ dataset.Service, definition dataset.Dataset) (dataset.Dataset, map[string]any, error) {
		if _, err := f.catalog.Set("files"); err == nil {
			t.Fatal("candidate became current before preparation")
		}
		if definition.Sources[0].Commit != "" {
			t.Fatal("portable recipe got a prematurely frozen source")
		}
		f.source.head = "c2"
		return definition, map[string]any{"recipeFile": ".kc-dataset.yaml", "setId": "hijack"}, nil
	}
	f.resources.PreparePublication = func(_ context.Context, _ identity.HTTPIdentity, definition dataset.Dataset) (dataset.Dataset, error) {
		candidate = definition
		if definition.Sources[0].Commit != "c2" {
			t.Fatal("publication missed the prepared source version")
		}
		if _, err := f.catalog.Set("files"); err == nil {
			t.Fatal("current pointer advanced during preparation")
		}
		return definition, nil
	}
	out := datasetHTTPCall(t, f.handler(), "POST", "/snapshot/v1/datasets", datasetHTTPDefinition(1, f.source.ID()), 200)
	if out["setId"] != "files" || out["recipeFile"] != ".kc-dataset.yaml" || len(candidate.Items) != 1 {
		t.Fatalf("publication receipt or scope changed: %#v", out)
	}
	accepted, err := f.catalog.DatasetVersion("files", 1)
	if err != nil || !reflect.DeepEqual(accepted, candidate) {
		t.Fatalf("prepared version differs from accepted record: %#v,%v", accepted, err)
	}
}

func TestDatasetHTTPPreparationFailureKeepsServingVersion(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	datasetHTTPCall(t, f.handler(), "POST", "/snapshot/v1/datasets", datasetHTTPDefinition(1, f.source.ID()), 200)
	f.resources.PreparePublication = func(context.Context, identity.HTTPIdentity, dataset.Dataset) (dataset.Dataset, error) {
		return dataset.Dataset{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "capability not ready")
	}
	datasetHTTPCall(t, f.handler(), "POST", "/snapshot/v1/datasets", datasetHTTPDefinition(2, f.source.ID()), 503)
	current, err := f.catalog.Set("files")
	if err != nil || current.Revision != 1 {
		t.Fatalf("failed preparation switched publication: %#v,%v", current, err)
	}
	if _, err := f.catalog.DatasetVersion("files", 2); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		t.Fatalf("unaccepted candidate became history: %v", err)
	}
}

func TestDatasetHTTPRejectsInvalidWireAndHistoryQueries(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	base := "/snapshot/v1/datasets"
	for _, body := range []any{map[string]any{"dataset": "files", "revision": 1, "sources": []any{map[string]any{"objectId": "fixture/a"}}}, map[string]any{"dataset": "files", "revision": 1, "sources": []any{}}, map[string]any{"dataset": "files", "revision": 1, "sources": []any{}, "extra": true}} {
		datasetHTTPCall(t, f.handler(), "POST", base, body, 400)
	}
	datasetHTTPCall(t, f.handler(), "POST", base, datasetHTTPDefinition(1, f.source.ID()), 200)
	for _, query := range []string{"revision=0", "revision=-1", "revision=nope", "limit=201", "limit=-1", "limit=nope", "continuation=invalid"} {
		datasetHTTPCall(t, f.handler(), "GET", base+"/files?"+query, nil, 400)
	}
	datasetHTTPCall(t, f.handler(), "GET", base+"/files?revision=2", nil, 404)
}

func TestDatasetHTTPNamedResolveDoesNotGrantTemporarySources(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	if _, err := f.catalog.DefineDataset("files", 1, []dataset.DatasetSource{{Repository: f.source.ID(), Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	before := dataset.NormalizeState(f.catalog.DumpState())
	f.rules.rules = []access.Rule{{ID: "named", Principal: "alice", Dataset: "files", Actions: []string{"dataset.resolve", "file.read"}}}
	pin := datasetHTTPPin(t, datasetHTTPCall(t, f.handler(), "POST", "/snapshot/v1/datasets/files/resolve", map[string]any{}, 200))
	if pin.SetID != "files" || pin.Revision != 1 || pin.Repositories[f.source.ID()] != "c1" {
		t.Fatalf("named grant did not resolve the accepted release: %#v", pin)
	}
	datasetHTTPCall(t, f.handler(), "POST", "/snapshot/v1/datasets:resolve", datasetHTTPDefinition(1, f.source.ID()), 403)
	assertDatasetHTTPState(t, f, before)
}

func TestDatasetHTTPManagementGrantDoesNotCrossDatasetTargets(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	h := f.handler()
	base := "/snapshot/v1/datasets"
	datasetHTTPCall(t, h, http.MethodPost, base, datasetHTTPDefinition(1, f.source.ID()), http.StatusOK)
	other := datasetHTTPDefinition(1, f.source.ID())
	other["dataset"] = "other"
	datasetHTTPCall(t, h, http.MethodPost, base, other, http.StatusOK)
	f.rules.rules = []access.Rule{{ID: "manage-files", Principal: "alice", Dataset: "files", Actions: []string{"dataset.manage"}}, sourcePublicationGrant(f)}
	datasetHTTPCall(t, h, http.MethodPost, base, datasetHTTPDefinition(2, f.source.ID()), http.StatusOK)
	accepted, err := f.catalog.DatasetVersion("files", 2)
	if err != nil || accepted.SetID != "files" || accepted.Revision != 2 || len(accepted.Items) != 1 {
		t.Fatalf("Dataset manager did not publish its own target: %#v %v", accepted, err)
	}
	before := dataset.NormalizeState(f.catalog.DumpState())
	f.source.inspections = 0
	other["revision"] = 2
	datasetHTTPCall(t, h, http.MethodPost, base, other, http.StatusForbidden)
	assertDatasetHTTPState(t, f, before)
	datasetHTTPCall(t, h, http.MethodPost, base+"/other/retire", map[string]any{}, http.StatusForbidden)
	assertDatasetHTTPState(t, f, before)
	if f.source.inspections != 0 {
		t.Fatal("cross-Dataset management reached source preparation")
	}
	datasetHTTPCall(t, h, http.MethodPost, base+"/files/retire", map[string]any{}, http.StatusOK)
	current, err := f.catalog.Set("files")
	untouched, otherErr := f.catalog.Set("other")
	if err != nil || !current.Retired || otherErr != nil || untouched.Revision != 1 || untouched.Retired {
		t.Fatalf("scoped retirement changed another Dataset: own=%#v err=%v other=%#v err=%v", current, err, untouched, otherErr)
	}
}

func TestDatasetHTTPBeforePublishCannotRedirectOrContinueAfterRevocation(t *testing.T) {
	for _, mode := range []string{"identity", "revision", "revoked", "invalid-revision"} {
		t.Run(mode, func(t *testing.T) {
			f := newDatasetHTTPFixture(t)
			before := dataset.NormalizeState(f.catalog.DumpState())
			called := false
			inspectionsAtCallback := 0
			f.resources.BeforePublish = func(_ context.Context, _ identity.HTTPIdentity, _ dataset.Service, definition dataset.Dataset) (dataset.Dataset, map[string]any, error) {
				called = true
				inspectionsAtCallback = f.source.inspections
				if mode == "identity" {
					definition.SetID = "other"
				} else if mode == "revision" {
					definition.Revision++
				} else {
					f.rules.rules = nil
				}
				return definition, nil, nil
			}
			revision, status := 1, 403
			if mode == "identity" || mode == "revision" {
				status = 409
			}
			if mode == "invalid-revision" {
				revision, status = 0, 400
			}
			datasetHTTPCall(t, f.handler(), "POST", "/snapshot/v1/datasets", datasetHTTPDefinition(revision, f.source.ID()), status)
			if called != (mode != "invalid-revision") {
				t.Fatalf("publication preparation entered the wrong phase: mode=%s called=%v", mode, called)
			}
			if called && f.source.inspections != inspectionsAtCallback {
				t.Fatal("rejected portable preparation continued inspecting sources")
			}
			assertDatasetHTTPState(t, f, before)
		})
	}
}

func TestDatasetHTTPDiagnosticBodyScopeDoesNotSelectTemporaryGrants(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	observed := []string{}
	f.resources.Authorize = func(_ context.Context, request service.Authorization) error {
		if request.Definition != nil && request.Query.Dataset != "" {
			t.Fatalf("diagnostic recipe label entered authorization: %#v", request)
		}
		return nil
	}
	h := service.NewHandler(service.Config{
		Identity: service.IdentityOptions{Mode: "local", Authenticator: localauth.New()},
		Acquire: func(context.Context, identity.HTTPIdentity, string) (service.Resources, func(), error) {
			return f.resources, func() {}, nil
		},
		Observe: func(ctx context.Context, _ identity.HTTPIdentity, operation string) (context.Context, func(any, error)) {
			metadata := service.ContextMetadata(ctx)
			if metadata.Dataset != "files" {
				t.Fatalf("body scope missing from diagnostic aspect: %#v", metadata)
			}
			observed = append(observed, operation)
			return ctx, nil
		},
	})
	datasetHTTPCall(t, h, "POST", "/snapshot/v1/datasets", datasetHTTPDefinition(1, f.source.ID()), 200)
	datasetHTTPCall(t, h, "POST", "/snapshot/v1/datasets:resolve", datasetHTTPDefinition(1, f.source.ID()), 200)
	if !reflect.DeepEqual(observed, []string{"dataset-define", "pin-source"}) {
		t.Fatalf("operation aspects changed: %v", observed)
	}
}
