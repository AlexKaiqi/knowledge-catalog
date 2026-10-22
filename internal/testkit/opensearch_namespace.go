package testkit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"kc/kernel"
)

// CleanupProjectionNamespace removes the projection indices owned by one test
// namespace. It is the single implementation of owned-projection cleanup: live
// fixtures inside declarativeindex keep their existing name and delegate here,
// and fixtures outside it import this package because the declarativeindex
// internal path is not importable from them.
//
// Ownership is proven by name: only indices carrying this namespace's scope
// prefix are deleted, and a wildcard DELETE is never issued. The empty
// namespace is rejected because adopting the legacy resources would drop
// indices this test does not own.
func CleanupProjectionNamespace(t *testing.T, endpoint, namespace string) {
	t.Helper()
	if strings.TrimSpace(namespace) == "" {
		t.Fatal("projection cleanup requires an invocation-owned namespace")
	}
	for _, problem := range CleanupProjectionNamespaceNow(endpoint, namespace) {
		t.Error(problem)
	}
}

// CleanupProjectionNamespaceNow removes one namespace's projection indices
// without a *testing.T so a package can run it after its tests have finished.
// It returns every problem instead of stopping at the first, because an
// ownership check that silently gives up leaves shards behind that later runs
// pay for.
func CleanupProjectionNamespaceNow(endpoint, namespace string) []error {
	if strings.TrimSpace(namespace) == "" {
		return []error{fmt.Errorf("projection cleanup requires an invocation-owned namespace")}
	}
	scope := string(kernel.CanonicalDigest(namespace))[:24]
	prefix := "kc-proj-" + scope + "-"
	response, err := http.Get(endpoint + "/_cat/indices/" + prefix + "*?format=json&h=index&expand_wildcards=all")
	if err != nil {
		return []error{fmt.Errorf("list owned projections: %w", err)}
	}
	var rows []struct {
		Index string `json:"index"`
	}
	err = json.NewDecoder(response.Body).Decode(&rows)
	_ = response.Body.Close()
	if err != nil {
		return []error{fmt.Errorf("owned projection inventory: %w", err)}
	}
	problems := make([]error, 0)
	for _, row := range rows {
		if !strings.HasPrefix(row.Index, prefix) {
			problems = append(problems, fmt.Errorf("unowned projection in cleanup inventory: %s", row.Index))
			continue
		}
		if err := deleteProjectionIndexNow(endpoint, row.Index); err != nil {
			problems = append(problems, err)
		}
	}
	if err := deleteProjectionIndexNow(endpoint, "kc-projection-control-v1-"+scope); err != nil {
		problems = append(problems, err)
	}
	return problems
}

func deleteProjectionIndexNow(endpoint, name string) error {
	request, err := http.NewRequest(http.MethodDelete, endpoint+"/"+name, nil)
	if err != nil {
		return fmt.Errorf("owned cleanup request: %w", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("owned cleanup: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 && response.StatusCode != http.StatusNotFound {
		return fmt.Errorf("owned cleanup %s HTTP %d", name, response.StatusCode)
	}
	return nil
}
