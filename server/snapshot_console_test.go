package server

import (
	"kc/home"
	"kc/identity"
	"testing"
)

func TestSnapshotConsoleLinkUsesConfiguredInstanceAndActualNativeName(t *testing.T) {
	bindings := []home.HomeRepo{{ID: "kr://org/logical-name", Driver: "lakefs", DSN: "https://native.test/platform/physical-files"}}
	caller := identity.HTTPIdentity{Principal: "reader"}
	origin := "https://native.test/platform/"
	if got := snapshotConsoleRepository(bindings, caller, "kr://org/logical-name", origin); got != "physical-files" {
		t.Fatalf("native link guessed logical identity: %q", got)
	}
	if got := snapshotConsoleRepository(bindings, caller, "kr://org/unregistered", origin); got != "" {
		t.Fatalf("unregistered source acquired a console link: %q", got)
	}
	if got := snapshotConsoleRepository(bindings, caller, "kr://org/logical-name", "https://other.test/platform"); got != "" {
		t.Fatalf("different lakeFS instance acquired a local console link: %q", got)
	}
	if got := snapshotConsoleRepository(bindings, identity.HTTPIdentity{}, "kr://org/logical-name", origin); got != "" {
		t.Fatalf("unauthenticated caller acquired a local console link: %q", got)
	}
	// A caller's own storage-account claim never selects the instance: lakeFS
	// is storage, not an identity provider.
	claimed := identity.HTTPIdentity{Principal: "reader", User: &identity.VerifiedUser{Provider: "lakefs", Issuer: "https://native.test/platform"}}
	if got := snapshotConsoleRepository(bindings, claimed, "kr://org/logical-name", ""); got != "" {
		t.Fatalf("caller storage claim selected a console instance: %q", got)
	}
}

func TestExternalConsoleLinkUsesConfiguredStoreWithoutImpersonatingLakeFSIdentity(t *testing.T) {
	bindings := []home.HomeRepo{{ID: "kr://org/logical", Driver: "lakefs", DSN: "https://native.test/files"}}
	caller := identity.HTTPIdentity{Principal: "alice", User: &identity.VerifiedUser{Provider: "taihu", Issuer: "https://identity.test", Subject: "123"}}
	if got := snapshotConsoleRepository(bindings, caller, "kr://org/logical", "https://native.test"); got != "files" {
		t.Fatalf("external identity lost registered native navigation: %q", got)
	}
	if got := snapshotConsoleRepository(bindings, caller, "kr://org/logical", "https://other.test"); got != "" {
		t.Fatal("navigation crossed native instances")
	}
}
