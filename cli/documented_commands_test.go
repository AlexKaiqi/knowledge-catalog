package cli

import (
	"html"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	inv "kc/invocation"
)

// Examples must use the same registered paths and flag boundaries as the
// product. This checks syntax without logging in, opening stores or issuing
// requests; domain values and successful journeys remain conformance's job.
func TestProductDocumentationUsesCurrentCommands(t *testing.T) {
	groups := map[string][]string{
		"README.md":           {"README.md"},
		"doc":                 documentationMarkdown(t, filepath.Join("..", "doc")),
		"product.html":        {"doc/product.html"},
		"object/reader guide": {"object/reader/README.md"},
	}
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
	lines := documentedCommandLines(path, string(raw))
	for _, line := range lines {
		if err := validateDocumentedCommand(line); err != nil {
			t.Errorf("%s: example %q: %v", path, line, err)
		}
	}
	return len(lines)
}

func documentedCommandLines(path, raw string) []string {
	pattern := "(?s)```(?:bash|sh|shell)\\n(.*?)```"
	isHTML := filepath.Ext(path) == ".html"
	if isHTML {
		pattern = `(?is)<pre\b[^>]*>\s*<code\b[^>]*>(.*?)</code\s*>\s*</pre\s*>`
	}
	var lines []string
	for _, block := range regexp.MustCompile(pattern).FindAllStringSubmatch(raw, -1) {
		text := block[1]
		if isHTML {
			text = html.UnescapeString(text)
		}
		text = strings.ReplaceAll(text, "\\\n", " ")
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "kc ") {
				continue
			}
			line, _, _ = strings.Cut(line, " #")
			lines = append(lines, line)
		}
	}
	return lines
}

func TestDocumentedCommandGuardReadsHTMLCodeBlocks(t *testing.T) {
	raw := `<p>kc retired-in-prose</p><code>kc retired-inline</code>
<pre>kc retired-pre-only</pre>
<pre class="shell"><code class="language-shell"># Example
kc help consume
kc files read --dataset notes \
  --path &lt;file&gt; --out &#39;./a&amp;b&#39; # local target
</code></pre>
<pre><code>kc &#112;in --source kr://example/source</code></pre>`
	want := []string{
		"kc help consume",
		"kc files read --dataset notes    --path <file> --out './a&b'",
		"kc pin --source kr://example/source",
	}
	got := documentedCommandLines("product.html", raw)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("HTML command examples = %#v, want %#v", got, want)
	}
	for _, line := range got[:2] {
		if err := validateDocumentedCommand(line); err != nil {
			t.Errorf("current HTML command rejected: %s: %v", line, err)
		}
	}
	if err := validateDocumentedCommand(got[2]); err == nil || !strings.Contains(err.Error(), "unknown command pin") {
		t.Fatalf("retired HTML command error = %v, want unknown command pin", err)
	}
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
		"kc read urn:kc:object:note/a --pin pin.json",
		"kc search --source kr://example/source --query note",
		"kc read --repo kr://example/source --dataset notes --object note/a",
		"kc read --reference urn:kc:object:note/a#/description --commit historical",
		"kc schema list --repo kr://example/source --dataset notes",
		"kc read urn:kc:object:note/a --unknown-flag value",
		"kc read --repo kr://example/source --object note/a --aspect description",
		"kc relations --repo kr://example/source --object note/a",
	} {
		if err := validateDocumentedCommand(line); err == nil {
			t.Errorf("invalid product example accepted: %s", line)
		}
	}
	for _, line := range []string{
		"kc read urn:kc:object:note/a --commit historical",
		"kc read urn:kc:object:note/a#/description --commit historical",
		"kc search --dataset notes --query note",
		"kc schema list --dataset notes --revision 2",
		"kc dataset define notes --revision 1 --source kr://example/source",
		"kc help consume",
		"kc serve --config deployment.yaml",
	} {
		if err := validateDocumentedCommand(line); err != nil {
			t.Errorf("current product example rejected: %s: %v", line, err)
		}
	}
}
