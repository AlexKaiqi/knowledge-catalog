package cli

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"kc/internal/telemetry"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

func TestObserveSnapshotStoreRecordsLakeFSKindAndPreservesCapabilities(t *testing.T) {
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "kc-test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })

	inner := testkit.MakeTreeStore(t, "kr://acme/core")
	store := observeSnapshotStore(inner, runtime, "lakefs")
	tree, ok := snapshot.TreeStoreOf(store)
	if !ok {
		t.Fatal("tree capability was dropped")
	}
	if _, ok := snapshot.DirectoryReaderOf(store); !ok {
		t.Fatal("directory capability was dropped")
	}
	if _, ok := store.(knowledge.UnitLocator); !ok {
		t.Fatal("unit locator was dropped")
	}
	head, err := store.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tree.ReadFile("missing", head); kernel.CodeOf(err) == "" {
		t.Fatalf("read should fail with a kernel code: %v", err)
	}

	recorder := httptest.NewRecorder()
	runtime.MetricsHandler().ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	body, _ := io.ReadAll(recorder.Result().Body)
	text := string(body)
	if !strings.Contains(text, `kc_snapshot_store="lakefs"`) || !strings.Contains(text, `kc_operation="resolve_ref"`) || !strings.Contains(text, `kc_operation="read"`) {
		t.Fatalf("snapshot instruments missing:\n%s", text)
	}
}

func TestSnapshotKindFromTypeName(t *testing.T) {
	kind := snapshotStoreKind(&lakefsAuthorityStub{}, "other")
	if kind != "lakefs" {
		t.Fatalf("kind=%s", kind)
	}
}

func TestObserveSnapshotStoreDoesNotInventTreeCapability(t *testing.T) {
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "kc-test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })
	store := observeSnapshotStore(&registryLikeStore{id: "kr://plain"}, runtime, "gitea")
	if _, ok := snapshot.TreeStoreOf(store); ok {
		t.Fatal("store-only member gained TreeStore")
	}
	if _, ok := store.(knowledge.UnitLocator); ok {
		t.Fatal("store-only member gained UnitLocator")
	}
	if _, err := store.Head("refs/heads/main"); err != nil {
		t.Fatal(err)
	}
}

func TestTelemetryFaceMapsCreateAndGrants(t *testing.T) {
	cases := map[string]string{
		"named-repository-create": "catalog",
		"create":                  "catalog",
		"attach":                  "catalog",
		"pin":                     "catalog",
		"dataset-define":          "catalog",
		"admin-grant-list":        "control",
		"grant-add":               "control",
		"knowledge-read":          "knowledge",
		"writer-commit":           "writer",
	}
	for command, want := range cases {
		if got := telemetryFace(command); got != want {
			t.Fatalf("%s face=%s want %s", command, got, want)
		}
	}
}

func TestStandingProjectionDoesNotFakeZeroWithoutEngine(t *testing.T) {
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "kc-test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })
	observeStandingProjection(runtime, &Home{}, nil)
	recorder := httptest.NewRecorder()
	runtime.MetricsHandler().ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	body, _ := io.ReadAll(recorder.Result().Body)
	if strings.Contains(string(body), "kc_projection_lagging_count{") {
		t.Fatalf("home without a projection engine exported lagging=0:\n%s", body)
	}
}

type lakefsAuthorityStub struct{}

func (lakefsAuthorityStub) ID() kernel.RepositoryID { return "kr://lakefs" }

type registryLikeStore struct{ id kernel.RepositoryID }

func (s *registryLikeStore) ID() kernel.RepositoryID { return s.id }
func (s *registryLikeStore) Head(string) (kernel.CommitID, error) {
	return "root", nil
}
func (s *registryLikeStore) GetRef(string) (kernel.CommitID, bool) {
	return "root", true
}
func (s *registryLikeStore) HasCommit(kernel.CommitID) bool { return true }
func (s *registryLikeStore) CreateRef(string, kernel.CommitID) error {
	return nil
}
func (s *registryLikeStore) Merge(string, kernel.CommitID, kernel.CommitID) (kernel.CommitID, error) {
	return "merged", nil
}
func (s *registryLikeStore) Archived() bool { return false }
func (s *registryLikeStore) Archive() error { return nil }
