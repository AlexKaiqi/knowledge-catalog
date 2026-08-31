package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

func TestCommandCoverageGateKeepsEvidenceDimensionsIndependent(t *testing.T) {
	report := commandCoverageReport{Commands: []commandCoverageRow{
		{Command: "never-called", RequiredBoundaries: 1},
		{Command: "failure-only", RequiredBoundaries: 1, operationEvidence: operationEvidence{Calls: 1, Boundaries: 1}},
		{Command: "success-without-boundary", RequiredBoundaries: 1, operationEvidence: operationEvidence{Calls: 1, SuccessfulRuns: 1, Successes: 1}},
		{Command: "write-with-one-boundary", RequiredBoundaries: 2, operationEvidence: operationEvidence{Calls: 2, SuccessfulRuns: 1, Successes: 1, Boundaries: 1}},
		{Command: "complete", RequiredBoundaries: 2, operationEvidence: operationEvidence{Calls: 3, SuccessfulRuns: 1, Successes: 1, Boundaries: 2}},
	}}
	if got, want := commandsWithoutCalls(report), []string{"never-called"}; !slices.Equal(got, want) {
		t.Fatalf("missing calls = %v, want %v", got, want)
	}
	if got, want := commandsWithoutSuccesses(report), []string{"never-called", "failure-only"}; !slices.Equal(got, want) {
		t.Fatalf("missing successes = %v, want %v", got, want)
	}
	if got, want := commandsBelowBoundaryRequirement(report), []string{
		"never-called: 0/1",
		"success-without-boundary: 0/1",
		"write-with-one-boundary: 1/2",
	}; !slices.Equal(got, want) {
		t.Fatalf("missing boundaries = %v, want %v", got, want)
	}
}

func TestRequiredAssertedBoundariesFollowSemanticRisk(t *testing.T) {
	for _, tc := range []struct {
		action string
		want   int
	}{
		{"knowledge.read", 1},
		{"catalog.audit.read", 1},
		{"catalog.manage", 2},
		{"operations.hooks.manage", 2},
		{"governance.proposal.create", 2},
		{"writer.commit", 2},
		{"writer.preview", 2},
		{"local.init", 2},
		{"local.repository.attach", 2},
		{"local.workspace.overlay", 2},
		{"feedback.write", 2},
	} {
		if got := requiredAssertedBoundaries(tc.action); got != tc.want {
			t.Errorf("requiredAssertedBoundaries(%q) = %d, want %d", tc.action, got, tc.want)
		}
	}
}

func TestCommandCoverageReportUsesTheCompletePublicSurface(t *testing.T) {
	report := commandCoverageSnapshot()
	commands := cli.CLICommandsForTest()
	if report.TotalCommands != len(commands) || len(report.Commands) != len(commands) {
		t.Fatalf("coverage report has %d/%d rows for %d public commands", len(report.Commands), report.TotalCommands, len(commands))
	}
	for i, command := range commands {
		if report.Commands[i].Command != command {
			t.Fatalf("coverage report row %d = %q, want %q", i, report.Commands[i].Command, command)
		}
		if report.Commands[i].Action == "" || report.Commands[i].RequiredBoundaries == 0 {
			t.Fatalf("coverage report row %q lacks action or boundary requirement: %#v", command, report.Commands[i])
		}
	}
	path := filepath.Join(t.TempDir(), "command-coverage.json")
	if err := writeCommandCoverageReport(path, report); err != nil {
		t.Fatal(err)
	}
	var decoded commandCoverageReport
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.TotalCommands != len(commands) || len(decoded.Commands) != len(commands) {
		t.Fatalf("written coverage report lost commands: %#v", decoded)
	}
}

