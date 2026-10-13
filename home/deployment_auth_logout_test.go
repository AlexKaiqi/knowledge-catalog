package home

import "testing"

func TestDeploymentLogoutOnlyUsesTrustedNavigation(t *testing.T) {
	for _, raw := range []string{"", "/auth/end-session?next=%2Fui", "https://identity.example/logout"} {
		if err := validateAuthLogoutURL(raw); err != nil {
			t.Fatalf("trusted URL rejected: %s: %v", raw, err)
		}
	}
	for _, raw := range []string{"//untrusted.example/logout", "javascript:alert(1)", "https://user:secret@identity.example/logout", "/\\untrusted", "https://identity.example/\nlogout"} {
		if validateAuthLogoutURL(raw) == nil {
			t.Fatalf("unsafe URL accepted: %q", raw)
		}
	}
}
