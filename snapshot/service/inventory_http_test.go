package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"kc/identity"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	snapshotcli "kc/snapshot/cli"
	snapshotclient "kc/snapshot/client"
	"kc/snapshot/dataset"
	"kc/snapshot/service"
)

func managementFixture(t *testing.T) (*datasetHTTPFixture, *datasetHTTPSource) {
	t.Helper()
	f := newDatasetHTTPFixture(t)
	f.rules.rules[0].Actions = append(f.rules.rules[0].Actions, "repositories.manage", "registry.audit.read")
	other := &datasetHTTPSource{id: "kr://management/other", head: "c1"}
	if err := f.resources.Sources.(snapshot.Registry).Add(other); err != nil {
		t.Fatal(err)
	}
	return f, other
}

func TestSnapshotStandaloneInventoryUsesOnlyRegisteredAuthority(t *testing.T) {
	f, other := managementFixture(t)
	if _, err := f.catalog.DefineDataset("delivery", 1, []dataset.DatasetSource{{Repository: f.source.id, Selector: snapshot.DefaultRef, Path: dataset.MountPath("docs")}}); err != nil {
		t.Fatal(err)
	}
	before := dataset.NormalizeState(f.catalog.DumpState())
	inspections := f.source.inspections + other.inspections
	result := datasetHTTPCall(t, f.handler(), http.MethodGet, "/snapshot/v1/inventory", nil, http.StatusOK)
	if !reflect.DeepEqual(result["repositories"], []any{map[string]any{"id": string(f.source.id)}}) || len(result) != 2 {
		t.Fatalf("inventory returned connected but unregistered sources or another authority: %#v", result)
	}
	rows := result["datasets"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["id"] != "delivery" || rows[0].(map[string]any)["revision"] != float64(1) {
		t.Fatalf("inventory lost accepted Dataset records: %#v", result)
	}
	assertDatasetHTTPState(t, f, before)
	if f.source.inspections+other.inspections != inspections {
		t.Fatal("bare inventory inspected source state or bytes")
	}
}

func TestSnapshotStandaloneRegistrationChangesOnlyMembershipAndCurrentAdmission(t *testing.T) {
	f, other := managementFixture(t)
	h := f.handler()
	attached := datasetHTTPCall(t, h, http.MethodPost, "/snapshot/v1/repositories", map[string]any{"repository": other.id}, http.StatusOK)
	if !reflect.DeepEqual(attached, map[string]any{"repositoryId": string(other.id)}) || !f.catalog.HasRepository(other.id) {
		t.Fatalf("registration was not accepted by the Dataset authority: %#v", attached)
	}
	if _, err := f.catalog.DefineDataset("delivery", 1, []dataset.DatasetSource{{Repository: other.id, Selector: snapshot.DefaultRef, Path: dataset.MountPath("docs")}}); err != nil {
		t.Fatal(err)
	}
	accepted, err := f.catalog.ResolveDataset("delivery")
	if err != nil {
		t.Fatal(err)
	}
	head := other.head
	definition, err := f.catalog.DatasetVersion("delivery", 1)
	if err != nil {
		t.Fatal(err)
	}
	detached := datasetHTTPCall(t, h, http.MethodDelete, "/snapshot/v1/repositories/kr%3A%2F%2Fmanagement%2Fother", nil, http.StatusOK)
	if !reflect.DeepEqual(detached, map[string]any{"repositoryId": string(other.id), "detached": true}) || f.catalog.HasRepository(other.id) || other.head != head {
		t.Fatalf("detach changed source content or did not remove membership: %#v", detached)
	}
	if after, err := f.catalog.DatasetVersion("delivery", 1); err != nil || !reflect.DeepEqual(after, definition) {
		t.Fatal("detach changed an accepted Dataset release", after, err)
	}
	if _, err := f.catalog.ReplayPublished(accepted); kernel.CodeOf(err) != kernel.ErrDatasetInvalid {
		t.Fatalf("old accepted pin bypassed current member admission: %v", err)
	}
}

func TestSnapshotStandaloneManagementAuthorizationAndInvalidInputsHaveNoSideEffects(t *testing.T) {
	for _, request := range []struct {
		method, path string
		body         any
		status       int
	}{
		{http.MethodGet, "/snapshot/v1/inventory", nil, http.StatusForbidden},
		{http.MethodGet, "/snapshot/v1/audit", nil, http.StatusForbidden},
		{http.MethodPost, "/snapshot/v1/repositories", map[string]any{"repository": "kr://management/other"}, http.StatusForbidden},
		{http.MethodDelete, "/snapshot/v1/repositories/kr%3A%2F%2Ffiles%2Fsource", nil, http.StatusForbidden},
	} {
		t.Run(request.method+request.path, func(t *testing.T) {
			f, other := managementFixture(t)
			f.rules.rules = []access.Rule{{ID: "dataset-only", Principal: "alice", Dataset: "delivery", Actions: []string{"inventory.read", "registry.audit.read", "repositories.manage"}}}
			before := dataset.NormalizeState(f.catalog.DumpState())
			inspections := f.source.inspections + other.inspections
			datasetHTTPCall(t, f.handler(), request.method, request.path, request.body, request.status)
			assertDatasetHTTPState(t, f, before)
			if f.source.inspections+other.inspections != inspections {
				t.Fatal("denied management reached source inspection")
			}
		})
	}
	for _, request := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/snapshot/v1/audit?limit=abc", nil},
		{http.MethodGet, "/snapshot/v1/audit?limit=-1", nil},
		{http.MethodGet, "/snapshot/v1/audit?limit=201", nil},
		{http.MethodGet, "/snapshot/v1/audit?continuation=not-a-history-cursor", nil},
		{http.MethodGet, "/snapshot/v1/inventory?Catalog=foreign", nil},
		{http.MethodPost, "/snapshot/v1/repositories", map[string]any{"repository": ""}},
		{http.MethodPost, "/snapshot/v1/repositories", map[string]any{"repository": "kr://management/other", "catalog": "another"}},
	} {
		t.Run("invalid "+request.path, func(t *testing.T) {
			f, other := managementFixture(t)
			before := dataset.NormalizeState(f.catalog.DumpState())
			inspections := f.source.inspections + other.inspections
			datasetHTTPCall(t, f.handler(), request.method, request.path, request.body, http.StatusBadRequest)
			assertDatasetHTTPState(t, f, before)
			if f.source.inspections+other.inspections != inspections {
				t.Fatal("invalid management inspected a source")
			}
		})
	}
}