func TestCommandSpecificUsageBoundaries(t *testing.T) {
	home := testkit.TempDir(t)
	repositoryID := "kr://acme/public/boundaries"
	body(t, kc(home, "local", "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(home, "local", "repository", "attach", "--repo", repositoryID))

	cases := []struct {
		name string
		args []string
	}{
		{"grant add requires a principal", []string{"admin", "grant", "add"}},
		{"grant remove requires an id", []string{"admin", "grant", "remove"}},
		{"repository archive requires a repository", []string{"catalog", "repository", "archive"}},
		{"workspace retire requires a workspace", []string{"catalog", "workspace", "retire"}},
		{"preview create requires a proposal", []string{"governance", "preview", "create"}},
		{"validation record requires an outcome", []string{"governance", "validation", "record"}},
		{"binding resolve requires an aspect", []string{"knowledge", "binding", "resolve", "--repo", repositoryID, "--object", "Service:x"}},
		{"provenance requires an object", []string{"knowledge", "provenance", "--repo", repositoryID}},
		{"schema describe requires a target", []string{"knowledge", "schema", "describe"}},
		{"overlay requires a workspace", []string{"local", "workspace", "overlay"}},
		{"trace requires a trace id", []string{"operations", "audit", "trace"}},
		{"gate remove requires an id", []string{"operations", "gate", "remove"}},
		{"projection describe requires a repository", []string{"operations", "projection", "describe"}},
		{"projection sync requires a repository", []string{"operations", "projection", "sync"}},
		{"writer remove requires a command id", []string{"writer", "remove"}},
		{"writer head requires a repository", []string{"writer", "head"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expectCode(t, kc(home, tc.args...), "USAGE_INVALID")
		})
	}
}

func TestReadOnlyCommandAuthorizationAndIdentityBoundaries(t *testing.T) {
	home := testkit.TempDir(t)
	body(t, kc(home, "local", "init", "--catalog", "kr://acme/catalog"))
	for _, tc := range []struct {
		name string
		args []string
		code string
	}{
		{"repository list does not enumerate for an unauthorized principal", []string{"catalog", "repository", "list", "--as", "untrusted"}, "FORBIDDEN"},
		{"workspace list does not enumerate for an unauthorized principal", []string{"catalog", "workspace", "list", "--as", "untrusted"}, "FORBIDDEN"},
		{"whoami rejects a malformed principal", []string{"identity", "whoami", "--as", "un\x00trusted"}, "USAGE_INVALID"},
		{"access audit does not enumerate for an unauthorized principal", []string{"operations", "audit", "access", "--as", "untrusted"}, "FORBIDDEN"},
		{"hitmap does not enumerate for an unauthorized principal", []string{"operations", "audit", "hitmap", "--as", "untrusted"}, "FORBIDDEN"},
		{"gate list does not enumerate for an unauthorized principal", []string{"operations", "gate", "list", "--as", "untrusted"}, "FORBIDDEN"},
		{"hook list does not enumerate for an unauthorized principal", []string{"operations", "hook", "list", "--as", "untrusted"}, "FORBIDDEN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expectCode(t, kc(home, tc.args...), tc.code)
		})
	}
}

func TestMutatingCommandsRejectInvalidStateTargetsAndAuthorization(t *testing.T) {
	home := testkit.TempDir(t)
	repositoryID := "kr://acme/public/mutation-boundaries"
	body(t, kc(home, "local", "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(home, "local", "repository", "attach", "--repo", repositoryID))
	body(t, kc(home, "catalog", "workspace", "define", "--workspace", "coverage", "--revision", "1",
		"--source", repositoryID+"=refs/heads/main@knowledge"))
	ingestDir := t.TempDir()

	for _, tc := range []struct {
		name string
		args []string
		code string
	}{
		{"grant add rejects a non-semantic action", []string{"admin", "grant", "add", "--principal", "agent:x", "--action", "invalid", "--repo", repositoryID}, "USAGE_INVALID"},
		{"grant remove rejects an unknown rule", []string{"admin", "grant", "remove", "--id", "alw_missing"}, "USAGE_INVALID"},
		{"catalog archive rejects an unauthorized principal", []string{"catalog", "archive", "--as", "untrusted"}, "FORBIDDEN"},
		{"repository archive rejects an unknown target", []string{"catalog", "repository", "archive", "--repo", "kr://missing/repository"}, "USAGE_INVALID"},
		{"workspace retire rejects an unknown target", []string{"catalog", "workspace", "retire", "--workspace", "missing"}, "WORKSPACE_INVALID"},
		{"preview create rejects an unknown proposal", []string{"governance", "preview", "create", "--proposal", "PR-missing", "--workspace", "coverage"}, "USAGE_INVALID"},
		{"preview validate rejects an unknown preview", []string{"governance", "preview", "validate", "--preview", "PV-missing"}, "USAGE_INVALID"},
		{"validation record rejects an invalid outcome", []string{"governance", "validation", "record", "--outcome", "UNKNOWN"}, "USAGE_INVALID"},
		{"overlay rejects file and clear together", []string{"local", "workspace", "overlay", "--workspace", "coverage", "--file", "ignored", "--clear"}, "USAGE_INVALID"},
		{"feedback rejects an invalid outcome", []string{"operations", "feedback", "record", "--workspace", "coverage", "--trace-id", "trace-x", "--outcome", "UNKNOWN"}, "USAGE_INVALID"},
		{"gate remove rejects an unknown rule", []string{"operations", "gate", "remove", "--id", "gate_missing"}, "USAGE_INVALID"},
		{"hook remove requires an id", []string{"operations", "hook", "remove"}, "USAGE_INVALID"},
		{"projection sync rejects an unknown repository", []string{"operations", "projection", "sync", "--repo", "kr://missing/repository"}, "USAGE_INVALID"},
		{"writer commit requires a changeset", []string{"writer", "commit", "--command-id", "missing-changeset"}, "USAGE_INVALID"},
		{"writer ingest requires a repository", []string{"writer", "ingest", "--dir", ingestDir}, "USAGE_INVALID"},
		{"writer head rejects an unknown repository", []string{"writer", "head", "--repo", "kr://missing/repository"}, "USAGE_INVALID"},
		{"writer remove rejects an unknown repository", []string{"writer", "remove", "--command-id", "remove-missing", "--repo", "kr://missing/repository", "--object", "Policy:x"}, "USAGE_INVALID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expectCode(t, kc(home, tc.args...), tc.code)
		})
	}
}

func TestCatalogArchiveRejectsAnUnknownExplicitTarget(t *testing.T) {
	home := testkit.TempDir(t)
	body(t, kc(home, "local", "init", "--catalog", "kr://acme/archive-boundary"))
	expectCode(t, kc(home, "catalog", "archive", "--catalog", "kr://missing/catalog"), "USAGE_INVALID")
}
