package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"kc/kernel"
	"kc/liveaccess"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	objectservice "kc/object/service"
)

// A shape plans its Aspects before any runtime contact: Bindings outside the
// selection are never hydrated, so trimming cannot happen after the fact.
func TestShapeHydratesOnlySelectedBindingAspects(t *testing.T) {
	var mu sync.Mutex
	called := []string{}
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req referenceRuntimeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		mu.Lock()
		called = append(called, req.Binding.Address.AspectName)
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"status": "running"}, "basis": objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "rev-1", ObservedAt: "2026-10-16T00:00:00Z"}})
	}))
	defer runtime.Close()
	base, ids, _ := newLogicalReferenceFixture(t, runtime.URL)
	svc := liveservice.Open(base, liveaccess.NewHTTPStateLookup(runtime.Client()), kernel.IdentityContext{Principal: "agent"})
	executor := objectservice.ShapeReadExecutor{
		Read: func(ctx context.Context, ref objectmodel.Reference) (objectmodel.ObservedValue, error) {
			values, err := svc.ReadReference(ctx, ref)
			if err != nil {
				return objectmodel.ObservedValue{}, err
			}
			if len(values) != 1 {
				t.Fatalf("one repository reference returned %d values", len(values))
			}
			return values[0], nil
		},
		Schema: func(context.Context, kernel.RepositoryID, kernel.CommitID, string) (*objectmodel.SchemaDefinition, error) {
			return nil, nil
		},
	}
	ref := objectmodel.Reference{Repository: ids[0], Object: "job/shared"}
	for _, tc := range []struct {
		shape string
		calls []string
	}{
		{`{ definition { owner } }`, nil},
		{`{ definition runtime { status } }`, []string{"runtime"}},
	} {
		mu.Lock()
		called = nil
		mu.Unlock()
		got, err := executor.Execute(context.Background(), objectservice.ShapeReadRequest{Reference: ref, Shape: tc.shape})
		if err != nil || len(got.Errors) != 0 {
			t.Fatalf("%s: %#v %v", tc.shape, got, err)
		}
		if _, ok := got.Data["audit"]; ok {
			t.Fatalf("%s: unselected aspect leaked into data: %#v", tc.shape, got.Data)
		}
		mu.Lock()
		if len(called) != len(tc.calls) || (len(called) == 1 && called[0] != tc.calls[0]) {
			t.Fatalf("%s: runtime calls %v, want %v", tc.shape, called, tc.calls)
		}
		mu.Unlock()
	}
}
