package home

import "testing"

func TestDeploymentAcceptsLakeFSAuthentication(t *testing.T) {
	config := deploymentFixture(t)
	config.Auth = "lakefs"
	config.AuthURL = "http://lakefs.example"
	if err := config.Validate(); err != nil {
		t.Fatalf("fixed lakeFS caller verifier rejected: %v", err)
	}
}
