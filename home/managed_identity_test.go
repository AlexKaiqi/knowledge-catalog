package home

import (
	"path/filepath"
	"reflect"
	"testing"

	"kc/identity"
	"kc/kernel"
)

func TestManagedLegacyOwnerAliasPreservesInventoryAndOriginalRequestAfterRestart(t *testing.T) {
	cfg, ws, creates := managedFixture(t)
	original := ManagedRepositoryRequest{CatalogID: cfg.Catalogs[0].ID, RepositoryID: "kr://managed/historical", CommandID: "original-command", Principal: "gitea:42", Name: "historical notes"}
	grants := 0
	grant := func(g ManagedRepositoryGrant) error { grants++; return nil }
	created, err := ws.CreateManagedRepository(original, grant)
	if err != nil {
		t.Fatal(err)
	}
	records, err := loadManagedRecords(ws.Dir)
	if err != nil || len(records) != 1 {
		t.Fatalf("original record: %#v %v", records, err)
	}
	before := records[0]
	user := identity.VerifiedUser{Username: "kaiqidong", Provider: "gitea", Issuer: "https://identity.test", Subject: "42"}
	bindings := filepath.Join(ws.Dir, identity.Filename)
	if err := identity.Bind(bindings, user); err != nil {
		t.Fatal(err)
	}
	if rows, err := ws.ListManagedRepositories(user.Username); err != nil || len(rows) != 0 {
		t.Fatalf("login alone took historical ownership: %#v %v", rows, err)
	}
	if err := identity.MigrateLegacyAlias(bindings, original.Principal, user); err != nil {
		t.Fatal(err)
	}
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	cfg.ManagedRepositories = nil
	ws, err = OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	rows, err := ws.ListManagedRepositories(user.Username)
	if err != nil || len(rows) != 1 || rows[0].Owner != user.Username || rows[0].RepositoryID != created.RepositoryID || rows[0].ManagementURL != created.ManagementURL {
		t.Fatalf("migrated inventory lost original result: %#v %v", rows, err)
	}
	if _, err := ws.GetOwnedManagedRepository("someone", original.RepositoryID); kernel.CodeOf(err) != kernel.ErrTargetRepositoryDenied {
		t.Fatalf("unrelated user claimed migrated source: %v", err)
	}
	if _, err := ws.GetOwnedManagedRepository(original.Principal, original.RepositoryID); kernel.CodeOf(err) != kernel.ErrTargetRepositoryDenied {
		t.Fatalf("retired principal kept ownership: %v", err)
	}
	replay := original
	replay.Principal, replay.IdentityProvider, replay.IdentityIssuer, replay.IdentitySubject = user.Username, user.Provider, user.Issuer, user.Subject
	result, err := ws.CreateManagedRepository(replay, grant)
	if err != nil || result.Status != "REPLAYED" || result.Owner != user.Username || result.ManagementURL != created.ManagementURL {
		t.Fatalf("explicit old command cannot resume: %#v %v", result, err)
	}
	result, err = ws.CreateNamedManagedRepository(ManagedRepositoryRequest{CatalogID: cfg.Catalogs[0].ID, Principal: user.Username, Name: original.Name, IdentityProvider: user.Provider, IdentityIssuer: user.Issuer, IdentitySubject: user.Subject}, grant)
	if err != nil || result.RepositoryID != original.RepositoryID || result.CommandID != original.CommandID || result.Owner != user.Username {
		t.Fatalf("name retry allocated another repository: %#v %v", result, err)
	}
	replay.Name = "different request"
	if _, err := ws.CreateManagedRepository(replay, grant); kernel.CodeOf(err) != kernel.ErrIdempotencyConflict {
		t.Fatalf("alias widened command idempotency: %v", err)
	}
	after, err := loadManagedRecords(ws.Dir)
	if err != nil || !reflect.DeepEqual(before, after[0]) || grants != 1 || *creates != 1 {
		t.Fatalf("migration rewrote original allocation or reissued grant: %#v %v grants=%d creates=%d", after, err, grants, *creates)
	}
}

func TestManagedLegacyOwnerResumesUnissuedPolicyUsingOriginalGrantDigest(t *testing.T) {
	cfg, ws, creates := managedFixture(t)
	original := ManagedRepositoryRequest{CatalogID: cfg.Catalogs[0].ID, RepositoryID: "kr://managed/interrupted", CommandID: "interrupted-command", Principal: "gitea:42"}
	var first ManagedRepositoryGrant
	if _, err := ws.CreateManagedRepository(original, func(g ManagedRepositoryGrant) error {
		first = g
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "policy not yet issued")
	}); err == nil {
		t.Fatal("initial request unexpectedly completed")
	}
	user := identity.VerifiedUser{Username: "kaiqidong", Provider: "gitea", Issuer: "https://identity.test", Subject: "42"}
	if err := identity.MigrateLegacyAlias(filepath.Join(ws.Dir, identity.Filename), original.Principal, user); err != nil {
		t.Fatal(err)
	}
	replay := original
	replay.Principal = user.Username
	result, err := ws.CreateManagedRepository(replay, func(g ManagedRepositoryGrant) error {
		if !reflect.DeepEqual(g, first) {
			t.Fatalf("original policy digest input changed: %#v => %#v", first, g)
		}
		return nil
	})
	if err != nil || result.Owner != user.Username || result.ProvisioningState != "READY" || *creates != 1 {
		t.Fatalf("interrupted migrated allocation not resumed: %#v %v", result, err)
	}
}
