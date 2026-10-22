package invocation

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kc/internal/telemetry"
	"kc/internal/testkit"
	objectmodel "kc/object"
	"kc/snapshot"
)

func TestObservedSnapshotContextCancelsHungLakeFSRead(t *testing.T) {
	runtime := snapshotContextTelemetry(t)
	entered := make(chan struct{}, 1)
	canceled := make(chan struct{}, 1)
	cleanup := make(chan struct{})
	fake := testkit.NewLakeFSFakeWithInterceptor(t, func(_ http.ResponseWriter, request *http.Request) bool {
		if request.Method != http.MethodGet || !strings.HasSuffix(request.URL.Path, "/objects") || request.URL.Query().Get("path") != "blocked.txt" {
			return false
		}
		entered <- struct{}{}
		select {
		case <-request.Context().Done():
			canceled <- struct{}{}
		case <-cleanup:
		}
		return true
	})
	// Only failed-test cleanup releases an uncanceled handler. The successful
	// path must finish both the read and server request through cancellation.
	t.Cleanup(func() { close(cleanup) })

	name := fake.NewRepo()
	raw, err := snapshot.OpenLakeFS("kr://telemetry/cancel", fake.DSN(name), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	head, err := raw.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	commit, err := fake.CommitFiles(name, head, []snapshot.TreeChange{
		{Path: "safe.txt", Content: []byte("unbound read")},
	})
	if err != nil {
		t.Fatal(err)
	}
	observed := ObserveSnapshotStore(raw, runtime, "lakefs")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bound := snapshot.BindStoreContext(observed, ctx)
	requireSnapshotContextCapabilities(t, observed, bound)
	tree, ok := snapshot.TreeStoreOf(bound)
	if !ok {
		t.Fatal("bound lakeFS view lost TreeStore")
	}
	done := make(chan error, 1)
	go func() {
		_, err := tree.ReadFile("blocked.txt", commit)
		done <- err
	}()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("read finished before reaching the blocked server: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("read never reached the blocked server")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("bound read returned %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("observed lakeFS read stayed blocked after cancellation")
	}
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not reach the lakeFS HTTP request")
	}
	requireSnapshotContextReadMetric(t, runtime, "error")

	unboundTree, ok := snapshot.TreeStoreOf(observed)
	if !ok {
		t.Fatal("original observed view lost TreeStore")
	}
	body, err := unboundTree.ReadFile("safe.txt", commit)
	if err != nil || string(body) != "unbound read" {
		t.Fatalf("canceling the bound view changed the original: body=%q error=%v", body, err)
	}
	requireSnapshotContextReadMetric(t, runtime, "ok")
}

