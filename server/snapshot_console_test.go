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
