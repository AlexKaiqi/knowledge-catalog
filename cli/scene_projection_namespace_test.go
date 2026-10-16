package cli_test

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// sceneProjectionNamespace isolates real OpenSearch media without changing the
// production provider or emulating retrieval. Only physical index coordinates
// are translated; documents, control payloads, queries and PITs are untouched.
type sceneProjectionNamespace struct {
	t        *testing.T
	server   *httptest.Server
	upstream *url.URL
	prefix   string
	mu       sync.Mutex
	indices  map[string]bool
	auth     string
	once     sync.Once
}

func newSceneProjectionNamespace(t *testing.T, endpoint string) *sceneProjectionNamespace {
	t.Helper()
	target, err := url.Parse(endpoint)
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		t.Fatalf("invalid scene OpenSearch endpoint %q: %v", endpoint, err)
	}
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	prefix := "kc-scene-"
	if runPrefix := os.Getenv("KC_TEST_OPENSEARCH_RUN_PREFIX"); runPrefix != "" {
		if !regexp.MustCompile(`^kc-test-[0-9a-f]{32}-$`).MatchString(runPrefix) {
			t.Fatalf("invalid owned scene OpenSearch prefix %q", runPrefix)
		}
		prefix = runPrefix
	}
	ns := &sceneProjectionNamespace{t: t, upstream: target, prefix: prefix + hex.EncodeToString(random[:]) + "-", indices: map[string]bool{}}
	proxy := httputil.NewSingleHostReverseProxy(target)
	ns.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := ns.rewrite(r); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(ns.close)
	return ns
}

func (ns *sceneProjectionNamespace) rewrite(r *http.Request) error {
	index, rest, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	ns.mu.Lock()
	if auth := r.Header.Get("Authorization"); auth != "" {
		ns.auth = auth
	}
	ns.mu.Unlock()
	if index != "" && !strings.HasPrefix(index, "_") {
		physical, err := ns.physicalIndex(index)
		if err != nil {
			return err
		}
		if r.Method == http.MethodPut || r.Method == http.MethodPost {
			ns.rememberIndex(physical)
		}
		r.URL.Path = "/" + physical
		if rest != "" {
			r.URL.Path += "/" + rest
		}
		r.URL.RawPath = ""
	}
	if r.URL.Path != "/_bulk" {
		return nil
	}
	raw, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if err != nil {
		return err
	}
	lines := bytes.Split(raw, []byte("\n"))
	for i := 0; i < len(lines); i++ {
		if len(bytes.TrimSpace(lines[i])) == 0 {
			continue
		}
		var action map[string]map[string]json.RawMessage
		if err := json.Unmarshal(lines[i], &action); err != nil || len(action) != 1 {
			return fmt.Errorf("invalid scene bulk action metadata")
		}
		for name, metadata := range action {
			if name != "index" && name != "create" && name != "update" && name != "delete" {
				return fmt.Errorf("unsupported scene bulk action %q", name)
			}
			var logical string
			if err := json.Unmarshal(metadata["_index"], &logical); err != nil {
				return fmt.Errorf("scene bulk action lacks an index")
			}
			physical, err := ns.physicalIndex(logical)
			if err != nil {
				return err
			}
			ns.rememberIndex(physical)
			metadata["_index"], _ = json.Marshal(physical)
			lines[i], err = json.Marshal(action)
			if err != nil {
				return err
			}
			if name != "delete" {
				i++ // The next line is source data: preserve its exact bytes.
			}
		}
	}
	raw = bytes.Join(lines, []byte("\n"))
	r.Body = io.NopCloser(bytes.NewReader(raw))
	r.ContentLength = int64(len(raw))
	return nil
}

func (ns *sceneProjectionNamespace) physicalIndex(logical string) (string, error) {
	if !strings.HasPrefix(logical, "kc-") || strings.ContainsAny(logical, ",*?/#") {
		return "", fmt.Errorf("unexpected scene projection index %q", logical)
	}
	return ns.prefix + logical, nil
}

func (ns *sceneProjectionNamespace) rememberIndex(index string) {
	ns.mu.Lock()
	ns.indices[index] = true
	ns.mu.Unlock()
}

