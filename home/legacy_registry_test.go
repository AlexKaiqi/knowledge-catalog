package home

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
	"kc/kernel"
)

func legacyMarker(t *testing.T, cfg DeploymentConfig) []byte {
	t.Helper()
	path := filepath.Join(cfg.StateDir, deploymentMarker)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "registryAuthority")
	fields["initializedCatalogs"], err = json.Marshal([]string{cfg.Registry.ID})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return raw
}

func legacyAllocation(t *testing.T, dir, repo, authority string) []byte {
	t.Helper()
	var raw []byte
	if err := managedDB(dir, true, func(tx *bolt.Tx) error {
		bucket := tx.Bucket(managedRecordsBucket)
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(bucket.Get([]byte(repo)), &fields); err != nil {
			return err
		}
		var request map[string]any
		if err := json.Unmarshal(fields["request"], &request); err != nil {
			return err
		}
		request["CatalogID"] = authority
		body, err := json.Marshal(request)
		if err != nil {
			return err
		}
		fields["request"] = body
		fields["digest"], err = json.Marshal(kernel.CanonicalDigest(json.RawMessage(body)))
		if err != nil {
			return err
		}
		raw, err = json.Marshal(fields)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(repo), raw)
	}); err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestLegacySingleRegistryRestoresWithoutWritingOrRegranting(t *testing.T) {
	cfg, ws, creates := managedFixture(t)
	req := ManagedRepositoryRequest{RepositoryID: "kr://managed/legacy", CommandID: "legacy-command", Principal: "alice"}
	grants := 0
	original, err := ws.CreateManagedRepository(req, func(ManagedRepositoryGrant) error { grants++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	marker := legacyMarker(t, cfg)
	allocation := legacyAllocation(t, cfg.StateDir, req.RepositoryID, cfg.Registry.ID)
	policyPath := filepath.Join(cfg.StateDir, "allow.json")
	policy, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	replay, err := restored.CreateManagedRepository(req, func(ManagedRepositoryGrant) error { grants++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if replay.Head != original.Head || replay.Status != "REPLAYED" || grants != 1 || *creates != 1 {
		t.Fatalf("legacy replay changed allocation or reissued policy: %#v grants=%d creates=%d", replay, grants, *creates)
	}
	for path, want := range map[string][]byte{filepath.Join(cfg.StateDir, deploymentMarker): marker, policyPath: policy} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("read recovery rewrote %s: %v", path, err)
		}
	}
	if err := managedDB(cfg.StateDir, false, func(tx *bolt.Tx) error {
		if !bytes.Equal(tx.Bucket(managedRecordsBucket).Get([]byte(req.RepositoryID)), allocation) {
			t.Fatal("READY replay rewrote the original allocation receipt")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyRegistryReceiptRejectsDifferentAuthority(t *testing.T) {
	cfg, ws, _ := managedFixture(t)
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	legacyMarker(t, cfg)
	cfg.Registry.ID = "kr://managed/different"
	if _, err := OpenDeployment(cfg); err == nil {
		t.Fatal("legacy receipt accepted a different registry authority")
	}
}

func TestLegacyRegistryReceiptsRejectMultipleAndMixedAuthorities(t *testing.T) {
	for _, body := range []string{
		`{"version":1,"initializedCatalogs":["kr://one/registry","kr://two/registry"]}`,
		`{"version":1,"initializedCatalogs":["kr://one/registry"],"registryAuthority":"kr://one/registry"}`,
		`{"version":1,"initializedCatalogs":["kr://one/registry"],"InitializedCatalogs":["kr://other/registry"]}`,
		`{"version":1,"initializedCatalogs":["kr://other/registry"],"initializedCatalogs":["kr://one/registry"]}`,
	} {
		var state deploymentState
		if err := json.Unmarshal([]byte(body), &state); err == nil {
			t.Fatalf("ambiguous historical authority accepted: %s", body)
		}
	}
}

func TestLegacyManagedAllocationRejectsDifferentRegistry(t *testing.T) {
	cfg, ws, _ := managedFixture(t)
	req := ManagedRepositoryRequest{RepositoryID: "kr://managed/different-history", CommandID: "historical", Principal: "alice"}
	if _, err := ws.CreateManagedRepository(req, func(ManagedRepositoryGrant) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	legacyAllocation(t, cfg.StateDir, req.RepositoryID, "kr://different/registry")
	if _, err := OpenDeployment(cfg); err == nil {
		t.Fatal("legacy managed allocation crossed registry authority")
	}
}

func TestLegacyPendingAllocationMigratesOnlyOnExplicitRetry(t *testing.T) {
	cfg, ws, creates := managedFixture(t)
	req := ManagedRepositoryRequest{RepositoryID: "kr://managed/pending-history", CommandID: "historical-pending", Principal: "alice"}
	if _, err := ws.CreateManagedRepository(req, func(ManagedRepositoryGrant) error {
		return kernel.Fail(kernel.ErrPreconditionFailed, "policy not issued")
	}); err == nil {
		t.Fatal("pending grant expected")
	}
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	raw := legacyAllocation(t, cfg.StateDir, req.RepositoryID, cfg.Registry.ID)
	restored, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := managedDB(cfg.StateDir, false, func(tx *bolt.Tx) error {
		if !bytes.Equal(tx.Bucket(managedRecordsBucket).Get([]byte(req.RepositoryID)), raw) {
			t.Fatal("startup migrated pending allocation")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	grants := 0
	if _, err := restored.CreateManagedRepository(req, func(ManagedRepositoryGrant) error { grants++; return nil }); err != nil {
		t.Fatal(err)
	}
	if grants != 1 || *creates != 1 {
		t.Fatalf("retry reprovisioned or reissued policy: grants=%d creates=%d", grants, *creates)
	}
	if err := managedDB(cfg.StateDir, false, func(tx *bolt.Tx) error {
		body := tx.Bucket(managedRecordsBucket).Get([]byte(req.RepositoryID))
		if bytes.Contains(body, []byte("CatalogID")) {
			t.Fatal("explicit retry retained public Catalog request")
		}
		_, err := decodeManagedRecord(body)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
