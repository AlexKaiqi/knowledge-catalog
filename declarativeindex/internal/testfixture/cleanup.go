package testfixture

import (
	"encoding/json"
	"kc/kernel"
	"net/http"
	"strings"
	"testing"
)

// CleanupNamespace removes exact indices in a unique namespace allocated by
// this test. It never deletes a shared control index or uses wildcard DELETE.
func CleanupNamespace(t *testing.T, endpoint, namespace string) {
	t.Helper()
	if namespace == "" {
		t.Fatal("test cleanup requires an invocation-owned namespace")
	}
	scope := string(kernel.CanonicalDigest(namespace))[:24]
	prefix := "kc-proj-" + scope + "-"
	response, err := http.Get(endpoint + "/_cat/indices/" + prefix + "*?format=json&h=index&expand_wildcards=all")
	if err != nil {
		t.Errorf("list owned projections: %v", err)
		return
	}
	var rows []struct {
		Index string `json:"index"`
	}
	err = json.NewDecoder(response.Body).Decode(&rows)
	_ = response.Body.Close()
	if err != nil {
		t.Errorf("owned projection inventory: %v", err)
		return
	}
	for _, row := range rows {
		if !strings.HasPrefix(row.Index, prefix) {
			t.Errorf("unowned projection in cleanup inventory: %s", row.Index)
			continue
		}
		deleteNamespaceIndex(t, endpoint, row.Index)
	}
	deleteNamespaceIndex(t, endpoint, "kc-projection-control-v1-"+scope)
}

func deleteNamespaceIndex(t *testing.T, endpoint, name string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodDelete, endpoint+"/"+name, nil)
	if err != nil {
		t.Errorf("owned cleanup request: %v", err)
		return
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Errorf("owned cleanup: %v", err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 && response.StatusCode != 404 {
		t.Errorf("owned cleanup %s HTTP %d", name, response.StatusCode)
	}
}
