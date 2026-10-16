package cli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

func TestMain(m *testing.M) {
	if strings.TrimSpace(os.Getenv("KC_SCENE_LIVE_LAKEFS_URL")) == "" {
		_ = os.Setenv("KC_LAKEFS_CREDENTIAL", testkit.LakeFSFakeCredential)
	}
	// Product tests and embedded-home tests share one process. Ambient Client
	// coordinates must not turn `kc --home` into a remote call, and a developer
	// Taihu session in ~/.config/kc must not collide with --as test principals.
	for _, name := range []string{"KC_SERVER_URL", "KC_HOME", "KC_CATALOG", "KC_DATASET", "KC_AUTH_TOKEN", "KC_AS"} {
		_ = os.Unsetenv(name)
	}
	config, err := os.MkdirTemp("", "kc-cli-config-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = os.Setenv("KC_CONFIG_DIR", config)
	code := m.Run()
	cli.CloseComponentFixturesForTest()
	// This package-wide directory protects the developer's real login, but it
	// must remain read-only to individual tests. Session-mutating tests need
	// their own t.Setenv(KC_CONFIG_DIR, t.TempDir()) so defaults cannot leak.
	entries, configErr := os.ReadDir(config)
	if configErr != nil {
		fmt.Fprintf(os.Stderr, "inspect isolated CLI test configuration: %v\n", configErr)
		if code == 0 {
			code = 1
		}
	} else if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		fmt.Fprintf(os.Stderr, "CLI tests wrote shared client configuration (%s); session-mutating tests must isolate KC_CONFIG_DIR\n", strings.Join(names, ", "))
		if code == 0 {
			code = 1
		}
	}
	_ = os.RemoveAll(config)
	report := commandCoverageSnapshot()
	if code == 0 && os.Getenv("KC_ASSERT_E2E_COVERAGE") == "1" {
		missingCalls := commandsWithoutCalls(report)
		missingSuccesses := commandsWithoutSuccesses(report)
		if len(missingCalls) > 0 {
			fmt.Fprintf(os.Stderr, "end-to-end suite did not exercise public commands: %v\n", missingCalls)
			code = 1
		}
		if len(missingSuccesses) > 0 {
			fmt.Fprintf(os.Stderr, "end-to-end suite has no asserted successful execution for public commands: %v\n", missingSuccesses)
			code = 1
		}
		if insufficientBoundaries := commandsBelowBoundaryRequirement(report); len(insufficientBoundaries) > 0 {
			fmt.Fprintf(os.Stderr, "end-to-end suite has insufficient asserted boundaries (command: actual/required): %v\n", insufficientBoundaries)
			code = 1
		}
		fmt.Fprintf(os.Stderr, "end-to-end public command coverage: %d/%d successful; %d/%d meet risk-tiered boundary requirements\n",
			report.SuccessfulCommands, report.TotalCommands, report.CommandsMeetingBoundaryRequirement, report.TotalCommands)
	}
	if path := os.Getenv("KC_COMMAND_COVERAGE_REPORT"); path != "" {
		if err := writeCommandCoverageReport(path, report); err != nil {
			fmt.Fprintf(os.Stderr, "write command coverage report: %v\n", err)
			code = 1
		}
	}
	os.Exit(code)
}

func TestCLIFixtureSetupPreservesLiveLakeFSCredentials(t *testing.T) {
	const probe = "KC_TEST_LIVE_CREDENTIAL_PROBE"
	const expected = "KC_TEST_LIVE_CREDENTIAL_EXPECTED"
	if os.Getenv(probe) == "1" {
		if got, want := os.Getenv("KC_LAKEFS_CREDENTIAL"), os.Getenv(expected); got != want {
			t.Fatal("CLI fixture setup replaced the live deployment credential")
		}
		return
	}
	for name, credential := range map[string]string{"configured": "live-access:live-secret", "missing": ""} {
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestCLIFixtureSetupPreservesLiveLakeFSCredentials$", "-test.count=1")
			cmd.Env = append(os.Environ(), probe+"=1", expected+"="+credential,
				"KC_SCENE_LIVE_LAKEFS_URL=https://lakefs.example", "KC_LAKEFS_CREDENTIAL="+credential,
				"KC_ASSERT_E2E_COVERAGE=", "KC_COMMAND_COVERAGE_REPORT=")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("live deployment credential isolation: %v\n%s", err, output)
			}
		})
	}
}

func commandsWithoutCalls(report commandCoverageReport) []string {
	missing := []string{}
	for _, row := range report.Commands {
		if row.Calls == 0 {
			missing = append(missing, row.Command)
		}
	}
	return missing
}

func commandsWithoutSuccesses(report commandCoverageReport) []string {
	missing := []string{}
	for _, row := range report.Commands {
		if row.Successes == 0 {
			missing = append(missing, row.Command)
		}
	}
	return missing
}

func commandsBelowBoundaryRequirement(report commandCoverageReport) []string {
	insufficient := []string{}
	for _, row := range report.Commands {
		if row.Boundaries < row.RequiredBoundaries {
			insufficient = append(insufficient, fmt.Sprintf("%s: %d/%d", row.Command, row.Boundaries, row.RequiredBoundaries))
		}
	}
	return insufficient
}

func writeCommandCoverageReport(path string, report commandCoverageReport) error {
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}
