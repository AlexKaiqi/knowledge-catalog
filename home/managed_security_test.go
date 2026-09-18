package home

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// The static source is deliberately not a managed allocation. Reusing the
// immutable System snapshot lets this boundary test avoid a backend process.
type managedSecurityStaticSource struct {
	snapshot.Store
	id kernel.RepositoryID
}

func (s managedSecurityStaticSource) ID() kernel.RepositoryID { return s.id }

func TestManagedRepositoryRejectsReservedAndStaticIdentitiesWithoutEffects(t *testing.T) {
	cfg, ws, creates := managedFixture(t)
	const staticID = "kr://managed/static"
	if err := ws.Store.Add(managedSecurityStaticSource{Store: knowledge.NewSystemRepository(), id: staticID}); err != nil {
		t.Fatal(err)
	}
	ws.Deployment.Repositories = append(ws.Deployment.Repositories, RepositoryBinding{ID: staticID, Driver: "gitea", DSN: cfg.ManagedRepositories.DSN + "/static"})
	before, err := ws.Registry.Head()
	if err != nil {
		t.Fatal(err)
	}
	grants := 0
	for _, id := range []string{string(knowledge.SystemRepositoryID), cfg.Catalogs[0].ID, staticID} {
		t.Run(id, func(t *testing.T) {
			_, err := ws.CreateManagedRepository(ManagedRepositoryRequest{CatalogID: cfg.Catalogs[0].ID, RepositoryID: id, CommandID: "reserved-" + id, Principal: "alice"}, func(ManagedRepositoryGrant) error { grants++; return nil })
			if err == nil {
				t.Fatal("reserved or statically bound identity was provisioned")
			}
		})
	}
	after, err := ws.Registry.Head()
	if err != nil {
		t.Fatal(err)
	}
	if *creates != 0 || grants != 0 || before != after {
		t.Fatalf("rejected identities caused effects: creates=%d grants=%d head=%s -> %s", *creates, grants, before, after)
	}
}

func TestManagedRepositoryAllocationCannotBeTakenByAnotherRequest(t *testing.T) {
	for _, pending := range []bool{false, true} {
		name := "ready"
		if pending {
			name = "grant-pending"
		}
		t.Run(name, func(t *testing.T) {
			cfg, ws, creates := managedFixture(t)
			req := ManagedRepositoryRequest{CatalogID: cfg.Catalogs[0].ID, RepositoryID: "kr://managed/owned", CommandID: "original", Principal: "alice"}
			grants := 0
			_, err := ws.CreateManagedRepository(req, func(ManagedRepositoryGrant) error {
				grants++
				if pending {
					return errors.New("grant unavailable")
				}
				return nil
			})
			if (err != nil) != pending || *creates != 1 || grants != 1 {
				t.Fatalf("initial allocation: creates=%d grants=%d err=%v", *creates, grants, err)
			}
			for _, competitor := range []ManagedRepositoryRequest{
				{CatalogID: req.CatalogID, RepositoryID: req.RepositoryID, CommandID: req.CommandID, Principal: "bob"},
				{CatalogID: req.CatalogID, RepositoryID: req.RepositoryID, CommandID: "other-command", Principal: req.Principal},
				{CatalogID: req.CatalogID, RepositoryID: req.RepositoryID, CommandID: "other-command", Principal: "bob"},
			} {
				if _, err := ws.CreateManagedRepository(competitor, func(ManagedRepositoryGrant) error { grants++; return nil }); err == nil {
					t.Fatalf("competing request took allocation: %#v", competitor)
				}
			}
			if *creates != 1 || grants != 1 {
				t.Fatalf("competing requests caused effects: creates=%d grants=%d", *creates, grants)
			}
		})
	}
}

func TestManagedRepositoryCommandCannotChangeCatalogOrRepository(t *testing.T) {
	cfg, first, creates := managedFixture(t)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(t.TempDir(), "second-catalog")
	cfg.Catalogs = append(cfg.Catalogs, CatalogBinding{ID: "kr://managed/second-catalog", Driver: "dolt", Dir: second})
	if err := InitializeDeployment(cfg, nil); err != nil {
		t.Fatal(err)
	}
	ws, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	req := ManagedRepositoryRequest{CatalogID: cfg.Catalogs[0].ID, RepositoryID: "kr://managed/stable-request", CommandID: "same-command", Principal: "alice"}
	grants := 0
	if _, err := ws.CreateManagedRepository(req, func(ManagedRepositoryGrant) error { grants++; return nil }); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"catalog", "repository"} {
		changed := req
		if field == "catalog" {
			changed.CatalogID = cfg.Catalogs[1].ID
		} else {
			changed.RepositoryID = "kr://managed/changed-request"
		}
		if _, err := ws.CreateManagedRepository(changed, func(ManagedRepositoryGrant) error { grants++; return nil }); kernel.CodeOf(err) != kernel.ErrIdempotencyConflict {
			t.Errorf("changed %s must conflict with original command: %v", field, err)
		}
	}
	if *creates != 1 || grants != 1 || ws.Catalogs[cfg.Catalogs[1].ID].HasRepository(kernel.RepositoryID(req.RepositoryID)) {
		t.Fatalf("changed request caused effects: creates=%d grants=%d", *creates, grants)
	}
}

