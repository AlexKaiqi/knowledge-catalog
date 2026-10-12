package controlplane

import (
	"os"
	"path/filepath"
	"testing"

	"kc/internal/jsonfile"
	"kc/kernel"
)

func TestUniqueControlStateRecoversLegacyWithoutStartupWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.json")
	want := ControlState{Proposals: map[string]Proposal{"retained": {ProposalID: "retained"}}}
	if err := jsonfile.Write(path, map[string]any{"catalogs": map[string]ControlState{"kr://test/registry": want}}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	store := NewFileControlState(path, "kr://test/registry")
	got, err := store.Load()
	if err != nil || got.Proposals["retained"].ProposalID != "retained" {
		t.Fatal(got, err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("recovery rewrote the authority")
	}
	if err := store.Save(got); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := jsonfile.Read(path, &raw); err != nil {
		t.Fatal(err)
	}
	if _, legacy := raw["catalogs"]; legacy {
		t.Fatal("explicit save retained multiple-space envelope")
	}
	reopened, err := store.Load()
	if err != nil || reopened.Proposals["retained"].ProposalID != "retained" {
		t.Fatal(reopened, err)
	}
}

func TestUniqueControlStateDoesNotMergeLegacySpaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.json")
	if err := jsonfile.Write(path, map[string]any{"catalogs": map[string]ControlState{"a": {}, "b": {}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileControlState(path, "a").Load(); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("multiple spaces were merged or misdiagnosed: %v", err)
	}
}

func TestUniqueControlStateRejectsMixedFormats(t *testing.T) {
	for _, selector := range []string{"catalogs", "Catalogs", "CATALOGS"} {
		t.Run(selector, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "control.json")
			legacy := map[string]ControlState{"old": {Proposals: map[string]Proposal{"retained": {ProposalID: "retained"}}}}
			if err := jsonfile.Write(path, map[string]any{selector: legacy}); err != nil {
				t.Fatal(err)
			}
			got, err := NewFileControlState(path, "old").Load()
			if err != nil || got.Proposals["retained"].ProposalID != "retained" {
				t.Fatal("case variant lost accepted control state", got, err)
			}
			if err := jsonfile.Write(path, map[string]any{selector: legacy, "proposals": map[string]Proposal{"other": {ProposalID: "other"}}}); err != nil {
				t.Fatal(err)
			}
			if _, err := NewFileControlState(path, "old").Load(); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("mixed state was merged or lost: %v", err)
			}
		})
	}
}

func TestUniqueControlStateRejectsForeignLegacyAuthority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.json")
	if err := jsonfile.Write(path, map[string]any{"catalogs": map[string]ControlState{"foreign": {Proposals: map[string]Proposal{"accepted": {ProposalID: "accepted"}}}}}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileControlState(path, "configured").Load(); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("foreign control state was adopted: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("rejected recovery changed durable state", err)
	}
}
