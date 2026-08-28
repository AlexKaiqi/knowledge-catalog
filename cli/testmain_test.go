package cli_test

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

var documentedOperation = regexp.MustCompile(`(?m)^\s+kc ((?:[a-z][a-z0-9-]*\s*)+)`)

func TestMain(m *testing.M) {
	code := m.Run()
	if code == 0 && os.Getenv("KC_ASSERT_E2E_COVERAGE") == "1" {
		missing := missingExercisedOperations()
		if len(missing) > 0 {
			fmt.Fprintf(os.Stderr, "end-to-end suite did not exercise public operations: %v\n", missing)
			code = 1
		}
	}
	if err := testkit.StopGitea(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

func missingExercisedOperations() []string {
	seen := map[string]bool{}
	for _, match := range documentedOperation.FindAllStringSubmatch(cli.Help, -1) {
		name := strings.TrimSpace(match[1])
		if name == "serve" || !cli.CLICommand(name) {
			continue
		}
		seen[name] = true
	}
	missing := []string{}
	for name := range seen {
		if _, ok := exercisedOperations.Load(name); !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}
