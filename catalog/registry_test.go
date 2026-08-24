package catalog_test

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"kc/catalog"
)

func TestRegistryWritesFlatYAML(t *testing.T) {
	s := setupFed(t)
	if _, err := s.catalog.DefineWorkspace("duty", 1, []catalog.WorkspaceSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	root := s.registry.RootDir()
	cmd := exec.Command("git", "ls-tree", "-r", "--name-only", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	names := strings.Split(strings.TrimSpace(string(out)), "\n")
	has := map[string]bool{}
	for _, name := range names {
		if strings.Contains(name, "/") {
			t.Fatalf("registry files must be flat, got %s", name)
		}
		if !strings.HasSuffix(name, ".yaml") {
			t.Fatalf("registry files must be yaml, got %s", name)
		}
		has[name] = true
	}
	if !has["catalog.yaml"] || !has["workspace-duty.yaml"] || !has["repository-kr_acme_public_core.yaml"] {
		t.Fatal(names)
	}
	id, err := catalog.PeekID(root)
	if err != nil || id != "kr://acme/catalog" {
		t.Fatalf("PeekID %s %v", id, err)
	}
	body, err := os.ReadFile(root + "/workspace-duty.yaml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.HasPrefix(text, "---") || strings.Contains(text, "object_id:") {
		t.Fatalf("must be plain yaml, not a knowledge file:\n%s", text)
	}
	if !strings.Contains(text, "workspaceId: duty") {
		t.Fatal(text)
	}
	hist := s.catalog.Log(catalog.CatalogLogQuery{Limit: 20, Workspace: "duty"})
	if len(hist.Commits) == 0 {
		t.Fatal(hist)
	}
}

func TestOpenExistingRegistryDoesNotRewriteConfigConcurrently(t *testing.T) {
	root := t.TempDir()
	const id = "kr://acme/catalog"
	if _, err := catalog.NewRegistry(root, id); err != nil {
		t.Fatal(err)
	}
	const readers = 24
	var wg sync.WaitGroup
	errs := make(chan error, readers)
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			registry, err := catalog.NewRegistry(root, id)
			if err == nil {
				_, err = registry.Load()
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent registry open must not contend on .git/config: %v", err)
		}
	}
}
