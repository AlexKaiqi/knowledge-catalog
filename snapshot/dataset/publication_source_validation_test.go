package dataset_test

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

type publicationSource struct {
	*catalogMem
	directories []snapshot.DirectoryRequest
	reads       []string
}

func (s *publicationSource) ReadFile(file string, commit kernel.CommitID) ([]byte, error) {
	s.reads = append(s.reads, file)
	return s.catalogMem.ReadFile(file, commit)
}

func (s *publicationSource) ListFiles(kernel.CommitID) ([]string, error) {
	return nil, kernel.Fail(kernel.ErrPreconditionFailed, "publication must not scan a source tree")
}

func (s *publicationSource) ReadDirectory(request snapshot.DirectoryRequest) (snapshot.DirectoryPage, error) {
	s.directories = append(s.directories, request)
	if request.Limit != 1 || request.Continuation != "" {
		return snapshot.DirectoryPage{}, kernel.Fail(kernel.ErrPreconditionFailed, "conflict probe must be one bounded directory page")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prefix := strings.Trim(request.Directory, "/") + "/"
	names := map[string]string{}
	for file := range s.commits[request.Commit] {
		if !strings.HasPrefix(file, prefix) {
			continue
		}
		relative := strings.TrimPrefix(file, prefix)
		name, _, nested := strings.Cut(relative, "/")
		kind := "file"
		if nested {
			kind = "directory"
		}
		names[name] = kind
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	page := snapshot.DirectoryPage{Generation: string(request.Commit), Exhausted: len(ordered) <= request.Limit}
	for _, name := range ordered[:min(len(ordered), request.Limit)] {
		page.Entries = append(page.Entries, snapshot.DirectoryEntry{Name: name, Kind: names[name]})
	}
	if !page.Exhausted {
		page.Continuation = "more"
	}
	return page, nil
}

func writePublicationFiles(t *testing.T, store *catalogMem, files map[string]string) kernel.CommitID {
	t.Helper()
	head, _ := store.Head(snapshot.DefaultRef)
	changes := make([]snapshot.TreeChange, 0, len(files))
	for file, body := range files {
		changes = append(changes, snapshot.TreeChange{Path: file, Content: []byte(body)})
	}
	commit, err := store.ApplyTreeCommit(snapshot.TreeChangeSet{TargetRepository: store.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head, Changes: changes})
	if err != nil {
		t.Fatal(err)
	}
	return commit
}

func publicationService(t *testing.T, members ...snapshot.Store) (catalog.Service, catalog.Registry) {
	t.Helper()
	sources := snapshot.NewRegistry()
	_, registry := createCatalogRegistry(t, "kr://publication/authority")
	service, err := catalog.NewService(sources, registry)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range members {
		if err := sources.Add(source); err != nil {
			t.Fatal(err)
		}
		if err := service.RegisterRepository(source.ID()); err != nil {
			t.Fatal(err)
		}
	}
	return service, registry
}

func TestDatasetPublicationRejectsActualSourceTargetConflicts(t *testing.T) {
	for _, test := range []struct{ name, sourcePath, target string }{
		{"same file", "docs/report.txt", "bundle/report.txt"},
		{"source ancestor file", "docs/guide", "bundle/guide/page.txt"},
		{"source descendant file", "docs/report.txt/child.txt", "bundle/report.txt"},
	} {
		t.Run(test.name, func(t *testing.T) {
			prefix := &publicationSource{catalogMem: newCatalogMem("kr://publication/prefix")}
			file := newCatalogMem("kr://publication/file")
			writePublicationFiles(t, prefix.catalogMem, map[string]string{test.sourcePath: "prefix bytes", "docs/old.txt": "old bytes"})
			writePublicationFiles(t, file, map[string]string{"selected.txt": "file bytes"})
			service, registry := publicationService(t, prefix, file)
			sources := []catalog.DatasetSource{{Repository: prefix.ID(), Selector: snapshot.DefaultRef, Path: catalog.MountPath("bundle"), SubPath: "docs"}}
			accepted, err := service.DefineDataset("delivery", 1, sources)
			if err != nil {
				t.Fatal(err)
			}
			head, _ := registry.Head()
			before := catalog.NormalizeState(service.DumpState())
			sources = append(sources, catalog.DatasetSource{Repository: file.ID(), Selector: snapshot.DefaultRef, File: "selected.txt", Target: test.target})
			for _, publish := range []func() error{
				func() error { _, err := service.DefineDataset("delivery", 2, sources); return err },
				func() error {
					sources[0].Commit, _ = prefix.Head(snapshot.DefaultRef)
					sources[1].Commit, _ = file.Head(snapshot.DefaultRef)
					_, err := service.PublishDataset(catalog.Dataset{SetID: "delivery", Revision: 2, Sources: sources})
					return err
				},
			} {
				if err := publish(); kernel.CodeOf(err) != kernel.ErrDatasetInvalid {
					t.Errorf("actual target conflict was accepted: %v", err)
				}
				if got, _ := registry.Head(); got != head || !reflect.DeepEqual(before, catalog.NormalizeState(service.DumpState())) {
					t.Fatal("failed publication changed HEAD/current/release history")
				}
			}
			if release, err := service.DatasetVersion("delivery", 1); err != nil || !reflect.DeepEqual(release, accepted) {
				t.Fatalf("prior accepted release changed: %#v %v", release, err)
			}
		})
	}
}

func TestDatasetPublicationRejectsMissingSelectedFileAndCheckFails(t *testing.T) {
	source := newCatalogMem("kr://publication/source")
	commit := writePublicationFiles(t, source, map[string]string{"present.txt": "accepted"})
	service, registry := publicationService(t, source)
	sources := []catalog.DatasetSource{{Repository: source.ID(), Selector: snapshot.DefaultRef, File: "present.txt", Target: "selected.txt"}}
	if _, err := service.DefineDataset("delivery", 1, sources); err != nil {
		t.Fatal(err)
	}
	head, _ := registry.Head()
	before := catalog.NormalizeState(service.DumpState())
	sources[0].File, sources[0].Commit = "missing.txt", commit
	for _, publish := range []func() error{
		func() error { _, err := service.DefineDataset("delivery", 2, sources); return err },
		func() error {
			_, err := service.PublishDataset(catalog.Dataset{SetID: "delivery", Revision: 2, Sources: sources})
			return err
		},
	} {
		if err := publish(); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
			t.Errorf("missing selected file was accepted: %v", err)
		}
		if got, _ := registry.Head(); got != head || !reflect.DeepEqual(before, catalog.NormalizeState(service.DumpState())) {
			t.Fatal("missing-file rejection changed HEAD/current/release history")
		}
	}
	pin := catalog.ResolvedDataset{SetID: "bad", Repositories: map[kernel.RepositoryID]kernel.CommitID{source.ID(): commit}, Items: []catalog.DatasetItem{{Kind: catalog.DatasetItemFile, Repository: source.ID(), Commit: commit, File: "missing.txt", Target: "selected.txt"}}}
	if check := service.CheckResolved(pin); check.Outcome != "FAILED" || len(check.Issues) != 1 || check.Issues[0].Code != kernel.ErrObjectRefUnresolved {
		t.Fatalf("nonexistent selected file passed consistency check: %#v", check)
	}
}

func TestDatasetRootFallbackRejectsFilesAtDeclaredMountDirectories(t *testing.T) {
	for _, collision := range []string{"docs/guide/private", "docs/guide"} {
		t.Run(collision, func(t *testing.T) {
			root := &publicationSource{catalogMem: newCatalogMem("kr://publication/root")}
			selected := &publicationSource{catalogMem: newCatalogMem("kr://publication/selected")}
			writePublicationFiles(t, root.catalogMem, map[string]string{collision: "colliding bytes", "docs/keep.txt": "fallback sibling"})
			writePublicationFiles(t, selected.catalogMem, map[string]string{"private/selected.txt": "selected bytes"})
			service, registry := publicationService(t, root, selected)
			sources := []catalog.DatasetSource{{Repository: root.ID(), Selector: snapshot.DefaultRef, Path: catalog.MountPath(""), SubPath: "docs"}}
			accepted, err := service.DefineDataset("delivery", 1, sources)
			if err != nil {
				t.Fatal(err)
			}
			head, _ := registry.Head()
			before := catalog.NormalizeState(service.DumpState())
			history, err := service.Log(catalog.LogQuery{Limit: 20})
			if err != nil {
				t.Fatal(err)
			}
			sources = append(sources, catalog.DatasetSource{Repository: selected.ID(), Selector: snapshot.DefaultRef, Path: catalog.MountPath("guide/private"), SubPath: "private"})
			for _, publish := range []func() error{
				func() error { _, err := service.DefineDataset("delivery", 2, sources); return err },
				func() error {
					sources[0].Commit, _ = root.Head(snapshot.DefaultRef)
					sources[1].Commit, _ = selected.Head(snapshot.DefaultRef)
					_, err := service.PublishDataset(catalog.Dataset{SetID: "delivery", Revision: 2, Sources: sources})
					return err
				},
			} {
				if err := publish(); kernel.CodeOf(err) != kernel.ErrDatasetInvalid {
					t.Errorf("root fallback file was accepted at a declared directory: %v", err)
				}
				afterHistory, historyErr := service.Log(catalog.LogQuery{Limit: 20})
				if got, _ := registry.Head(); got != head || !reflect.DeepEqual(before, catalog.NormalizeState(service.DumpState())) || historyErr != nil || !reflect.DeepEqual(history, afterHistory) {
					t.Fatal("directory conflict changed HEAD/current/release history")
				}
			}
			for _, probe := range root.reads {
				if probe != "docs/guide/private" && probe != "docs/guide" && probe != "docs" {
					t.Fatalf("directory collision scanned unrelated root path %s", probe)
				}
			}
			if len(root.directories) != 0 || len(selected.reads) != 0 || len(selected.directories) != 0 {
				t.Fatal("directory collision enumerated or read the selected prefix")
			}
			if old, err := service.DatasetVersion("delivery", 1); err != nil || !reflect.DeepEqual(old, accepted) {
				t.Fatalf("directory conflict changed prior release: %#v %v", old, err)
			}
			if old, err := service.ReadVirtualFile("delivery", "keep.txt"); err != nil || string(old.Content) != "fallback sibling" {
				t.Fatalf("directory conflict damaged root sibling consumption: %#v %v", old, err)
			}
		})
	}
}

func TestDatasetRootFallbackDirectoryProbesPreserveValidLayoutsAndRequireCapability(t *testing.T) {
	for _, capable := range []bool{true, false} {
		t.Run(map[bool]string{true: "valid root fallback", false: "missing reader"}[capable], func(t *testing.T) {
			root := &publicationSource{catalogMem: newCatalogMem("kr://publication/root")}
			selected := newCatalogMem("kr://publication/selected")
			writePublicationFiles(t, root.catalogMem, map[string]string{"docs/keep.txt": "fallback sibling", "docs/guide/private/hidden.txt": "excluded subtree"})
			writePublicationFiles(t, selected, map[string]string{"private/selected.txt": "selected bytes"})
			var source snapshot.Store = root
			if !capable {
				source = struct{ snapshot.Store }{Store: root}
			}
			service, registry := publicationService(t, source, selected)
			head, _ := registry.Head()
			before := catalog.NormalizeState(service.DumpState())
			_, err := service.DefineDataset("delivery", 1, []catalog.DatasetSource{
				{Repository: root.ID(), Selector: snapshot.DefaultRef, Path: catalog.MountPath(""), SubPath: "docs"},
				{Repository: selected.ID(), Selector: snapshot.DefaultRef, Path: catalog.MountPath("guide/private"), SubPath: "private"},
			})
			if !capable {
				if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
					t.Fatalf("unprovable root directory was accepted: %v", err)
				}
				if got, _ := registry.Head(); got != head || !reflect.DeepEqual(before, catalog.NormalizeState(service.DumpState())) {
					t.Fatal("missing root reader changed authority")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				for path, want := range map[string]string{"keep.txt": "fallback sibling", "guide/private/selected.txt": "selected bytes"} {
					got, err := service.ReadVirtualFile("delivery", path)
					if err != nil || string(got.Content) != want {
						t.Fatalf("valid root fallback %s: %#v %v", path, got, err)
					}
				}
			}
		})
	}
}

func TestSnapshotRegistryRejectsErasedAuthorityWithoutReinitializing(t *testing.T) {
	authority, registry := createCatalogRegistry(t, "kr://publication/erased")
	service, err := catalog.NewService(snapshot.NewRegistry(), registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RegisterRepository("kr://publication/member"); err != nil {
		t.Fatal(err)
	}
	head, _ := authority.Head(snapshot.DefaultRef)
	erased, err := authority.ApplyTreeCommit(snapshot.TreeChangeSet{TargetRepository: authority.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head, Changes: []snapshot.TreeChange{{Path: catalog.RegistryFile(), Remove: true}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, open := range []func(snapshot.Store, string) (catalog.Registry, error){catalog.OpenSnapshotRegistry, catalog.CreateSnapshotRegistry} {
		if _, err := open(authority, ""); kernel.CodeOf(err) != kernel.ErrDatasetInvalid {
			t.Errorf("erased accepted authority was treated as new: %v", err)
		}
		if got, _ := authority.Head(snapshot.DefaultRef); got != erased {
			t.Fatal("rejected recovery reinitialized authority HEAD")
		}
	}
	if _, err := authority.ReadFile(catalog.RegistryFile(), head); err != nil {
		t.Fatal("old accepted authority was changed", err)
	}
}
