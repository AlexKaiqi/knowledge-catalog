package dataset_test

import (
	"strings"
	"sync"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

func TestRegistryWritesFlatYAML(t *testing.T) {
	s := setupFed(t)
	if _, err := s.catalog.DefineKnowledgeSet("duty", 1, []catalog.KnowledgeSetSource{
		{Repository: "kr://acme/public/core", Selector: snapshot.DefaultRef},
	}); err != nil {
		t.Fatal(err)
	}
	head, err := s.registry.Head()
	if err != nil {
		t.Fatal(err)
	}
	names, err := s.authority.ListFiles(kernel.CommitID(head))
	if err != nil {
		t.Fatal(err)
	}
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
	if !has["registry.yaml"] || !has["dataset-duty.yaml"] {
		t.Fatal(names)
	}
	for name := range has {
		if strings.HasPrefix(name, "repository-") {
			t.Fatalf("membership must live in registry.yaml, got %s", name)
		}
	}
	state, err := s.registry.Load()
	if err != nil || state.AuthorityID != "kr://acme/catalog" {
		t.Fatalf("catalog identity %s %v", state.AuthorityID, err)
	}
	body, err := s.authority.ReadFile("dataset-duty.yaml", kernel.CommitID(head))
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
	hist := s.catalog.Log(catalog.LogQuery{Limit: 20, Dataset: "duty"})
	if len(hist.Commits) == 0 {
		t.Fatal(hist)
	}
}

func TestRegistryIgnoresRetiredKsetPrefix(t *testing.T) {
	s := setupFed(t)
	if _, err := s.catalog.DefineKnowledgeSet("duty", 1, []catalog.KnowledgeSetSource{
		{Repository: "kr://acme/public/core", Selector: snapshot.DefaultRef},
	}); err != nil {
		t.Fatal(err)
	}
	head, err := s.authority.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	body, err := s.authority.ReadFile("dataset-duty.yaml", head)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.authority.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: s.authority.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: head, ExpectedTargetCommit: head,
		Changes: []snapshot.TreeChange{{Path: "dataset-duty.yaml", Remove: true}, {Path: "kset-duty.yaml", Content: body}},
		Message: "retired kset prefix",
	}); err != nil {
		t.Fatal(err)
	}
	reopened, err := catalog.OpenSnapshotRegistry(s.authority, "")
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

func TestOpenExistingRegistryDoesNotWriteConcurrently(t *testing.T) {
	const id = "kr://acme/catalog"
	authority, created := createCatalogRegistry(t, id)
	before, err := created.Head()
	if err != nil {
		t.Fatal(err)
	}
	const readers = 24
	var wg sync.WaitGroup
	errs := make(chan error, readers)
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			registry, err := catalog.OpenSnapshotRegistry(authority, "")
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
			t.Fatalf("concurrent read-only registry open: %v", err)
		}
	}
	after, err := authority.Head(snapshot.DefaultRef)
	if err != nil || string(after) != before {
		t.Fatalf("concurrent registry open rewrote authority: %s %v", after, err)
	}
}
