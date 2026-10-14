package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	inv "kc/invocation"
)

// Examples must use the same registered paths and flag boundaries as the
// product. This checks syntax without logging in, opening stores or issuing
// requests; domain values and successful journeys remain conformance's job.
func TestProductDocumentationUsesCurrentCommands(t *testing.T) {
	groups := map[string][]string{"README.md": {"README.md"}, "doc": documentationMarkdown(t, filepath.Join("..", "doc"))}
	for name, paths := range groups {
		t.Run(name, func(t *testing.T) {
			count := 0
			for _, path := range paths {
				count += checkDocumentedCommands(t, path)
			}
			if count == 0 {
				t.Fatal("no executable kc examples checked")
			}
			t.Logf("checked %d command examples against the product surface", count)
		})
	}
}

func documentationMarkdown(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && filepath.Ext(path) == ".md" {
			rel, _ := filepath.Rel("..", path)
			paths = append(paths, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("no Markdown under %s", root)
	}
	return paths
}

func checkDocumentedCommands(t *testing.T, path string) int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", path))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, block := range regexp.MustCompile("(?s)```(?:bash|sh|shell)\\n(.*?)```").FindAllStringSubmatch(string(raw), -1) {
		text := strings.ReplaceAll(block[1], "\\\n", " ")
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "kc ") {
				continue
			}
			line, _, _ = strings.Cut(line, " #")
			count++
			if err := validateDocumentedCommand(line); err != nil {
				t.Errorf("%s: example %q: %v", path, line, err)
			}
		}
	}
	return count
}

func validateDocumentedCommand(line string) error {
	parsed, err := ParseArgs(strings.Fields(line)[1:])
	if err != nil {
		return err
	}
	if parsed.Command == "help" {
		_, err := helpFor(strings.Join(parsed.Args, " "))
		return err
	}
	if parsed.Command == "serve" {
		return inv.RejectServeFlags(parsed.Flags)
	}
	_, positionals, err := resolveCLICommand(parsed.Command, parsed.Args)
	if err != nil {
		return err
	}
	publicPath := strings.Join(append([]string{parsed.Command}, parsed.Args[:len(parsed.Args)-len(positionals)]...), " ")
	if err := rejectHostFlags(publicPath, parsed.Flags); err != nil {
		return err
	}
	if err := rejectPublicSurfaceFlags(publicPath, parsed.Flags); err != nil {
		return err
	}
	return inv.RejectServeOnlyFlags(parsed.Flags)
}

func TestDocumentedCommandGuardRejectsRetiredAndMixedScopes(t *testing.T) {
	for _, line := range []string{
		"kc pin --source kr://example/source",
		"kc read urn:kc:object:kr%3A%2F%2Fexample%2Fsource:note%2Fa --pin pin.json",
		"kc search --source kr://example/source --query note",
		"kc read --repo kr://example/source --dataset notes --object note/a",
		"kc read --reference urn:kc:object:kr%3A%2F%2Fexample%2Fsource:note%2Fa#/description --commit historical",
		"kc schema list --dataset notes",
		"kc read urn:kc:object:kr%3A%2F%2Fexample%2Fsource:note%2Fa --unknown-flag value",
	} {
		if err := validateDocumentedCommand(line); err == nil {
			t.Errorf("invalid product example accepted: %s", line)
		}
	}
	for _, line := range []string{
		"kc read urn:kc:object:kr%3A%2F%2Fexample%2Fsource:note%2Fa --commit historical",
		"kc read urn:kc:object:kr%3A%2F%2Fexample%2Fsource:note%2Fa#/description --commit historical",
		"kc search --dataset notes --query note",
		"kc dataset define notes --revision 1 --source kr://example/source",
		"kc help consume",
		"kc serve --config deployment.yaml",
	} {
		if err := validateDocumentedCommand(line); err != nil {
			t.Errorf("current product example rejected: %s: %v", line, err)
		}
	}
}
