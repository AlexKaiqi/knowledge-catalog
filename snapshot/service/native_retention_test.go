package service

import (
	"testing"

	"kc/snapshot/access"
)

func TestNativeRetentionCannotInvalidateCommittedHistory(t *testing.T) {
	rules := &nativeTestRules{rules: []access.Rule{{Principal: "alice", Repo: "kr://test/a", Actions: []string{access.RepositorySettingsManage}}}}
	handler, upstream := nativeTestHandler(t, rules)
	for _, method := range []string{"PUT", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			callNativeTest(t, handler, "alice", method, "/api/v1/repositories/alpha/settings/gc_rules", `{"default_retention_days":1}`, 409)
			upstream.Lock()
			defer upstream.Unlock()
			if len(upstream.calls) != 0 {
				t.Fatalf("retention mutation reached storage: %v", upstream.calls)
			}
		})
	}
}
