package arch_test

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	testCaseID      = regexp.MustCompile(`^[A-Z]+-[0-9]+$`)
	documentedTest  = regexp.MustCompile("`(Test[A-Za-z0-9_]+)`")
	implementedTest = regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]+)\(`)
)

// TestCatalogIsMachineCheckable keeps docs/TEST_CATALOG.md useful as an
// acceptance manifest instead of a hand-maintained status essay.
func TestCatalogIsMachineCheckable(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "docs", "TEST_CATALOG.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	knownTests := collectGoTests(t, root)
	seen := map[string]int{}
	caseCount := 0
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := scanner.Text()
		columns := markdownColumns(line)
		if len(columns) == 0 || !testCaseID.MatchString(columns[0]) {
			continue
		}
		caseCount++
		id := columns[0]
		if previous := seen[id]; previous != 0 {
			t.Errorf("%s:%d duplicates case %s first declared at line %d", path, lineNo, id, previous)
		}
		seen[id] = lineNo
		if len(columns) != 6 {
			t.Errorf("%s:%d case %s has %d columns; want ID/前置/操作/预期/现况/已有测试", path, lineNo, id, len(columns))
			continue
		}
		status := strings.ToLower(columns[4])
		if !containsAny(status, "ok", "partial", "gap", "frozen") {
			t.Errorf("%s:%d case %s has unrecognized status %q", path, lineNo, id, columns[4])
		}
		if !strings.Contains(status, "frozen") && (columns[5] == "" || columns[5] == "—") {
			t.Errorf("%s:%d non-frozen case %s has no test evidence", path, lineNo, id)
		}
		for _, match := range documentedTest.FindAllStringSubmatch(columns[5], -1) {
			if !knownTests[match[1]] {
				t.Errorf("%s:%d case %s references missing Go test %s", path, lineNo, id, match[1])
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if caseCount < 150 {
		t.Fatalf("test catalog unexpectedly shrank to %d cases", caseCount)
	}
}

func TestArchitectureInvariantsHaveExecutableEvidence(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "docs", "ARCHITECTURE_INVARIANTS.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	knownTests := collectGoTests(t, root)
	seen := map[string]int{}
	count := 0
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	for lineNo := 1; scanner.Scan(); lineNo++ {
		columns := markdownColumns(scanner.Text())
		if len(columns) == 0 || !testCaseID.MatchString(columns[0]) {
			continue
		}
		count++
		id := columns[0]
		if previous := seen[id]; previous != 0 {
			t.Errorf("%s:%d duplicates invariant %s first declared at line %d", path, lineNo, id, previous)
		}
		seen[id] = lineNo
		if len(columns) != 4 {
			t.Errorf("%s:%d invariant %s has %d columns; want ID/property/forbidden/evidence", path, lineNo, id, len(columns))
			continue
		}
		evidence := documentedTest.FindAllStringSubmatch(columns[3], -1)
		if len(evidence) == 0 {
			t.Errorf("%s:%d invariant %s has no executable Go test evidence", path, lineNo, id)
		}
		for _, match := range evidence {
			if !knownTests[match[1]] {
				t.Errorf("%s:%d invariant %s references missing Go test %s", path, lineNo, id, match[1])
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count < 19 {
		t.Fatalf("architecture invariant registry unexpectedly shrank to %d entries", count)
	}
}

func collectGoTests(t *testing.T, root string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range implementedTest.FindAllStringSubmatch(string(body), -1) {
			out[string(match[1])] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func markdownColumns(line string) []string {
	if !strings.HasPrefix(strings.TrimSpace(line), "|") {
		return nil
	}
	line = strings.TrimSpace(line)
	var columns []string
	var current strings.Builder
	inCode := false
	escaped := false
	for i, r := range line {
		if i == 0 && r == '|' {
			continue
		}
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			current.WriteRune(r)
			continue
		}
		if r == '`' {
			inCode = !inCode
			current.WriteRune(r)
			continue
		}
		if r == '|' && !inCode {
			columns = append(columns, strings.TrimSpace(current.String()))
			current.Reset()
			continue
		}
		current.WriteRune(r)
	}
	if strings.TrimSpace(current.String()) != "" {
		columns = append(columns, strings.TrimSpace(current.String()))
	}
	return columns
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
