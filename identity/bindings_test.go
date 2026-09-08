package identity_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"kc/identity"
	"kc/kernel"
)

func TestVerifiedUsernameBindingsSurviveReplacementAndRejectTakeover(t *testing.T) {
	path := filepath.Join(t.TempDir(), identity.Filename)
	user := identity.VerifiedUser{Username: "kaiqidong", Provider: "gitea", Issuer: "https://id.example", Subject: "42"}
	if err := identity.Bind(path, user); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("missing durable state: %v", err)
	}
	if err := identity.Initialize(path); err != nil {
		t.Fatal(err)
	}
	if err := identity.Bind(path, user); err != nil {
		t.Fatal(err)
	}
	if err := identity.Initialize(path); err != nil {
		t.Fatal(err)
	}
	if err := identity.Bind(path, user); err != nil {
		t.Fatalf("binding replay after replacement: %v", err)
	}
	for _, change := range []struct {
		name  string
		alter func(*identity.VerifiedUser)
	}{
		{"recycled username", func(u *identity.VerifiedUser) { u.Subject = "43" }},
		{"different issuer", func(u *identity.VerifiedUser) { u.Issuer = "https://other.example" }},
		{"different provider", func(u *identity.VerifiedUser) { u.Provider = "taihu" }},
		{"renamed user", func(u *identity.VerifiedUser) { u.Username = "renamed" }},
	} {
		t.Run(change.name, func(t *testing.T) {
			altered := user
			change.alter(&altered)
			if err := identity.Bind(path, altered); kernel.CodeOf(err) != kernel.ErrForbidden {
				t.Fatalf("identity change must require explicit migration: %v", err)
			}
		})
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := identity.Bind(path, user); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("lost state was silently rebuilt: %v", err)
	}
}

func TestConcurrentBindingHasOneOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), identity.Filename)
	if err := identity.Initialize(path); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			subject := "42"
			if i%2 == 0 {
				subject = "43"
			}
			results <- identity.Bind(path, identity.VerifiedUser{Username: "kaiqidong", Provider: "gitea", Issuer: "https://id.example", Subject: subject})
		}(i)
	}
	wg.Wait()
	close(results)
	ok, denied := 0, 0
	for err := range results {
		if err == nil {
			ok++
		} else if kernel.CodeOf(err) == kernel.ErrForbidden {
			denied++
		} else {
			t.Fatal(err)
		}
	}
	if ok != 8 || denied != 8 {
		t.Fatalf("concurrent owners success=%d denied=%d", ok, denied)
	}
	if err := identity.Validate(path); err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalUsernameDoesNotSilentlyRename(t *testing.T) {
	for _, valid := range []string{"kaiqidong", "walk-provider", "team_member", "name.example", "alice1"} {
		if got, err := identity.CanonicalUsername(valid); err != nil || got != valid {
			t.Fatalf("username %q: %q %v", valid, got, err)
		}
	}
	for _, invalid := range []string{"", " Alice", "Alice", "a/b", "a:b", "a..b", "-alice", "alice_", "用户"} {
		if _, err := identity.CanonicalUsername(invalid); err == nil {
			t.Errorf("accepted %q", invalid)
		}
	}
}

func TestLegacyOwnerAliasIsExplicitAndBoundToOriginalVerifiedIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), identity.Filename)
	if err := identity.Initialize(path); err != nil {
		t.Fatal(err)
	}
	user := identity.VerifiedUser{Username: "kaiqidong", Provider: "gitea", Issuer: "https://identity.test", Subject: "42"}
	if err := identity.Bind(path, user); err != nil {
		t.Fatal(err)
	}
	if got, err := identity.ResolvePrincipal(path, "gitea:42"); err != nil || got != "gitea:42" {
		t.Fatalf("login inferred legacy ownership: %s %v", got, err)
	}
	if err := identity.MigrateLegacyAlias(path, "gitea:42", user); err != nil {
		t.Fatal(err)
	}
	if err := identity.MigrateLegacyAlias(path, "gitea:42", user); err != nil {
		t.Fatal(err)
	}
	if got, err := identity.ResolvePrincipal(path, "gitea:42"); err != nil || got != user.Username {
		t.Fatalf("alias not durable: %s %v", got, err)
	}
	other := user
	other.Username = "someone"
	if err := identity.MigrateLegacyAlias(path, "gitea:42", other); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("legacy subject silently renamed: %v", err)
	}
	other = user
	other.Subject = "43"
	if err := identity.MigrateLegacyAlias(path, "gitea:42", other); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("unrelated subject claimed old owner: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := identity.ResolvePrincipal(path, "gitea:42"); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("lost alias state silently reverted identity: %v", err)
	}
}
