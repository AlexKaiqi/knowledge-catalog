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
	if _, err := s.catalog.DefineKnowledgeSet("duty", 1, []catalog.KnowledgeSetSource{
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
	if !has["catalog.yaml"] || !has["dataset-duty.yaml"] || !has["repository-kr_acme_public_core.yaml"] {
		t.Fatal(names)
	}
	id, err := catalog.PeekID(root)
	if err != nil || id != "kr://acme/catalog" {
		t.Fatalf("PeekID %s %v", id, err)
	}
	body, err := os.ReadFile(root + "/dataset-duty.yaml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.HasPrefix(text, "---") || strings.Contains(text, "object_id:") {
		t.Fatalf("must be plain yaml, not a knowledge file:\n%s", text)
	}
	if !strings.Contains(text, "setId: duty") {
		t.Fatal(text)
	}
	hist := s.catalog.Log(catalog.CatalogLogQuery{Limit: 20, Dataset: "duty"})
	if len(hist.Commits) == 0 {
		t.Fatal(hist)
	}
}

func TestRegistryIgnoresRetiredKsetPrefix(t *testing.T) {
	s := setupFed(t)
	if _, err := s.catalog.DefineKnowledgeSet("duty", 1, []catalog.KnowledgeSetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	root := s.registry.RootDir()
	if err := os.Rename(root+"/dataset-duty.yaml", root+"/kset-duty.yaml"); err != nil {
		t.Fatal(err)
	}
	add := exec.Command("git", "add", "-A")
	add.Dir = root
	if out, err := add.CombinedOutput(); err != nil {
		t.Fatalf("git add: %s %v", out, err)
	}
	commit := exec.Command("git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", "retired kset prefix")
	commit.Dir = root
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %s %v", out, err)
	}
	reopened, err := catalog.NewRegistry(root, "kr://acme/catalog")
	if err != nil {
		t.Fatal(err)
	}
	state, err := reopened.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, set := range state.KnowledgeSets {
		if set.SetID == "duty" {
			t.Fatalf("retired kset- registry files must not load: %#v", state.KnowledgeSets)
		}
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
