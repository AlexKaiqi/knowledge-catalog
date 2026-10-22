package home_test

import (
	"fmt"
	"os"
	"testing"

	"kc/internal/testkit"
)

// TestMain reclaims the projection indices this run created. The package's
// fixtures build homes from DefaultStores, which adopts the explicit endpoint's
// shared resources; without this, every run leaves its indices behind and the
// instance eventually refuses new ones on the shard limit rather than failing an
// assertion. Ownership is the difference from the inventory taken here, taken by
// the same rule the test-group entries apply at their boundary.
func TestMain(m *testing.M) {
	reclaimSearch := testkit.OwnSharedSearchIndices()
	code := m.Run()
	if reclaimSearch != nil {
		if err := reclaimSearch(); err != nil {
			fmt.Fprintf(os.Stderr, "reclaim owned test projection indices: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}
	os.Exit(code)
}
