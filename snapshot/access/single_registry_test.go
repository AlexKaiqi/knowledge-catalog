package access_test

import (
	"encoding/json"
	"testing"

	"kc/snapshot/access"
)

func TestRulesRejectRetiredCatalogSelector(t *testing.T) {
	for _, raw := range []string{`{"catalog":"old"}`, `{"catalog":""}`, `{"Catalog":"old"}`, `{"actions":["catalog.read"]}`} {
		var rule access.Rule
		if err := json.Unmarshal([]byte(raw), &rule); err == nil {
			t.Fatalf("retired Catalog selector was silently accepted: %s", raw)
		}
	}
}
