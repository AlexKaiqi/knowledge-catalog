package lakefsimpl

import (
	"strings"
	"testing"
)

func TestParseDSNRejectsCredentialsAndKeepsDeploymentPrefix(t *testing.T) {
	endpoint, err := ParseDSN("https://lake.example.test/platform/object")
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Origin != "https://lake.example.test/platform" ||
		endpoint.API != "https://lake.example.test/platform/api/v1" ||
		endpoint.Repository != "object" {
		t.Fatalf("endpoint=%#v", endpoint)
	}
	for _, raw := range []string{
		"https://access:secret@lake.example.test/object",
		"https://lake.example.test/object?token=secret",
		"s3://bucket/object",
	} {
		if _, err := ParseDSN(raw); err == nil {
			t.Fatalf("accepted unsafe dsn %q", raw)
		}
	}
}

func TestCredentialNeverAppearsInRepositoryString(t *testing.T) {
	endpoint, err := ParseDSN("https://lake.example.test/object")
	if err != nil {
		t.Fatal(err)
	}
	client, err := newAPIClient(endpoint, "access:very-secret")
	if err != nil {
		t.Fatal(err)
	}
	repo := &Repository{id: "kr://acme/object", ep: endpoint, client: client}
	if strings.Contains(repo.String(), "very-secret") || strings.Contains(repo.String(), "access") {
		t.Fatalf("repository string leaked credentials: %s", repo)
	}
}

func TestEncodeLakeFSBranchKeepsMainAndFlattensCandidates(t *testing.T) {
	if got := encodeLakeFSBranch("main"); got != "main" {
		t.Fatalf("main=%q", got)
	}
	if got := encodeLakeFSBranch("candidates/PR-scene"); got != "candidates--PR-scene" {
		t.Fatalf("candidate=%q", got)
	}
}
