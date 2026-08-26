package reader_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge/reader"
)

func TestWriteCheckoutPinsAssembledObjects(t *testing.T) {
	dir := testkit.TempDir(t)
	root := filepath.Join(dir, "agent")
	pin := reader.WorkspacePin{
		WorkspaceID: "agent",
		Revision:    1,
		Repositories: map[kernel.RepositoryID]kernel.CommitID{
			"kr://acme/public/core":     "c-public",
			"kr://acme/groups/payments": "c-group",
		},
	}
	values := []reader.FederatedValue{
		{Repository: "kr://acme/public/core", Commit: "c-public", ObjectID: "policy/P-103", Value: map[string]any{"body": "public"}},
		{Repository: "kr://acme/groups/payments", Commit: "c-group", ObjectID: "policy/P-103", Value: map[string]any{"body": "group"}},
		{Repository: "kr://acme/public/core", Commit: "c-public", ObjectID: "ETLTask:job-1", Value: map[string]any{"io": map[string]any{"in": []any{"a"}}}},
		{Repository: "kr://acme/public/core", Commit: "c-public", ObjectID: "runbooks/oncall", Value: map[string]any{"text": "freeze"}},
	}
	got, err := reader.WriteCheckout(root, pin, values)
	if err != nil {
		t.Fatal(err)
	}
	if got.Objects != 4 || got.Pin.Provider != reader.GrepProvider {
		t.Fatalf("%#v", got)
	}

	raw, err := os.ReadFile(filepath.Join(root, reader.CheckoutPinFile))
	if err != nil {
		t.Fatal(err)
	}
	var onDisk reader.CheckoutPin
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatal(err)
	}
	if onDisk.WorkspaceID != "agent" || onDisk.Repositories["kr://acme/public/core"] != "c-public" {
		t.Fatalf("pin %#v", onDisk)
	}
	if onDisk.Provider != "grep" {
		t.Fatal(onDisk)
	}

	public := filepath.Join(root, "kr_acme_public_core", "policy", "P-103.json")
	group := filepath.Join(root, "kr_acme_groups_payments", "policy", "P-103.json")
	if readJSON(t, public)["body"] != "public" || readJSON(t, group)["body"] != "group" {
		t.Fatal("federated values must not override")
	}
	if _, err := os.Stat(filepath.Join(root, "kr_acme_public_core", "ETLTask:job-1.json")); err != nil {
		t.Fatal(err)
	}
	if readJSON(t, filepath.Join(root, "kr_acme_public_core", "runbooks", "oncall.json"))["text"] != "freeze" {
		t.Fatal("object_id is the path, not pathHint")
	}
	info, err := os.Stat(public)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o222 != 0 {
		t.Fatalf("checkout file must be read-only: %s", info.Mode())
	}

	later := []reader.FederatedValue{
		{Repository: "kr://acme/public/core", Commit: "c-later", ObjectID: "policy/P-103", Value: map[string]any{"body": "later"}},
	}
	pin.Repositories["kr://acme/public/core"] = "c-later"
	if _, err := reader.WriteCheckout(root, pin, later); err != nil {
		t.Fatal(err)
	}
	if readJSON(t, public)["body"] != "later" {
		t.Fatal("re-checkout replaces the tree")
	}
	if _, err := os.Stat(group); !os.IsNotExist(err) {
		t.Fatal("stale federated file must disappear on replace")
	}
}

func TestObjectCheckoutRelRejectsEscape(t *testing.T) {
	if _, err := reader.ObjectCheckoutRel("../secret"); err == nil {
		t.Fatal("expected escape")
	}
	if _, err := reader.ObjectCheckoutRel("/etc/passwd"); err == nil {
		t.Fatal("expected absolute")
	}
	rel, err := reader.ObjectCheckoutRel("policy/P-103")
	if err != nil || rel != "policy/P-103.json" {
		t.Fatal(rel, err)
	}
}

func TestWriteCheckoutRejectsEscapingObject(t *testing.T) {
	dir := testkit.TempDir(t)
	_, err := reader.WriteCheckout(filepath.Join(dir, "v"), reader.WorkspacePin{WorkspaceID: "v"}, []reader.FederatedValue{
		{Repository: "kr://acme/public/core", ObjectID: "../x", Value: map[string]any{"a": 1}},
	})
	if err == nil {
		t.Fatal("expected reject")
	}
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err, string(raw))
	}
	return out
}
