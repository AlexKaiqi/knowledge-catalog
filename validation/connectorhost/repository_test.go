package connectorhost

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverRejectsNestedBusinessLayout(t *testing.T) {
	repo := t.TempDir()
	nested := filepath.Join(repo, "connectors", "billing", "invoice")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "connector.yaml"), []byte("invalid only because it must never be silently ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Discover(repo)
	if err == nil || !strings.Contains(err.Error(), "flat layout") {
		t.Fatalf("expected flat layout error, got %v", err)
	}
}

func TestBrokenConnectorDoesNotHideValidSharedRepositoryPackages(t *testing.T) {
	repo := copyTestRepo(t)
	broken := filepath.Join(repo, "connectors", "billing-broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "connector.yaml"), []byte("apiVersion: wrong\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, _ := NewStore(t.TempDir())
	config := HostConfig{RepoPath: repo, KCURL: "http://kc.invalid"}
	host := NewHost(store, config, KCClient{BaseURL: config.KCURL})
	items, err := host.Connectors(context.Background(), false)
	if err != nil || len(items) != 2 {
		t.Fatalf("shared discovery: %#v %v", items, err)
	}
	byID := map[string]ConnectorInfo{}
	for _, item := range items {
		byID[item.Manifest.Metadata.ID] = item
	}
	if !byID["file-observer"].Valid || byID["billing-broken"].Valid || byID["billing-broken"].Error == "" {
		t.Fatalf("per-directory isolation missing: %#v", byID)
	}
	if _, err := host.Connector("file-observer"); err != nil {
		t.Fatalf("valid connector hidden by broken neighbor: %v", err)
	}
	if _, err := host.Connector("billing-broken"); err == nil {
		t.Fatal("broken connector unexpectedly executable")
	}
}

func TestValidateManifestRequiresSharedRepositoryIdentity(t *testing.T) {
	manifest := Manifest{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata:   Metadata{ID: "Billing_Invoice", Owner: "billing"},
		Spec: Spec{
			Command:     []string{"true"},
			Maintenance: MaintenancePolicy{Representation: "current-state"},
			Target:      Target{Repository: "kr://demo/public/billing", Scope: Scope{Aspects: []string{"observed"}}},
		},
	}
	if err := ValidateManifest(manifest, manifest.Metadata.ID); err == nil || !strings.Contains(err.Error(), "lowercase") {
		t.Fatalf("expected connector id convention error, got %v", err)
	}
	manifest.Metadata.ID = "billing-invoice"
	manifest.Metadata.Owner = ""
	if err := ValidateManifest(manifest, manifest.Metadata.ID); err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("expected owner error, got %v", err)
	}
}
