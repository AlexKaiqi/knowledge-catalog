package invocation

import (
	apphome "kc/home"
	"kc/invoke"
	"kc/kernel"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestManagedInitialGrantIsScopedAndNeverRestoredByRetry(t *testing.T) {
	dir := t.TempDir()
	if err := WriteAllow(dir, AllowFile{}); err != nil {
		t.Fatal(err)
	}
	g := apphome.ManagedRepositoryGrant{AllocationID: "allocation-a", Principal: "provider", RepositoryID: "kr://provider/new", Actions: []string{"writer.preview", "writer.commit", "knowledge.read"}}
	if err := EnsureManagedRepositoryGrant(dir, g); err != nil {
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
		{Principal: "other", Repo: g.RepositoryID, Action: "writer.commit"},
		{Principal: g.Principal, Repo: "kr://provider/other", Action: "writer.commit"},
		{Principal: g.Principal, Repo: g.RepositoryID, Action: "admin.grants.manage"},
	} {
		if _, ok := MatchAllow(policy.Rules, q); ok {
			t.Fatalf("initial policy escalates: %#v", q)
		}
	}
	// Revoke through the ordinary operation. Its save must retain the durable
	// creation receipt, including if provisioning crashed before its final ACK.
	if _, err := VerbRevoke(&Invocation{Home: dir, Flags: map[string]invoke.FlagValue{"id": policy.Rules[0].ID}}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "allow.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureManagedRepositoryGrant(dir, g); err != nil {
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
	changed.Principal = "other"
	if err := EnsureManagedRepositoryGrant(dir, changed); kernel.CodeOf(err) != kernel.ErrIdempotencyConflict {
		t.Fatalf("allocation was transferable: %v", err)
	}
}

func TestManagedInitialGrantRequiresDurablePolicy(t *testing.T) {
	dir := t.TempDir()
	g := apphome.ManagedRepositoryGrant{AllocationID: "allocation-a", Principal: "provider", RepositoryID: "kr://provider/new", Actions: []string{"knowledge.read"}}
	if err := EnsureManagedRepositoryGrant(dir, g); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("missing policy accepted as empty: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "allow.json")); !os.IsNotExist(err) {
		t.Fatalf("missing policy recreated: %v", err)
	}
}
