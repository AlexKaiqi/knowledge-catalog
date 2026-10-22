package testfixture

import (
	"kc/internal/testkit"
	"testing"
)

// CleanupNamespace removes exact indices in a unique namespace allocated by
// this test. It never deletes a shared control index or uses wildcard DELETE.
//
// The implementation lives in internal/testkit so that live fixtures outside
// this package delete the same set of indices under the same ownership rules;
// the declarativeindex internal path is not importable from them.
func CleanupNamespace(t *testing.T, endpoint, namespace string) {
	t.Helper()
	testkit.CleanupProjectionNamespace(t, endpoint, namespace)
}
