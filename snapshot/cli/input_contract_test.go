package cli_test

import (
	"context"
	"path/filepath"
	"testing"

	"kc/kernel"
	snapshotcli "kc/snapshot/cli"
	"kc/snapshot/client"
)

func TestModuleParameterContractOwnsReservedFlagsAcrossHostLeaves(t *testing.T) {
	executor := snapshotcli.New(nil, snapshotcli.Context{})
	for _, row := range []struct {
		path, flag, message string
	}{
		{"writer commit", "metadata", "unknown flag --metadata"},
		{"files read", "metadata", "unknown flag --metadata"},
		{"writer commit", "offset", "--offset is only valid for kc files read"},
		{"dataset define", "length", "--length is only valid for kc files read"},
		{"files list", "expected-file", "--expected-file is only valid for kc grant update"},
	} {
		flags := map[string]any{row.flag: "value"}
		for _, err := range []error{executor.ValidateFlags(row.path, flags), executor.Validate(row.path, flags)} {
			fault := kernel.AsIngress(err)
			if fault == nil || fault.Code != kernel.ErrUsageInvalid || fault.Message != row.message {
				t.Fatalf("%s --%s changed its range error: %v", row.path, row.flag, err)
			}
		}
	}
	if err := executor.ValidateFlags("writer commit", map[string]any{"dir": "workspace", "bulk": true, "repo": []string{"a", "b"}}); err != nil {
		t.Fatalf("module constrained a host-owned flag: %v", err)
	}
	// Keep the existing order when multiple reserved flags are invalid.
	if fault := kernel.AsIngress(executor.ValidateFlags("writer commit", map[string]any{"metadata": "{}", "offset": "0"})); fault == nil || fault.Message != "unknown flag --metadata" {
		t.Fatalf("reserved-flag error precedence changed: %v", fault)
	}
}

func TestModuleParameterContractChecksWhitelistAndRepeatedValuesOnly(t *testing.T) {
	executor := snapshotcli.New(nil, snapshotcli.Context{})
	for _, row := range []struct {
		flags   map[string]any
		message string
	}{
		{map[string]any{"ref": "main"}, "--ref is not valid for kc files read"},
		{map[string]any{"path": []string{"one", "two"}}, "--path must be supplied once"},
		{map[string]any{"path": true}, "--path requires a value"},
	} {
		if fault := kernel.AsIngress(executor.ValidateFlags("files read", row.flags)); fault == nil || fault.Message != row.message {
			t.Fatalf("whitelist/repetition error changed: %v", fault)
		}
	}
	for _, path := range []string{"files read", "files list", "grant update", "dataset show"} {
		if err := executor.ValidateFlags(path, nil); err != nil {
			t.Fatalf("parameter contract required semantic input for %s: %v", path, err)
		}
	}
	if err := executor.ValidateFlags("dataset define", map[string]any{"source": []string{"one@", "two@"}, "metadata": "not parsed by parameter validation"}); err != nil {
		t.Fatalf("nil Flags lost existing repeated-source semantics: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "not-readable")
	if err := executor.ValidateFlags("grant update", map[string]any{"file": missing, "expected-file": missing}); err != nil {
		t.Fatalf("parameter validation read a rule file: %v", err)
	}
	if err := executor.ValidateFlags("dataset define", map[string]any{"file": missing}); err != nil {
		t.Fatalf("parameter validation read a recipe file: %v", err)
	}
}

func TestModuleHelpBypassesSemanticInputsAfterParameterValidation(t *testing.T) {
	executor := snapshotcli.New(nil, snapshotcli.Context{})
	for _, path := range []string{"dataset define", "grant update", "files read"} {
		flags := map[string]any{"help": true}
		if path == "grant update" {
			flags["file"], flags["expected-file"] = "missing-replacement", "missing-observation"
		}
		if err := executor.Validate(path, flags); err != nil {
			t.Fatalf("help required semantic operands for %s: %v", path, err)
		}
		if value, err := executor.Execute(context.Background(), path, flags, client.RequestOptions{}); err != nil || value == nil {
			t.Fatalf("help required HTTP or file input for %s: %#v %v", path, value, err)
		}
	}
}
