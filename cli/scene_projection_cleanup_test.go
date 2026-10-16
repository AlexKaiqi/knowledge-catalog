package cli_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestSceneProjectionNamespaceUsesInvocationOwnership(t *testing.T) {
	const prefix = "kc-test-0123456789abcdef0123456789abcdef-"
	t.Setenv("KC_TEST_OPENSEARCH_RUN_PREFIX", prefix)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(upstream.Close)
	first := newSceneProjectionNamespace(t, upstream.URL)
	second := newSceneProjectionNamespace(t, upstream.URL)
	for _, ns := range []*sceneProjectionNamespace{first, second} {
		physical, err := ns.physicalIndex("kc-projection-control-v1")
		if err != nil || !strings.HasPrefix(physical, prefix) {
			t.Fatalf("physical index is outside invocation ownership: %q %v", physical, err)
		}
	}
	if first.prefix == second.prefix {
		t.Fatal("independent worlds reused one physical namespace")
	}
}

func TestSceneProjectionCloneFailureStillCleansCreatedTarget(t *testing.T) {
	var mu sync.Mutex
	created, deleted := "", ""
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if _, target, ok := strings.Cut(r.URL.Path, "/_clone/"); ok {
			// The upstream accepted creation but the response was lost/failed.
			created = target
			http.Error(w, "upstream response lost after creation", http.StatusBadGateway)
			return
		}
		if r.Method == http.MethodDelete {
			deleted = strings.TrimPrefix(r.URL.Path, "/")
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(upstream.Close)
	source := newSceneProjectionNamespace(t, upstream.URL)
	target := newSceneProjectionNamespace(t, upstream.URL)
	source.rememberIndex(source.prefix + "kc-proj-clone-g-one")
	if err := source.forkInto(target); err == nil || !strings.Contains(err.Error(), "HTTP 502") || !strings.Contains(err.Error(), "upstream response lost after creation") {
		t.Fatalf("clone failure was hidden: %v", err)
	}
	target.close()
	mu.Lock()
	defer mu.Unlock()
	if created == "" || deleted != created || !strings.HasPrefix(deleted, target.prefix) {
		t.Fatalf("accepted clone escaped exact cleanup: created=%q deleted=%q", created, deleted)
	}
}
