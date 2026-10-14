package cli

import (
	apphome "kc/home"
	"kc/identity"
	inv "kc/invocation"
	"kc/kernel"
	"path/filepath"
	"testing"
)

func TestManagedLegacyGrantAliasPreservesOriginalReceiptAndRevocation(t *testing.T) {
	for _, previouslyIssued := range []bool{false, true} {
		t.Run(map[bool]string{false: "not yet issued", true: "already issued"}[previouslyIssued], func(t *testing.T) {
			dir := t.TempDir()
			if err := inv.WriteAllow(dir, inv.AllowFile{}); err != nil {
				t.Fatal(err)
			}
			if err := identity.Initialize(filepath.Join(dir, identity.Filename)); err != nil {
				t.Fatal(err)
			}
			g := apphome.ManagedRepositoryGrant{AllocationID: "old-allocation", Principal: "gitea:42", RepositoryID: "kr://old/repo", Actions: []string{"knowledge.read"}}
			if previouslyIssued {
				if err := inv.EnsureManagedRepositoryGrant(dir, g); err != nil {
					t.Fatal(err)
				}
			}
			user := identity.VerifiedUser{Username: "kaiqidong", Provider: "gitea", Issuer: "https://identity.test", Subject: "42"}
			if _, err := migrateLegacyIdentity(dir, IdentityMigrationRequest{LegacyPrincipal: g.Principal, User: user}); err != nil {
				t.Fatal(err)
			}
			if err := inv.EnsureManagedRepositoryGrant(dir, g); err != nil {
				t.Fatal(err)
			}
			policy, err := inv.ReadAllow(dir)
			if err != nil || len(policy.Rules) != 1 || policy.Rules[0].Principal != user.Username || policy.InitialGrants[g.AllocationID] != kernel.CanonicalDigest(g) {
				t.Fatalf("alias changed policy receipt or recipient: %#v %v", policy, err)
			}
			policy.Rules = nil
			if err := inv.WriteAllow(dir, policy); err != nil {
				t.Fatal(err)
			}
			if err := inv.EnsureManagedRepositoryGrant(dir, g); err != nil {
				t.Fatal(err)
			}
			if _, err := migrateLegacyIdentity(dir, IdentityMigrationRequest{LegacyPrincipal: g.Principal, User: user}); err != nil {
				t.Fatal(err)
			}
			policy, _ = inv.ReadAllow(dir)
			if len(policy.Rules) != 0 {
				t.Fatal("alias retry restored revoked grant")
			}
		})
	}
}
