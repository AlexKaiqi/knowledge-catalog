package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kc/kernel"
)

func connectedHandlerEngineEndpoint(t *testing.T, namespace string) (string, *atomic.Int64) {
	t.Helper()
	endpoint := strings.TrimRight(os.Getenv("KC_TEST_OPENSEARCH_URL"), "/")
	if endpoint == "" {
		t.Fatal("handler lifecycle regression requires the existing real OpenSearch endpoint")
	}
	target, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	writes := &atomic.Int64{}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ModifyResponse = func(response *http.Response) error {
		request := response.Request
		if response.StatusCode < 300 && (request.Method == http.MethodPut ||
			(request.Method == http.MethodPost && (strings.HasSuffix(request.URL.Path, "/_bulk") || strings.HasSuffix(request.URL.Path, "/_aliases") || strings.HasSuffix(request.URL.Path, "/_refresh")))) {
			writes.Add(1)
		}
		return nil
	}
	forwarder := httptest.NewServer(proxy)
	t.Cleanup(forwarder.Close)
	t.Cleanup(func() { cleanupConnectedHandlerNamespace(t, endpoint, namespace) })
	return forwarder.URL, writes
}

// Enumerate only this test's unique physical namespace, then delete the exact
// returned names. The shared control index and other tests' resources are never
// candidates for cleanup.
func cleanupConnectedHandlerNamespace(t *testing.T, endpoint, namespace string) {
	t.Helper()
	scope := string(kernel.CanonicalDigest(namespace)[:24])
	prefix := "kc-proj-" + scope + "-"
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Get(endpoint + "/_cat/indices/" + prefix + "*?format=json&h=index")
	if err != nil {
		t.Error(err)
		return
	}
	defer response.Body.Close()
	var owned []struct {
		Index string `json:"index"`
	}
	if response.StatusCode == http.StatusOK {
		if err := json.NewDecoder(response.Body).Decode(&owned); err != nil {
			t.Error(err)
			return
		}
	} else if response.StatusCode != http.StatusNotFound {
		t.Errorf("list this handler's own physical namespace: HTTP %d", response.StatusCode)
		return
	}
	for _, resource := range owned {
		if !strings.HasPrefix(resource.Index, prefix) {
			t.Errorf("namespace listing returned an unowned resource: %s", resource.Index)
			continue
		}
		deleteConnectedHandlerResource(t, client, endpoint, resource.Index)
	}
	deleteConnectedHandlerResource(t, client, endpoint, "kc-projection-control-v1-"+scope)
}

func deleteConnectedHandlerResource(t *testing.T, client *http.Client, endpoint, index string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodDelete, endpoint+"/"+index, nil)
	if err != nil {
		t.Error(err)
		return
	}
	response, err := client.Do(request)
	if err != nil {
		t.Error(err)
		return
	}
	_ = response.Body.Close()
	if response.StatusCode >= 300 && response.StatusCode != http.StatusNotFound {
		t.Errorf("delete this handler's resource %s: HTTP %d", index, response.StatusCode)
	}
}
