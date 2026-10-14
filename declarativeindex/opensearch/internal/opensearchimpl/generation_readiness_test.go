package opensearchimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	index "kc/discovery/projection"
	"kc/kernel"
)

func TestOpenSearchMetadataChecksActualGenerationAndPreservesReadErrors(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		transport bool
		absent    bool
		failed    bool
	}{
		{name: "exists", status: http.StatusOK},
		{name: "missing", status: http.StatusNotFound, absent: true},
		{name: "denied", status: http.StatusForbidden, failed: true},
		{name: "service failure", status: http.StatusServiceUnavailable, failed: true},
		{name: "transport failure", transport: true, failed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control := controlDoc{ActiveIndex: "owned-generation", Generation: "g1", State: index.ProjectionStateReady, Basis: "c1", AccessDigest: "a1", PhysicalDigest: "p1", ProjectionRevision: "r1", ObservationDigest: "o1"}
			var gets, heads atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/"+controlIndexName+"/_doc/control":
					gets.Add(1)
					_ = json.NewEncoder(w).Encode(map[string]any{"_source": control, "_seq_no": 1, "_primary_term": 1})
				case r.Method == http.MethodHead && r.URL.Path == "/owned-generation":
					heads.Add(1)
					if tc.transport {
						conn, _, err := w.(http.Hijacker).Hijack()
						if err != nil {
							t.Error(err)
							return
						}
						_ = conn.Close()
						return
					}
					w.WriteHeader(tc.status)
				default:
					t.Errorf("readiness sent unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()
			engine := &openSearchEngine{base: server.URL, http: server.Client(), controlID: "control"}
			meta, err := engine.LoadMetaContext(context.Background())
			if gets.Load() != 1 || heads.Load() == 0 {
				t.Fatalf("did not inspect the exact actual generation: GET=%d HEAD=%d", gets.Load(), heads.Load())
			}
			switch {
			case tc.failed:
				if err == nil || !reflect.DeepEqual(meta, index.Meta{}) {
					t.Fatalf("provider read failure looked ready: meta=%#v error=%v", meta, err)
				}
				if tc.transport || tc.status == http.StatusServiceUnavailable {
					if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
						t.Fatalf("transient failure lost its typed error: %v", err)
					}
				}
			case tc.absent:
				if err != nil || !reflect.DeepEqual(meta, index.Meta{}) {
					t.Fatalf("lost generation retained READY control metadata: meta=%#v error=%v", meta, err)
				}
			default:
				if err != nil || !reflect.DeepEqual(meta, metaFromControl(control)) || heads.Load() != 1 {
					t.Fatalf("existing generation changed metadata: meta=%#v error=%v HEAD=%d", meta, err, heads.Load())
				}
			}
		})
	}
}

func TestOpenSearchMetadataCancellationReachesGenerationRead(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/"+controlIndexName+"/_doc/control" {
			_ = json.NewEncoder(w).Encode(map[string]any{"_source": controlDoc{ActiveIndex: "owned-generation", State: index.ProjectionStateReady}})
			return
		}
		if r.Method != http.MethodHead || r.URL.Path != "/owned-generation" {
			t.Errorf("unexpected metadata read: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	engine := &openSearchEngine{base: server.URL, http: server.Client(), controlID: "control"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := engine.LoadMetaContext(ctx); result <- err }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("metadata did not reach generation existence read")
	}
	cancel()
	select {
	case err := <-result:
		if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
			t.Fatalf("generation read did not preserve cancellation failure: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("generation read ignored caller cancellation")
	}
}
