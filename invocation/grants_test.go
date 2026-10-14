package invocation

import (
	"context"
	"reflect"
	"testing"

	"kc/invoke"
	"kc/kernel"
)

func TestGrantLegacyCLIQuestionRequiresGlobalReadAndRetainsEvaluation(t *testing.T) {
	dir := t.TempDir()
	if err := WriteAllow(dir, AllowFile{Rules: []AllowRule{
		{ID: "global-read", Principal: "operator", Actions: []string{"admin.grants.read"}},
		{ID: "scoped-read", Principal: "scoped", Repo: "repo-A", Actions: []string{"admin.grants.read"}},
		{ID: "wildcard", Principal: "reader", Repo: "repo-A", Object: "data/*", Actions: []string{"object.read"}},
	}}); err != nil {
		t.Fatal(err)
	}
	flags := map[string]invoke.FlagValue{"as": "operator", "principal": "reader", "action": "object.read", "repo": "repo-A", "object": "data/file"}
	operation := Command{Stage: invoke.StageHome, Run: VerbAllowed}
	value, err := ExecuteApplicationOperation(context.Background(), "grant-list", "admin.grants.read", operation, flags, nil, nil, dir, nil)
	if err != nil || !reflect.DeepEqual(value, map[string]any{"allow": true, "ruleId": "wildcard"}) {
		t.Fatalf("legacy rule evaluation changed: %#v %v", value, err)
	}
	flags["as"] = "scoped"
	if _, err := ExecuteApplicationOperation(context.Background(), "grant-list", "admin.grants.read", operation, flags, nil, nil, dir, nil); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("scoped inventory read evaluated global rules: %v", err)
	}
	delete(flags, "action")
	delete(flags, "object")
	value, err = ExecuteApplicationOperation(context.Background(), "grant-list", "admin.grants.read", operation, flags, nil, nil, dir, nil)
	if err != nil || len(value.(GrantList).Rules) != 1 || value.(GrantList).Rules[0].ID != "wildcard" {
		t.Fatalf("scoped CLI inventory stopped working: %#v %v", value, err)
	}
}

func TestGrantIDsRemainDistinctAfterRevocationAndStaleIDCannotRevokeReplacement(t *testing.T) {
	dir := t.TempDir()
	add := func(principal string) AllowRule {
		t.Helper()
		value, err := VerbAllow(&Invocation{Home: dir, Flags: map[string]invoke.FlagValue{"principal": principal, "repo": "repo-A", "action": "object.read"}})
		if err != nil {
			t.Fatal(err)
		}
		return value.(AllowRule)
	}
	first, kept := add("first"), add("kept")
	if _, err := VerbRevoke(&Invocation{Home: dir, Flags: map[string]invoke.FlagValue{"id": first.ID}}); err != nil {
		t.Fatal(err)
	}
	replacement := add("replacement")
	if replacement.ID == kept.ID || replacement.ID == first.ID {
		t.Fatalf("reused grant identity: first=%s kept=%s replacement=%s", first.ID, kept.ID, replacement.ID)
	}
	if _, err := VerbRevoke(&Invocation{Home: dir, Flags: map[string]invoke.FlagValue{"id": first.ID}}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("stale ID revocation: %v", err)
	}
	file, err := ReadAllow(dir)
	if err != nil || !reflect.DeepEqual(file.Rules, []AllowRule{kept, replacement}) {
		t.Fatalf("stale ID changed current grants: %#v %v", file, err)
	}
}

func TestGrantRevokeRejectsAmbiguousLegacyIDWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	file := AllowFile{Rules: []AllowRule{{ID: "legacy", Principal: "one"}, {ID: "legacy", Principal: "two"}}}
	if err := WriteAllow(dir, file); err != nil {
		t.Fatal(err)
	}
	if _, err := VerbRevoke(&Invocation{Home: dir, Flags: map[string]invoke.FlagValue{"id": "legacy"}}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("ambiguous legacy ID was accepted: %v", err)
	}
	after, err := ReadAllow(dir)
	if err != nil || !reflect.DeepEqual(after.Rules, file.Rules) {
		t.Fatalf("ambiguous revoke changed rules: %#v %v", after, err)
	}
}