// forkInto copies every live index of this namespace into dst with the
// OpenSearch _clone API, so a frozen indexed state can seed independent worlds
// without replaying its construct chain. Sources are write-blocked first: a
// frozen namespace must never change after it seeded a clone.
func (ns *sceneProjectionNamespace) forkInto(dst *sceneProjectionNamespace) error {
	ns.mu.Lock()
	indices := make([]string, 0, len(ns.indices))
	for index := range ns.indices {
		indices = append(indices, index)
	}
	auth := ns.auth
	ns.mu.Unlock()
	sort.Strings(indices)
	if auth != "" {
		dst.mu.Lock()
		if dst.auth == "" {
			dst.auth = auth
		}
		dst.mu.Unlock()
	}
	for _, source := range indices {
		status, body, err := ns.upstreamDo(http.MethodGet, "/"+source+"/_settings", "", auth)
		if err != nil {
			return err
		}
		if status == http.StatusNotFound {
			continue // A replaced generation was already retired.
		}
		if status >= 300 {
			return fmt.Errorf("read scene projection %s settings: HTTP %d: %s", source, status, body)
		}
		target := dst.prefix + strings.TrimPrefix(source, ns.prefix)
		// A clone may be created even if its response is lost: own it before dispatch.
		dst.rememberIndex(target)
		steps := []struct{ method, path, body string }{
			{http.MethodPost, "/" + source + "/_flush", ""},
			{http.MethodPut, "/" + source + "/_settings", `{"index.blocks.write":true}`},
			{http.MethodPost, "/" + source + "/_clone/" + target + "?wait_for_active_shards=1", `{"settings":{"index.blocks.write":false}}`},
		}
		for _, step := range steps {
			status, body, err := ns.upstreamDo(step.method, step.path, step.body, auth)
			if err != nil {
				return err
			}
			if status >= 300 {
				return fmt.Errorf("fork scene projection %s: %s %s HTTP %d: %s", source, step.method, step.path, status, body)
			}
		}
	}
	return nil
}

