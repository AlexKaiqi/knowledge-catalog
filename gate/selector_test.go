package gate

import (
	"encoding/json"
	"testing"

	"kc/kernel"
)

func TestRetiredCatalogScopeCannotBecomeServiceGate(t *testing.T) {
	for _, selector := range []string{"catalog", "Catalog", "CATALOG"} {
		var rule Rule
		err := json.Unmarshal([]byte(`{"on":"merge","require":["suite:review"],"`+selector+`":"old-space"}`), &rule)
		if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("retired scope was widened or misdiagnosed: %v", err)
		}
	}
}
