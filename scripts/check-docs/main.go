// Command check-docs validates the maintained documentation.
//
// The design documents live under doc/. Every document there is reachable from
// doc/README.md, starts with a title instead of frontmatter, and only links to
// local files that exist. Root, task and package documents get the same link
// check so that moving or deleting a design document cannot leave dead links.
// The task board under tasks/ is checked for layout (see tasks.go) and links.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	docRoot  = "doc"
	docIndex = "doc/README.md"
)

var (
	mdLink = regexp.MustCompile(`\]\(([^)#\s]+)`)
	// rootMarkdown is the closed set of Markdown files allowed at the repository root.
	rootMarkdown = map[string]struct{}{"README.md": {}, "AGENTS.md": {}}
	// maintainedMarkdown lists tracked notes outside doc/ that are not README files.
	maintainedMarkdown = []string{"cli/SURFACE.md"}
	skippedDirectories = map[string]struct{}{
		"node_modules": {}, "vendor": {}, "dist": {}, "dist-ssr": {}, "build": {},
		"coverage": {}, "test-results": {}, "playwright-report": {},
	}
)

func main() {
	root, err := repoRoot()
	if err != nil {
		fatal(err)
	}
	docs, err := checkDocumentation(root)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("documentation: PASS (%d documents under %s/, reachable from %s, no frontmatter, valid links)\n",
		docs, docRoot, docIndex)
}

func checkDocumentation(root string) (int, error) {
	docs, err := designDocuments(root)
	if err != nil {
		return 0, err
	}
	if err := validateRootMarkdown(root); err != nil {
		return 0, err
	}
	for _, doc := range docs {
		if err := validateDocumentShape(root, doc); err != nil {
			return 0, err
		}
	}
	if err := validateReachable(root, docs); err != nil {
		return 0, err
	}
	tasks, err := taskDocuments(root)
	if err != nil {
		return 0, err
	}
	files, err := markdownLinkFiles(root, append(append([]string{}, docs...), tasks...))
	if err != nil {
		return 0, err
	}
	if err := validateMarkdownLinks(root, files); err != nil {
		return 0, err
	}
	return len(docs), nil
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		if filepath.Dir(dir) == dir {
			return "", fmt.Errorf("go.mod not found from %s", wd)
		}
	}
}

// designDocuments lists every Markdown file under doc/, relative to root.
func designDocuments(root string) ([]string, error) {
	var docs []string
	err := filepath.WalkDir(filepath.Join(root, docRoot), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".md") {
			docs = append(docs, relPath(root, path))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%s/: %w", docRoot, err)
	}
	sort.Strings(docs)
	for _, doc := range docs {
		if doc == docIndex {
			return docs, nil
		}
	}
	return nil, fmt.Errorf("%s is missing", docIndex)
}

func validateRootMarkdown(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if _, ok := rootMarkdown[entry.Name()]; !ok {
			return fmt.Errorf("root Markdown %s is not allowed; design documents belong under %s/, tasks under %s/", entry.Name(), docRoot, taskRoot)
		}
	}
	return nil
}

// validateDocumentShape keeps design documents parseable as plain Markdown:
// the first non-empty line is a level-one title, never a YAML frontmatter fence.
func validateDocumentShape(root, doc string) error {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(doc)))
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			continue
		case line == "---":
			return fmt.Errorf("%s starts with frontmatter; design documents start with a # title", doc)
		case strings.HasPrefix(line, "# "):
			return nil
		default:
			return fmt.Errorf("%s must start with a # title", doc)
		}
	}
	return fmt.Errorf("%s is empty", doc)
}

// validateReachable requires every design document to be linked, directly or
// through other design documents, from doc/README.md.
func validateReachable(root string, docs []string) error {
	known := map[string]struct{}{}
	for _, doc := range docs {
		known[doc] = struct{}{}
	}
	seen := map[string]struct{}{docIndex: {}}
	queue := []string{docIndex}
	for len(queue) > 0 {
		doc := queue[0]
		queue = queue[1:]
		targets, err := localLinks(root, doc)
		if err != nil {
			return err
		}
		for _, target := range targets {
			if _, ok := known[target]; !ok {
				continue
			}
			if _, ok := seen[target]; ok {
				continue
			}
			seen[target] = struct{}{}
			queue = append(queue, target)
		}
	}
	var unreachable []string
	for _, doc := range docs {
		if _, ok := seen[doc]; !ok {
			unreachable = append(unreachable, doc)
		}
	}
	if len(unreachable) > 0 {
		return fmt.Errorf("design documents not reachable from %s:\n  %s", docIndex, strings.Join(unreachable, "\n  "))
	}
	return nil
}

// localLinks returns the repository-relative targets of relative links in doc.
func localLinks(root, doc string) ([]string, error) {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(doc)))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, match := range mdLink.FindAllStringSubmatch(string(raw), -1) {
		target := match[1]
		if external(target) || strings.HasPrefix(target, "/") {
			continue
		}
		out = append(out, filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(doc), filepath.FromSlash(target)))))
	}
	return out, nil
}

func external(target string) bool {
	return strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "mailto:")
}

func validateMarkdownLinks(root string, files []string) error {
	var broken []string
	for _, rel := range files {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		for _, match := range mdLink.FindAllStringSubmatch(string(raw), -1) {
			target := match[1]
			if external(target) {
				continue
			}
			resolved := target
			if !strings.HasPrefix(target, "/") {
				resolved = filepath.Join(root, filepath.Dir(filepath.FromSlash(rel)), filepath.FromSlash(target))
			}
			if local, err := filepath.Rel(root, resolved); err == nil && strings.HasPrefix(filepath.ToSlash(local), ".validation/") {
				broken = append(broken, fmt.Sprintf("%s -> %s (local validation evidence is not checked in; use a code path)", rel, target))
				continue
			}
			if _, err := os.Stat(resolved); err != nil {
				broken = append(broken, fmt.Sprintf("%s -> %s", rel, target))
			}
		}
	}
	if len(broken) > 0 {
		return fmt.Errorf("broken local Markdown link:\n  %s", strings.Join(broken, "\n  "))
	}
	return nil
}

// markdownLinkFiles adds root, task, package and other maintained notes to the
// design documents for the link check. Hidden and generated trees are skipped.
func markdownLinkFiles(root string, docs []string) ([]string, error) {
	root = filepath.Clean(root)
	paths := map[string]struct{}{}
	for _, doc := range docs {
		paths[doc] = struct{}{}
	}
	for _, rel := range maintainedMarkdown {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			paths[rel] = struct{}{}
		}
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			if _, skip := skippedDirectories[d.Name()]; skip || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		rel := relPath(root, path)
		if _, ok := rootMarkdown[rel]; ok || d.Name() == "README.md" {
			paths[rel] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(paths))
	for path := range paths {
		files = append(files, path)
	}
	sort.Strings(files)
	return files, nil
}

func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "%v\n", err)
	os.Exit(1)
}