func (ns *sceneProjectionNamespace) upstreamDo(method, path, body, auth string) (int, string, error) {
	request, err := http.NewRequest(method, strings.TrimRight(ns.upstream.String(), "/")+path, strings.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if auth != "" {
		request.Header.Set("Authorization", auth)
	}
	response, err := (&http.Client{Timeout: 60 * time.Second}).Do(request)
	if err != nil {
		return 0, "", err
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	return response.StatusCode, strings.TrimSpace(string(raw)), err
}

func (ns *sceneProjectionNamespace) close() {
	ns.once.Do(func() {
		ns.server.Close()
		ns.mu.Lock()
		indices := make([]string, 0, len(ns.indices))
		for index := range ns.indices {
			indices = append(indices, index)
		}
		auth := ns.auth
		ns.mu.Unlock()
		sort.Strings(indices)
		client := &http.Client{Timeout: 10 * time.Second}
		for _, index := range indices {
			request, err := http.NewRequest(http.MethodDelete, strings.TrimRight(ns.upstream.String(), "/")+"/"+index, nil)
			if err != nil {
				ns.t.Errorf("clean scene projection: %v", err)
				continue
			}
			request.Header.Set("Authorization", auth)
			response, err := client.Do(request)
			if err != nil {
				ns.t.Errorf("clean scene projection %s: %v", index, err)
				continue
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode >= 300 && response.StatusCode != http.StatusNotFound {
				ns.t.Errorf("clean scene projection %s: HTTP %d", index, response.StatusCode)
			}
		}
	})
}

func TestSceneProjectionNamespaceRewritesOnlyPhysicalCoordinates(t *testing.T) {
	type requestRecord struct{ method, path, query, body, auth string }
	var mu sync.Mutex
	var requests []requestRecord
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		requests = append(requests, requestRecord{r.Method, r.URL.Path, r.URL.RawQuery, string(raw), r.Header.Get("Authorization")})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"pit_id":"opaque-pit","_index":"provider-response"}`)
	}))
	t.Cleanup(upstream.Close)
	first := newSceneProjectionNamespace(t, upstream.URL)
	second := newSceneProjectionNamespace(t, upstream.URL)
	do := func(ns *sceneProjectionNamespace, method, path, body string) {
		t.Helper()
		request, err := http.NewRequest(method, ns.server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Basic ZmFrZQ==")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		if response.StatusCode != http.StatusOK || string(raw) != `{"pit_id":"opaque-pit","_index":"provider-response"}` {
			t.Fatalf("namespace changed the provider response: %d %s", response.StatusCode, raw)
		}
	}
	control := `{"repository":"kr://scene/object","active_index":"kc-proj-note-g-1","_index":"user-value"}`
	document := `{"all_text":"kc-proj-note-g-1","_index":"user-value","index":{"_index":"source-data"}}`
	bulk := "{\"index\":{\"_index\":\"kc-proj-note-g-1\",\"_id\":\"one\"}}\n" + document + "\n{\"delete\":{\"_index\":\"kc-proj-note-g-1\",\"_id\":\"two\"}}\n"
	query := `{"pit":{"id":"opaque-pit"},"query":{"term":{"all_text":"kc-proj-note-g-1"}}}`
	do(first, http.MethodPut, "/kc-projection-control-v1/_doc/repo?refresh=true", control)
	do(first, http.MethodPost, "/_bulk?refresh=wait_for", bulk)
	do(first, http.MethodPost, "/_search?allow_partial_search_results=false", query)
	do(first, http.MethodDelete, "/_search/point_in_time", `{"pit_id":"opaque-pit"}`)
	do(second, http.MethodGet, "/kc-projection-control-v1/_doc/repo", "")
	first.close()
	first.close() // Closing twice must not repeat the destructive cleanup.
	mu.Lock()
	got := append([]requestRecord(nil), requests...)
	mu.Unlock()
	if len(got) != 7 {
		t.Fatalf("requests=%#v", got)
	}
	if got[0].path != "/"+first.prefix+"kc-projection-control-v1/_doc/repo" || got[0].query != "refresh=true" || got[0].body != control {
		t.Fatalf("control translation changed payload or CAS query: %#v", got[0])
	}
	lines := strings.Split(got[1].body, "\n")
	if got[1].path != "/_bulk" || got[1].query != "refresh=wait_for" || len(lines) != 4 || lines[1] != document {
		t.Fatalf("bulk translation changed source data or request: %#v", got[1])
	}
	for _, i := range []int{0, 2} {
		var metadata map[string]map[string]string
		if err := json.Unmarshal([]byte(lines[i]), &metadata); err != nil {
			t.Fatal(err)
		}
		for _, action := range metadata {
			if action["_index"] != first.prefix+"kc-proj-note-g-1" {
				t.Fatalf("bulk action escaped namespace: %s", lines[i])
			}
		}
	}
	if got[2].path != "/_search" || got[2].body != query || got[2].query != "allow_partial_search_results=false" || got[3].path != "/_search/point_in_time" || got[3].body != `{"pit_id":"opaque-pit"}` {
		t.Fatalf("PIT/query changed: %#v", got[2:4])
	}
	if first.prefix == second.prefix || got[4].path != "/"+second.prefix+"kc-projection-control-v1/_doc/repo" {
		t.Fatalf("worlds share a control index: %#v", got[4])
	}
	for _, request := range got[5:] {
		if request.method != http.MethodDelete || !strings.HasPrefix(request.path, "/"+first.prefix) || strings.ContainsAny(request.path, "*,") || request.auth != "Basic ZmFrZQ==" {
			t.Fatalf("cleanup exceeded its own exact index names: %#v", request)
		}
	}
}

func TestSceneProjectionNamespaceForkIsolatesFrozenMedia(t *testing.T) {
	endpoint := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	if endpoint == "" {
		t.Fatal("KC_TEST_OPENSEARCH_URL is required: frozen scene media are forked on real OpenSearch")
	}
	count := func(ns *sceneProjectionNamespace) string {
		t.Helper()
		response, err := http.Get(ns.server.URL + "/kc-proj-fork/_count")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var body struct{ Count int }
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return fmt.Sprint(body.Count)
	}
	put := func(ns *sceneProjectionNamespace, id string) int {
		t.Helper()
		request, _ := http.NewRequest(http.MethodPut, ns.server.URL+"/kc-proj-fork/_doc/"+id+"?refresh=true", strings.NewReader(`{"v":1}`))
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		return response.StatusCode
	}
	built := newSceneProjectionNamespace(t, endpoint)
	if status := put(built, "a"); status >= 300 {
		t.Fatalf("seed HTTP %d", status)
	}
	frozen := newSceneProjectionNamespace(t, endpoint)
	if err := built.forkInto(frozen); err != nil {
		t.Fatal(err)
	}
	first, second := newSceneProjectionNamespace(t, endpoint), newSceneProjectionNamespace(t, endpoint)
	for _, clone := range []*sceneProjectionNamespace{first, second} {
		if err := frozen.forkInto(clone); err != nil {
			t.Fatal(err)
		}
	}
	if status := put(first, "b"); status >= 300 {
		t.Fatalf("clone must stay writable: HTTP %d", status)
	}
	if got := []string{count(frozen), count(first), count(second)}; got[0] != "1" || got[1] != "2" || got[2] != "1" {
		t.Fatalf("fork leaked writes between worlds: frozen/first/second=%v", got)
	}
	if status := put(frozen, "c"); status < 400 {
		t.Fatalf("frozen media accepted a write: HTTP %d", status)
	}
}
