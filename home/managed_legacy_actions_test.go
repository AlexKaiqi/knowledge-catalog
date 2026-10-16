package home

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	bolt "go.etcd.io/bbolt"
	"kc/kernel"
)

func replaceManagedActions(t *testing.T, dir, repo string, actions, share []string, phase string) []byte {
	t.Helper()
	var raw []byte
	if err := managedDB(dir, true, func(tx *bolt.Tx) error {
		bucket := tx.Bucket(managedRecordsBucket)
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(bucket.Get([]byte(repo)), &fields); err != nil {
			return err
		}
		fields["actions"], _ = json.Marshal(actions)
		fields["shareActions"], _ = json.Marshal(share)
		fields["phase"], _ = json.Marshal(phase)
		var err error
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

func TestManagedHistoricalActionsRestoreWithoutChangingReceiptsOrRegranting(t *testing.T) {
	cfg, ws, creates := managedFixture(t)
	req := ManagedRepositoryRequest{RepositoryID: "kr://managed/historical-actions", CommandID: "historical-actions", Principal: "alice"}
	grants := 0
	original, err := ws.CreateManagedRepository(req, func(ManagedRepositoryGrant) error { grants++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	legacy := managedLegacyActionPrefix
	actions := []string{"writer.commit", legacy + "read", legacy + "schema.read", "object.search"}
	allocation := replaceManagedActions(t, cfg.StateDir, req.RepositoryID, actions, []string{legacy + "read"}, managedReady)
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
	if err != nil || replay.Head != original.Head || replay.Status != "REPLAYED" || grants != 1 || *creates != 1 {
		t.Fatalf("historical replay changed allocation or reissued policy: %#v grants=%d creates=%d err=%v", replay, grants, *creates, err)
	}
	if got, err := os.ReadFile(policyPath); err != nil || !bytes.Equal(got, policy) {
		t.Fatalf("historical recovery rewrote authorization policy: %v", err)
	}
	if err := managedDB(cfg.StateDir, false, func(tx *bolt.Tx) error {
		raw := tx.Bucket(managedRecordsBucket).Get([]byte(req.RepositoryID))
		if !bytes.Equal(raw, allocation) {
			t.Fatal("historical recovery rewrote the original allocation receipt")
		}
		record, err := decodeManagedRecord(raw)
		if err == nil && !reflect.DeepEqual(record.Actions, actions) {
			t.Fatal("historical recovery replaced the frozen creator policy")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestManagedHistoricalActionCompatibilityKeepsTheNarrowPolicy(t *testing.T) {
	cfg, ws, _ := managedFixture(t)
	req := ManagedRepositoryRequest{RepositoryID: "kr://managed/action-boundary", CommandID: "action-boundary", Principal: "alice"}
	if _, err := ws.CreateManagedRepository(req, func(ManagedRepositoryGrant) error { return nil }); err != nil {
		t.Fatal(err)
	}
	legacy := managedLegacyActionPrefix
	for _, actions := range [][]string{{legacy + "*"}, {legacy + "manage"}, {legacy + "read", "object.read"}, {"identity.grant"}} {
		raw := replaceManagedActions(t, cfg.StateDir, req.RepositoryID, actions, nil, managedReady)
		if _, err := decodeManagedRecord(raw); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
			t.Fatalf("historical creator policy accepted %v: %v", actions, err)
		}
	}
	raw := replaceManagedActions(t, cfg.StateDir, req.RepositoryID, []string{legacy + "read"}, []string{"writer.commit"}, managedReady)
	if _, err := decodeManagedRecord(raw); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("historical sharing policy widened to write: %v", err)
	}
	if validateCreatorActions([]string{legacy + "read"}) == nil || validateShareActions([]string{legacy + "read"}) == nil {
		t.Fatal("historical compatibility admitted retired spellings in new configuration")
	}
}

func TestManagedHistoricalActionsAcceptOnlyTheCompletedFrozenReadPolicy(t *testing.T) {
	cfg, ws, creates := managedFixture(t)
	req := ManagedRepositoryRequest{RepositoryID: "kr://managed/historical-phases", CommandID: "historical-phases", Principal: "alice"}
	grants := 0
	if _, err := ws.CreateManagedRepository(req, func(ManagedRepositoryGrant) error { grants++; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"read", "schema.read", "provenance", "history.read", "search", "relations"} {
		t.Run(suffix, func(t *testing.T) {
			actions := []string{managedLegacyActionPrefix + suffix}
			raw := replaceManagedActions(t, cfg.StateDir, req.RepositoryID, actions, actions, managedReady)
			record, err := decodeManagedRecord(raw)
			if err != nil || !reflect.DeepEqual(record.Actions, actions) || !reflect.DeepEqual(record.ShareActions, actions) {
				t.Fatalf("completed historical read policy changed: %#v %v", record, err)
			}
			if validateCreatorActions(actions) == nil || validateShareActions(actions) == nil {
				t.Fatal("historical action was accepted for new configuration")
			}
		})
	}
	for _, phase := range []string{managedReserved, managedOwned, managedRegistered} {
		t.Run(phase, func(t *testing.T) {
			actions := []string{managedLegacyActionPrefix + "read"}
			replaceManagedActions(t, cfg.StateDir, req.RepositoryID, actions, nil, phase)
			path := filepath.Join(cfg.StateDir, managedLedgerFile)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if restored, err := OpenDeployment(cfg); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				if restored != nil {
					_ = restored.Close()
				}
				t.Fatalf("incomplete historical creation was silently accepted: %v", err)
			}
			if after, err := os.ReadFile(path); err != nil || !bytes.Equal(before, after) || grants != 1 || *creates != 1 {
				t.Fatalf("rejected historical recovery changed state or effects: %v grants=%d creates=%d", err, grants, *creates)
			}
		})
	}
}
