package opensearchimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"kc/kernel"
)

func scopeDemandEngine(t *testing.T) *openSearchEngine {
	t.Helper()
	var records resourceControlFixture
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !records.serve(w, r) {
			t.Errorf("unexpected physical IO: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected physical IO", 500)
		}
	}))
	t.Cleanup(server.Close)
	owner := &openSearchEngine{base: server.URL, http: server.Client(), namespace: "owned", controlIndex: "scope-control"}
	return owner.resourceScope("repo")
}

func TestScopeDemandPersistsEpochAndRejectsObsoleteAuthority(t *testing.T) {
	e := scopeDemandEngine(t)
	ctx := context.Background()
	if err := e.checkScopeDemand(ctx, strings.Repeat("a", 32)); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("missing scope granted admission: %v", err)
	}
	first, err := e.activateScopeDemand(ctx)
	if err != nil || !validGeneration(first.Epoch) {
		t.Fatalf("activate: %#v %v", first, err)
	}
	peer := e.resourceScope(e.repository)
	if repeated, err := peer.activateScopeDemand(ctx); err != nil || repeated != first {
		t.Fatalf("reopen changed active epoch: %#v %v", repeated, err)
	}
	if err := peer.checkScopeDemand(ctx, first.Epoch); err != nil {
		t.Fatal(err)
	}
	if err := e.retireScopeDemand(ctx, first.Epoch); err != nil {
		t.Fatal(err)
	}
	if err := peer.checkScopeDemand(ctx, first.Epoch); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("retired scope admitted old writer: %v", err)
	}
	second, err := peer.activateScopeDemand(ctx)
	if err != nil || second.Epoch == first.Epoch {
		t.Fatalf("reactivation reused epoch: %#v %v", second, err)
	}
	if err := e.retireScopeDemand(ctx, first.Epoch); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("obsolete stop closed new demand: %v", err)
	}
	if err := e.checkScopeDemand(ctx, second.Epoch); err != nil {
		t.Fatal(err)
	}
}

func TestScopeDemandConcurrentActivationUsesOneDurableEpoch(t *testing.T) {
	e := scopeDemandEngine(t)
	var wg sync.WaitGroup
	results := make(chan scopeDemand, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			demand, err := e.resourceScope(e.repository).activateScopeDemand(context.Background())
			if err != nil {
				t.Error(err)
				return
			}
			results <- demand
		}()
	}
	wg.Wait()
	close(results)
	var epoch string
	for demand := range results {
		if epoch != "" && epoch != demand.Epoch {
			t.Fatalf("simultaneous activators diverged: %s / %s", epoch, demand.Epoch)
		}
		epoch = demand.Epoch
	}
	if !validGeneration(epoch) {
		t.Fatal("no successful activation")
	}
}

func TestScopeDemandRejectsUnknownDataAndOwnership(t *testing.T) {
	epoch := strings.Repeat("a", 32)
	for _, raw := range []string{
		`null`, `{}`, `{"version":2,"epoch":"` + epoch + `","phase":"ACTIVE"}`,
		`{"version":1,"epoch":"` + epoch + `","phase":"UNKNOWN"}`,
		`{"version":1,"epoch":"` + epoch + `","phase":"ACTIVE","future":true}`,
	} {
		var demand scopeDemand
		if err := json.Unmarshal([]byte(raw), &demand); err == nil {
			t.Fatalf("accepted unknown demand: %s", raw)
		}
	}
	e := scopeDemandEngine(t)
	ctx := context.Background()
	if _, err := e.putControlRecord(e.controlID+"-scope", controlDoc{Repository: "foreign", State: "SCOPE", ScopeDemand: &scopeDemand{Version: 1, Epoch: epoch, Phase: scopeActive}}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.activateScopeDemand(ctx); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("foreign scope was adopted: %v", err)
	}
	if err := e.retireScopeDemand(ctx, epoch); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("foreign scope was retired: %v", err)
	}
	e.namespace = ""
	if _, err := e.activateScopeDemand(ctx); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("legacy namespace granted owned scope: %v", err)
	}
}

func TestScopeDemandReadOnlyAndUnavailableAuthorityCannotMutate(t *testing.T) {
	e := scopeDemandEngine(t)
	ctx := context.Background()
	active, err := e.activateScopeDemand(ctx)
	if err != nil {
		t.Fatal(err)
	}
	view := e.resourceScope(e.repository)
	view.fixedControl = []byte(`{}`)
	if _, err := view.activateScopeDemand(ctx); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("fixed query view activated demand: %v", err)
	}
	if err := view.retireScopeDemand(ctx, active.Epoch); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("fixed query view retired demand: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := e.retireScopeDemand(cancelled, active.Epoch); err == nil {
		t.Fatal("cancelled authority read retired demand")
	}
	if err := e.checkScopeDemand(ctx, active.Epoch); err != nil {
		t.Fatalf("rejected mutation changed scope: %v", err)
	}
	// A failed authority response is not a missing document or an empty demand.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unavailable authority caused write: %s", r.Method)
		}
		http.Error(w, "authority unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	e.base, e.http = server.URL, server.Client()
	if _, err := e.activateScopeDemand(ctx); err == nil {
		t.Fatal("unavailable authority granted activation")
	}
	if err := e.retireScopeDemand(ctx, active.Epoch); err == nil {
		t.Fatal("unavailable authority granted retirement")
	}
}
