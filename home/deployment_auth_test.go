package home

import (
	"strings"
	"testing"
)

// Deployment auth names come from the identity registry: every registered
// authenticator is accepted, and the retired --auth lakefs (lakeFS is storage
// only; KC reaches it with a service credential) is rejected by name.
func TestDeploymentAuthIsAnIdentityRegistryName(t *testing.T) {
	for _, mode := range []string{"local", "taihu"} {
		config := deploymentFixture(t)
		config.Auth = mode
		if err := config.Validate(); err != nil {
			t.Fatalf("registered authenticator %q rejected: %v", mode, err)
		}
	}
	for _, mode := range []string{"lakefs", "gitea", ""} {
		config := deploymentFixture(t)
		config.Auth = mode
		err := config.Validate()
		if err == nil || !strings.Contains(err.Error(), "deployment auth must be one of") {
			t.Fatalf("auth %q accepted or misreported: %v", mode, err)
		}
	}
}
