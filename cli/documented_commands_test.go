package cli

import (
	"html"
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
	for _, path := range []string{"README.md", "docs/product.html", "docs/reviewed/mvp-acceptance.md"} {
		t.Run(path, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", path))
			if err != nil {
				t.Fatal(err)
			}
			pattern := "(?s)```(?:bash|sh|shell)\\n(.*?)```"
			if filepath.Ext(path) == ".html" {
				pattern = `(?s)<pre\b[^>]*>\s*<code[^>]*>(.*?)</code>\s*</pre>`
			}
			count := 0
			for _, block := range regexp.MustCompile(pattern).FindAllStringSubmatch(string(raw), -1) {
				text := strings.ReplaceAll(html.UnescapeString(block[1]), "\\\n", " ")
				for _, line := range strings.Split(text, "\n") {
					line = strings.TrimSpace(line)
					if !strings.HasPrefix(line, "kc ") {
						continue
					}
					line, _, _ = strings.Cut(line, " #")
					count++
					if err := validateDocumentedCommand(line); err != nil {
						t.Errorf("example %q: %v", line, err)
					}
				}
			}
			if count == 0 {
				t.Fatal("no executable kc examples checked")
			}
			t.Logf("checked %d command examples against the product surface", count)
		})
	}
}

func validateDocumentedCommand(line string) error {
	parsed, err := ParseArgs(strings.Fields(line)[1:])
	if err != nil {
		return err
	}
	if err := inv.RejectUnknownFlags(parsed.Flags); err != nil {
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
	if err := rejectPublicSurfaceFlags(publicPath, parsed.Flags); err != nil {
		return err
	}
	return inv.RejectServeOnlyFlags(parsed.Flags)
}

func TestDocumentedCommandGuardRejectsRetiredAndMixedScopes(t *testing.T) {
	for _, line := range []string{
		"kc pin --source kr://example/source",
		"kc read --pin pin.json --object note/a",
		"kc search --source kr://example/source --query note",
		"kc read --repo kr://example/source --dataset notes --object note/a",
		"kc schema list --dataset notes",
		"kc read --repo kr://example/source --unknown-flag value",
	} {
		if err := validateDocumentedCommand(line); err == nil {
			t.Errorf("invalid product example accepted: %s", line)
		}
	}
	for _, line := range []string{
		"kc read --repo kr://example/source --commit historical --object note/a",
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
