package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	apphome "kc/home"
	"kc/identity"
	"kc/kernel"
)

func TestManagedInitialGrantIsScopedAndNeverRestoredByRetry(t *testing.T) {
	dir := t.TempDir()
	if err := WriteAllow(dir, AllowFile{}); err != nil {
		t.Fatal(err)
	}
	g := apphome.ManagedRepositoryGrant{AllocationID: "allocation-a", Principal: "user:provider", RepositoryID: "kr://provider/new", Actions: []string{"writer.preview", "writer.commit", "knowledge.read"}}
	if err := ensureManagedRepositoryGrant(dir, g); err != nil {
		t.Fatal(err)
	}
	policy, err := ReadAllow(dir)
	if err != nil || len(policy.Rules) != 1 {
		t.Fatalf("initial policy: %#v %v", policy, err)
	}
	for _, action := range g.Actions {
		if _, ok := MatchAllow(policy.Rules, AllowQuery{Principal: g.Principal, Repo: g.RepositoryID, Action: action}); !ok {
			t.Fatalf("creator cannot %s", action)
		}
	}
	for _, q := range []AllowQuery{
		{Principal: "user:other", Repo: g.RepositoryID, Action: "writer.commit"},
		{Principal: g.Principal, Repo: "kr://provider/other", Action: "writer.commit"},
		{Principal: g.Principal, Repo: g.RepositoryID, Action: "admin.grants.manage"},
	} {
		if _, ok := MatchAllow(policy.Rules, q); ok {
			t.Fatalf("initial policy escalates: %#v", q)
		}
	}
	// Revoke through the ordinary operation. Its save must retain the durable
	// creation receipt, including if provisioning crashed before its final ACK.
	if _, err := verbRevoke(&invocation{Home: dir, Flags: map[string]FlagValue{"id": policy.Rules[0].ID}}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "allow.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureManagedRepositoryGrant(dir, g); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(dir, "allow.json"))
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("retry rewrote revoked policy: %v", err)
	}
	policy, err = ReadAllow(dir)
	if err != nil || len(policy.Rules) != 0 {
		t.Fatalf("retry restored revoked permissions: %#v %v", policy, err)
	}
	changed := g
	changed.Principal = "user:other"
	if err := ensureManagedRepositoryGrant(dir, changed); kernel.CodeOf(err) != kernel.ErrIdempotencyConflict {
		t.Fatalf("allocation was transferable: %v", err)
	}
}

func TestManagedLegacyGrantAliasPreservesOriginalReceiptAndRevocation(t *testing.T) {
	for _, previouslyIssued := range []bool{false, true} {
		t.Run(map[bool]string{false: "not yet issued", true: "already issued"}[previouslyIssued], func(t *testing.T) {
			dir := t.TempDir()
			if err := WriteAllow(dir, AllowFile{}); err != nil {
				t.Fatal(err)
			}
			if err := identity.Initialize(filepath.Join(dir, identity.Filename)); err != nil {
				t.Fatal(err)
			}
			g := apphome.ManagedRepositoryGrant{AllocationID: "old-allocation", Principal: "gitea:42", RepositoryID: "kr://old/repo", Actions: []string{"knowledge.read"}}
			if previouslyIssued {
				if err := ensureManagedRepositoryGrant(dir, g); err != nil {
					t.Fatal(err)
				}
			}
			user := identity.VerifiedUser{Username: "kaiqidong", Provider: "gitea", Issuer: "https://identity.test", Subject: "42"}
			if _, err := MigrateLegacyIdentity(dir, IdentityMigrationRequest{LegacyPrincipal: g.Principal, User: user}); err != nil {
				t.Fatal(err)
			}
			if err := ensureManagedRepositoryGrant(dir, g); err != nil {
				t.Fatal(err)
			}
			policy, err := ReadAllow(dir)
			if err != nil || len(policy.Rules) != 1 || policy.Rules[0].Principal != user.Username || policy.InitialGrants[g.AllocationID] != kernel.CanonicalDigest(g) {
				t.Fatalf("alias changed policy receipt or recipient: %#v %v", policy, err)
			}
			policy.Rules = nil
			if err := WriteAllow(dir, policy); err != nil {
				t.Fatal(err)
			}
			if err := ensureManagedRepositoryGrant(dir, g); err != nil {
				t.Fatal(err)
			}
			if _, err := MigrateLegacyIdentity(dir, IdentityMigrationRequest{LegacyPrincipal: g.Principal, User: user}); err != nil {
				t.Fatal(err)
			}
			policy, _ = ReadAllow(dir)
			if len(policy.Rules) != 0 {
				t.Fatal("alias retry restored revoked grant")
			}
		})
	}
}

func TestManagedInitialGrantRequiresDurablePolicy(t *testing.T) {
	dir := t.TempDir()
	g := apphome.ManagedRepositoryGrant{AllocationID: "allocation-a", Principal: "user:provider", RepositoryID: "kr://provider/new", Actions: []string{"knowledge.read"}}
	if err := ensureManagedRepositoryGrant(dir, g); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("missing policy accepted as empty: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "allow.json")); !os.IsNotExist(err) {
		t.Fatalf("missing policy recreated: %v", err)
	}
}