func TestManagedRepositoryReadyReplayRejectsLostCatalogAuthority(t *testing.T) {
	cfg, ws, creates := managedFixture(t)
	req := ManagedRepositoryRequest{CatalogID: cfg.Catalogs[0].ID, RepositoryID: "kr://managed/authority-loss", CommandID: "create", Principal: "alice"}
	grants := 0
	grant := func(ManagedRepositoryGrant) error { grants++; return nil }
	if _, err := ws.CreateManagedRepository(req, grant); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(cfg.Catalogs[0].Dir); err != nil {
		t.Fatalf("remove Catalog authority: %v", err)
	}
	if _, err := ws.CreateManagedRepository(req, grant); err == nil {
		t.Fatal("READY replay returned success after Catalog authority was lost")
	}
	if *creates != 1 || grants != 1 {
		t.Fatalf("lost authority replay repeated effects: creates=%d grants=%d", *creates, grants)
	}
}

func TestManagedRepositoryStateLossCannotBeReinitialized(t *testing.T) {
	for _, missing := range []string{deploymentMarker, managedLedgerFile} {
		t.Run(missing, func(t *testing.T) {
			cfg, ws, creates := managedFixture(t)
			if err := ws.Close(); err != nil {
				t.Fatal(err)
			}
			policyBefore, err := os.ReadFile(filepath.Join(cfg.StateDir, "allow.json"))
			if err != nil {
				t.Fatal(err)
			}
			missingPath := filepath.Join(cfg.StateDir, missing)
			if err := os.Remove(missingPath); err != nil {
				t.Fatalf("initialized deployment must have %s: %v", missing, err)
			}
			if err := ValidateDeploymentState(cfg); err == nil {
				t.Fatal("incomplete durable volume validated")
			}
			if reopened, err := OpenDeployment(cfg); err == nil {
				_ = reopened.Close()
				t.Fatal("incomplete durable volume reopened")
			}
			seeds := 0
			if err := InitializeDeployment(cfg, func(string, string) error { seeds++; return nil }); err == nil {
				t.Fatal("initialization repaired missing durable evidence")
			}
			if _, err := os.Stat(missingPath); !os.IsNotExist(err) {
				t.Fatalf("missing durable file recreated: %v", err)
			}
			policyAfter, err := os.ReadFile(filepath.Join(cfg.StateDir, "allow.json"))
			if err != nil {
				t.Fatal(err)
			}
			if seeds != 0 || *creates != 0 || string(policyBefore) != string(policyAfter) {
				t.Fatalf("state loss reset policy or sources: seeds=%d creates=%d", seeds, *creates)
			}
		})
	}
}

func TestManagedRepositoryRecoveryRejectsLostLedgerInitializationReceipt(t *testing.T) {
	cfg, ws, _ := managedFixture(t)
	req := ManagedRepositoryRequest{CatalogID: cfg.Catalogs[0].ID, RepositoryID: "kr://managed/receipt-loss", CommandID: "create", Principal: "alice"}
	if _, err := ws.CreateManagedRepository(req, func(ManagedRepositoryGrant) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	state, err := readDeploymentState(cfg)
	if err != nil {
		t.Fatal(err)
	}
	state.ManagedStoreInitialized = false
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.StateDir, deploymentMarker), raw, 0600); err != nil {
		t.Fatal(err)
	}
	// Disabling new provisioning is supported, but must not turn an existing
	// allocation ledger into an ignored legacy file after receipt damage.
	cfg.ManagedRepositories = nil
	if err := ValidateDeploymentState(cfg); err == nil {
		t.Error("existing allocations were accepted without their initialization receipt")
	}
	if reopened, err := OpenDeployment(cfg); err == nil {
		_ = reopened.Close()
		t.Error("recovery silently omitted managed bindings with a missing receipt")
	}
}