type unavailableAuditAuthority struct {
	snapshot.Store
	snapshot.TreeStore
	snapshot.HistoryStore
	fail bool
}

func (authority *unavailableAuditAuthority) CommitHistory(commit kernel.CommitID, limit int) ([]kernel.CommitID, error) {
	if authority.fail {
		return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "registry history unavailable")
	}
	return authority.HistoryStore.CommitHistory(commit, limit)
}

func TestSnapshotStandaloneAuditPropagatesAuthorityFailureAndMissingCapability(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "history failure", true: "missing history"}[missing], func(t *testing.T) {
			base := testkit.MakeTreeStore(t, "kr://audit/authority")
			tree, ok := snapshot.TreeStoreOf(base)
			if !ok {
				t.Fatal("fixture requires literal tree capability")
			}
			history, ok := snapshot.HistoryStoreOf(base)
			if !ok {
				t.Fatal("fixture requires real first-parent history")
			}
			backend := &unavailableAuditAuthority{Store: base, TreeStore: tree, HistoryStore: history}
			if _, err := dataset.CreateSnapshotRegistry(backend, ""); err != nil {
				t.Fatal(err)
			}
			var reader snapshot.Store = backend
			if missing {
				reader = struct {
					snapshot.Store
					snapshot.TreeStore
				}{Store: base, TreeStore: tree}
			}
			registry, err := dataset.OpenSnapshotRegistry(reader, "")
			if err != nil {
				t.Fatal(err)
			}
			cat, err := dataset.NewService(snapshot.NewRegistry(), registry)
			if err != nil {
				t.Fatal(err)
			}
			backend.fail = true
			rules := &datasetHTTPRules{rules: []access.Rule{{ID: "audit", Principal: "alice", Actions: []string{"registry.audit.read"}}}}
			resources := service.Resources{Datasets: cat, Access: access.NewService(rules)}
			released := 0
			handler := service.NewHandler(service.Config{Identity: service.IdentityOptions{Local: true}, Acquire: func(context.Context, identity.HTTPIdentity, string) (service.Resources, func(), error) {
				return resources, func() { released++ }, nil
			}})
			status, code := http.StatusServiceUnavailable, kernel.ErrTemporaryUnavailable
			if missing {
				status, code = http.StatusBadRequest, kernel.ErrCapabilityUnsatisfied
			}
			result := datasetHTTPCall(t, handler, http.MethodGet, "/snapshot/v1/audit", nil, status)
			if result["error"].(map[string]any)["code"] != string(code) || result["entries"] != nil || released != 1 {
				t.Fatalf("failed authority became successful empty audit or lost its lease: %#v released=%d", result, released)
			}
		})
	}
}

