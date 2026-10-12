package hook

import (
	"encoding/json"
	"testing"

	"kc/kernel"
)

func TestRetiredCatalogScopeCannotBecomeServiceHook(t *testing.T) {
	for _, selector := range []string{"catalog", "Catalog", "CATALOG"} {
		var binding Binding
		err := json.Unmarshal([]byte(`{"on":"dataset.manage","phase":"post","run":"notify","`+selector+`":"old-space"}`), &binding)
		if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("retired scope was widened or misdiagnosed: %v", err)
		}
	}
}
