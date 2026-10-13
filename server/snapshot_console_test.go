package server

import (
	"kc/home"
	"kc/identity"
	"testing"
)

func TestSnapshotConsoleLinkUsesVerifiedInstanceAndActualNativeName(t *testing.T) {
	bindings := []home.HomeRepo{{ID: "kr://org/logical-name", Driver: "lakefs", DSN: "https://native.test/platform/physical-files"}}
	caller := identity.HTTPIdentity{Principal: "reader", User: &identity.VerifiedUser{Provider: "lakefs", Issuer: "https://native.test/platform/"}}
	if got := snapshotConsoleRepository(bindings, caller, "kr://org/logical-name"); got != "physical-files" {
		t.Fatalf("native link guessed logical identity: %q", got)
	}
	if got := snapshotConsoleRepository(bindings, caller, "kr://org/unregistered"); got != "" {
		t.Fatalf("unregistered source acquired a console link: %q", got)
	}
	caller.User.Issuer = "https://other.test/platform"
	if got := snapshotConsoleRepository(bindings, caller, "kr://org/logical-name"); got != "" {
		t.Fatalf("different lakeFS instance acquired a local console link: %q", got)
	}
	caller.User.Issuer = "https://native.test/platform"
	caller.User.Provider = "taihu"
	if got := snapshotConsoleRepository(bindings, caller, "kr://org/logical-name"); got != "" {
		t.Fatalf("unverified native instance acquired a local console link: %q", got)
	}
	caller.User = nil
	if got := snapshotConsoleRepository(bindings, caller, "kr://org/logical-name"); got != "" {
		t.Fatalf("missing caller proof acquired a local console link: %q", got)
	}
}

func TestExternalConsoleLinkUsesConfiguredStoreWithoutImpersonatingLakeFSIdentity(t *testing.T) {
	bindings := []home.HomeRepo{{ID: "kr://org/logical", Driver: "lakefs", DSN: "https://native.test/files"}}
	caller := identity.HTTPIdentity{Principal: "alice", User: &identity.VerifiedUser{Provider: "gitea", Issuer: "https://identity.test", Subject: "123"}}
	if got := snapshotConsoleRepository(bindings, caller, "kr://org/logical", "https://native.test"); got != "files" {
		t.Fatalf("external identity lost registered native navigation: %q", got)
	}
	if got := snapshotConsoleRepository(bindings, caller, "kr://org/logical", "https://other.test"); got != "" {
		t.Fatal("navigation crossed native instances")
	}
}