func TestSnapshotStandaloneInventoryPreservesOnlyOptionalHostDescriptions(t *testing.T) {
	f, _ := managementFixture(t)
	f.resources.DiscoveryWorkspaceID = "discovery"
	described := []kernel.RepositoryID{}
	f.resources.DescribeRepository = func(_ context.Context, caller identity.HTTPIdentity, repository kernel.RepositoryID) (map[string]any, error) {
		if caller.Principal != "alice" {
			t.Fatal("description lost verified identity")
		}
		described = append(described, repository)
		return map[string]any{"id": "forged", "schemaCount": 3}, nil
	}
	result := datasetHTTPCall(t, f.handler(), http.MethodGet, "/snapshot/v1/inventory", nil, http.StatusOK)
	if !reflect.DeepEqual(result["repositories"], []any{map[string]any{"id": string(f.source.id), "schemaCount": float64(3)}}) || result["discoveryWorkspaceId"] != "discovery" || !reflect.DeepEqual(described, []kernel.RepositoryID{f.source.id}) {
		t.Fatalf("host descriptions changed authoritative membership: %#v described=%#v", result, described)
	}
}

func TestSnapshotStandaloneAuditAuthorizationIsSeparateAndCurrent(t *testing.T) {
	f, _ := managementFixture(t)
	f.rules.rules[0].Actions = []string{"registry.audit.read"}
	handler := f.handler()
	datasetHTTPCall(t, handler, http.MethodGet, "/snapshot/v1/audit?limit=0", nil, http.StatusOK)
	datasetHTTPCall(t, handler, http.MethodGet, "/snapshot/v1/inventory", nil, http.StatusForbidden)
	f.rules.rules = nil
	datasetHTTPCall(t, handler, http.MethodGet, "/snapshot/v1/audit", nil, http.StatusForbidden)
}

func TestSnapshotStandaloneRegistrationRechecksAfterHooksBeforeHostValidation(t *testing.T) {
	f, other := managementFixture(t)
	before := dataset.NormalizeState(f.catalog.DumpState())
	validated, hooks := 0, 0
	f.resources.BeforeAttach = func(context.Context, identity.HTTPIdentity, kernel.RepositoryID) error { validated++; return nil }
	handler := service.NewHandler(service.Config{Identity: service.IdentityOptions{Local: true}, Acquire: func(context.Context, identity.HTTPIdentity, string) (service.Resources, func(), error) {
		return f.resources, func() {}, nil
	},
		Around: func(ctx context.Context, caller identity.HTTPIdentity, operation string, run func() (any, error)) (any, error) {
			hooks++
			if operation != "repository-attach" || service.ContextMetadata(ctx).Repository != string(other.id) || caller.Principal != "alice" {
				t.Fatal("registration hook lost repository or verified identity")
			}
			f.rules.rules[0].Actions = []string{"inventory.read"}
			return run()
		},
	})
	inspections := other.inspections
	datasetHTTPCall(t, handler, http.MethodPost, "/snapshot/v1/repositories", map[string]any{"repository": other.id}, http.StatusForbidden)
	if validated != 0 || other.inspections != inspections || hooks != 1 {
		t.Fatalf("revoked pre-hook reached validation: validated=%d inspections=%d hooks=%d", validated, other.inspections, hooks)
	}
	assertDatasetHTTPState(t, f, before)
}