func TestObservedSnapshotContextPreservesOptionalCapabilities(t *testing.T) {
	fake := testkit.NewLakeFSFake(t)
	lakefs, err := snapshot.OpenLakeFS("kr://telemetry/verifying", fake.DSN(fake.NewRepo()), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	for name, inner := range map[string]snapshot.Store{
		"store only":            &registryLikeStore{id: "kr://telemetry/plain"},
		"object tree":           testkit.MakeTreeStore(t, "kr://telemetry/tree"),
		"verifying tree":        lakefs,
		"verifying object tree": &verifyingLocatorFixture{ObjectRepository: testkit.MakeRepository(t, "kr://telemetry/verifying-object")},
	} {
		t.Run(name, func(t *testing.T) {
			runtime := snapshotContextTelemetry(t)
			observed := ObserveSnapshotStore(inner, runtime, "lakefs")
			_, hadVerifier := inner.(snapshot.TreeWriteVerifier)
			_, hasVerifier := observed.(snapshot.TreeWriteVerifier)
			if hadVerifier != hasVerifier {
				t.Fatal("observation changed write verification capability")
			}
			if got := snapshot.BindStoreContext(observed, nil); got != observed {
				t.Fatal("nil context changed the observed view")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			bound := snapshot.BindStoreContext(observed, ctx)
			requireSnapshotContextCapabilities(t, observed, bound)
			if _, hasTree := snapshot.TreeStoreOf(inner); !hasTree {
				capabilities := snapshotContextCapabilities(bound)
				if capabilities.tree || capabilities.directory || capabilities.history || capabilities.changes || capabilities.locator {
					t.Fatalf("store-only member gained capabilities: %+v", capabilities)
				}
			}
			head, err := bound.Head(snapshot.DefaultRef)
			if err != nil {
				t.Fatal(err)
			}
			if fixture, ok := inner.(*verifyingLocatorFixture); ok {
				if err := bound.(snapshot.TreeWriteVerifier).VerifyTreeWrite(snapshot.TreeChangeSet{}, false); err != nil || fixture.calls.Load() != 1 {
					t.Fatalf("object verifier did not reach original authority: calls=%d err=%v", fixture.calls.Load(), err)
				}
			}
			if locator, ok := bound.(objectmodel.UnitLocator); ok {
				before, err := inner.(objectmodel.UnitLocator).ObjectUnitPaths("item/a", head)
				if err != nil {
					t.Fatal(err)
				}
				after, err := locator.ObjectUnitPaths("item/a", head)
				if err != nil || !slices.Equal(before, after) {
					t.Fatalf("bound locator changed: before=%v after=%v error=%v", before, after, err)
				}
			}
		})
	}
}

func snapshotContextTelemetry(t *testing.T) *telemetry.Runtime {
	t.Helper()
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "kc-test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })
	return runtime
}

type observedSnapshotCapabilities struct {
	tree, directory, history, changes, origin, locator, verifier bool
}

func snapshotContextCapabilities(store snapshot.Store) observedSnapshotCapabilities {
	var capabilities observedSnapshotCapabilities
	_, capabilities.tree = store.(snapshot.TreeStore)
	_, capabilities.directory = store.(snapshot.DirectoryReader)
	_, capabilities.history = store.(snapshot.HistoryStore)
	_, capabilities.changes = store.(snapshot.ChangeStore)
	_, capabilities.origin = store.(snapshot.StoreOrigin)
	_, capabilities.locator = store.(objectmodel.UnitLocator)
	_, capabilities.verifier = store.(snapshot.TreeWriteVerifier)
	return capabilities
}

func requireSnapshotContextCapabilities(t *testing.T, before, after snapshot.Store) {
	t.Helper()
	want, got := snapshotContextCapabilities(before), snapshotContextCapabilities(after)
	if want != got {
		t.Fatalf("context binding changed capabilities: before=%+v after=%+v", want, got)
	}
	if before.ID() != after.ID() {
		t.Fatalf("context binding changed repository: %s -> %s", before.ID(), after.ID())
	}
	if want.origin {
		oldOrigin := before.(snapshot.StoreOrigin).Origin()
		newOrigin := after.(snapshot.StoreOrigin).Origin()
		if oldOrigin != newOrigin {
			t.Fatalf("context binding changed origin: %s -> %s", oldOrigin, newOrigin)
		}
	}
}

func requireSnapshotContextReadMetric(t *testing.T, runtime *telemetry.Runtime, outcome string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	runtime.MetricsHandler().ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if strings.HasPrefix(line, "kc_snapshot_operations_total{") &&
			strings.Contains(line, `kc_operation="read"`) &&
			strings.Contains(line, `kc_snapshot_store="lakefs"`) &&
			strings.Contains(line, `kc_outcome="`+outcome+`"`) &&
			strings.HasSuffix(line, " 1") {
			return
		}
	}
	t.Fatalf("missing observed read counter for outcome %s:\n%s", outcome, recorder.Body.String())
}

type verifyingLocatorFixture struct {
	*testkit.ObjectRepository
	calls atomic.Int32
}

func (s *verifyingLocatorFixture) VerifyTreeWrite(snapshot.TreeChangeSet, bool) error {
	s.calls.Add(1)
	return nil
}

func TestObservedVerifierPreservesCancellationAndDistinctOperationMetric(t *testing.T) {
	runtime := snapshotContextTelemetry(t)
	fake := testkit.NewLakeFSFake(t)
	repo, err := snapshot.OpenLakeFS("kr://telemetry/verify-cancel", fake.DSN(fake.NewRepo()), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	observed := ObserveSnapshotStore(repo, runtime, "lakefs")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	bound := snapshot.BindStoreContext(observed, ctx)
	err = bound.(snapshot.TreeWriteVerifier).VerifyTreeWrite(snapshot.TreeChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head, Changes: []snapshot.TreeChange{{Path: "file.txt", Content: []byte("value")}}}, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("verification lost cancellation: %v", err)
	}
	recorder := httptest.NewRecorder()
	runtime.MetricsHandler().ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(recorder.Body.String(), `kc_operation="verify_write"`) {
		t.Fatalf("verification counted as commit or other: %s", recorder.Body.String())
	}
}
