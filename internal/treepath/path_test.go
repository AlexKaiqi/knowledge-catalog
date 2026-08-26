package treepath

import (
	"testing"

	"kc/kernel"
)

func TestCleanNormalizesRelativePathsAndRejectsEscape(t *testing.T) {
	got, err := Clean("docs/./runbooks/../policy.md")
	if err != nil || got != "docs/policy.md" {
		t.Fatalf("normalized path = %q, %v", got, err)
	}
	for _, raw := range []string{"", "/absolute", "../outside", "docs/../../outside"} {
		if _, err := Clean(raw); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("%q must fail with USAGE_INVALID: %v", raw, err)
		}
	}
}
