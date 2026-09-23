package cli

import (
	"testing"

	"kc/internal/testkit"
)

// internalLakeFSRepoDSN provisions one protocol-faithful lakeFS fake
// repository for an internal-scoped test and returns its DSN; the credential
// stays in the process environment. The external cli_test twin is
// lakeFSRepoDSN in kc_test.go.
func internalLakeFSRepoDSN(t *testing.T) string {
	t.Helper()
	fake := testkit.NewLakeFSFake(t)
	t.Setenv("KC_LAKEFS_CREDENTIAL", testkit.LakeFSFakeCredential)
	return fake.DSN(fake.NewRepo())
}