func TestSnapshotStandaloneRegistrationRejectsHostBindingFailureBeforeMembership(t *testing.T) {
	f, other := managementFixture(t)
	before := dataset.NormalizeState(f.catalog.DumpState())
	f.resources.BeforeAttach = func(_ context.Context, caller identity.HTTPIdentity, repository kernel.RepositoryID) error {
		if caller.Principal != "alice" || repository != other.id {
			t.Fatal("host validation lost checked scope")
		}
		return kernel.Fail(kernel.ErrPreconditionFailed, "durable source binding unavailable")
	}
	inspections := other.inspections
	result := datasetHTTPCall(t, f.handler(), http.MethodPost, "/snapshot/v1/repositories", map[string]any{"repository": other.id}, http.StatusConflict)
	if result["error"].(map[string]any)["message"] != "durable source binding unavailable" || other.inspections != inspections {
		t.Fatalf("binding refusal was changed or source inspected: %#v", result)
	}
	assertDatasetHTTPState(t, f, before)
}

type managementTransport struct{ client *http.Client }

func (transport managementTransport) Do(ctx context.Context, _ string, request *http.Request) (*http.Response, error) {
	request = request.Clone(ctx)
	request.Header.Set("X-Kc-As", "alice")
	return transport.client.Do(request)
}

func TestIndependentSnapshotCLIUsesRealStandaloneManagementHandler(t *testing.T) {
	f, other := managementFixture(t)
	// Use the actual lakeFS Snapshot adapter and its HTTP fixture for accepted
	// commit provenance; the generic memory fixture has no metadata capability.
	backend := testkit.NewLakeFSFake(t)
	authority, err := snapshot.OpenLakeFS("kr://management/authority", backend.DSN(backend.NewRepo()), backend.Credential())
	if err != nil {
		t.Fatal(err)
	}
	registry, err := dataset.CreateSnapshotRegistry(authority, "")
	if err != nil {
		t.Fatal(err)
	}
	f.catalog, err = dataset.NewService(f.resources.Sources.(snapshot.Registry), registry)
	if err != nil {
		t.Fatal(err)
	}
	f.catalog.SetStamp("alice", "initial-registration", "allowed")
	if err := f.catalog.RegisterRepository(f.source.id); err != nil {
		t.Fatal(err)
	}
	f.resources.Datasets = f.catalog
	lease, released := 0, 0
	h := service.NewHandler(service.Config{Identity: service.IdentityOptions{Local: true}, Acquire: func(ctx context.Context, caller identity.HTTPIdentity, _ string) (service.Resources, func(), error) {
		lease++
		f.catalog.SetStamp(caller.Principal, service.ContextMetadata(ctx).RequestID, "allowed")
		return f.resources, func() { released++ }, nil
	}})
	server := httptest.NewServer(h)
	defer server.Close()
	sdk, err := snapshotclient.New(snapshotclient.Config{BaseURL: server.URL, Transport: managementTransport{client: server.Client()}})
	if err != nil {
		t.Fatal(err)
	}
	cli := snapshotcli.New(sdk, snapshotcli.Context{Principal: "alice"})
	for _, command := range []struct {
		path  string
		flags map[string]any
	}{
		{"show", nil},
		{"attach", map[string]any{"repo": string(other.id)}},
		{"registry audit", map[string]any{"limit": "2"}},
		{"detach", map[string]any{"repo": string(other.id)}},
	} {
		output, err := cli.Execute(context.Background(), command.path, command.flags, snapshotclient.RequestOptions{RequestID: "standalone-management"})
		if err != nil {
			t.Fatalf("real standalone %s failed: %v", command.path, err)
		}
		if command.path == "registry audit" {
			raw, err := json.Marshal(output)
			var audit struct {
				Source, AuthorityID string
				Entries             []map[string]any
			}
			if err != nil || json.Unmarshal(raw, &audit) != nil || audit.Source != "registry" || audit.AuthorityID != string(authority.ID()) || len(audit.Entries) != 2 || audit.Entries[0]["message"] != "register "+string(other.id) || audit.Entries[0]["author"] != "alice" || audit.Entries[0]["requestId"] != "standalone-management" || audit.Entries[0]["ruleId"] != "allowed" {
				t.Fatalf("real audit did not read accepted authority history: %#v", output)
			}
		}
	}
	if _, err := cli.Execute(context.Background(), "registry audit", map[string]any{"continuation": "unsupported"}, snapshotclient.RequestOptions{}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("standalone CLI advertised a false continuation page: %v", err)
	}
	if lease != 4 || released != lease || f.catalog.HasRepository(other.id) {
		t.Fatalf("real management lost request leases or final membership: %d acquired/%d released", lease, released)
	}
}
