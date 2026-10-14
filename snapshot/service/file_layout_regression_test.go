package service_test

import (
	"reflect"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
	"kc/snapshot/service"
)

func legacyLayoutView(t *testing.T, mountPath string, files map[string][]byte, target string, visible bool) service.FileView {
	t.Helper()
	fixture, source := newFileHTTPFixture(t)
	source.files["c1"] = files
	prefix := dataset.DatasetItem{Kind: dataset.DatasetItemPrefix, Target: mountPath, Prefix: "reference", Repository: source.id, Commit: "c1"}
	file := dataset.DatasetItem{Kind: dataset.DatasetItemFile, Target: target, File: "chosen.txt", Repository: source.id, Commit: "c1"}
	items := []dataset.DatasetItem{}
	if visible {
		items = append(items, file)
	}
	pin := dataset.ResolvedDataset{SetID: "legacy", Revision: 1, PinID: "trusted-old", Items: []dataset.DatasetItem{prefix, file}, Repositories: map[kernel.RepositoryID]kernel.CommitID{source.id: "c1"}}
	return service.NewFileView(service.ViewOptions{Pin: pin, Mounts: []dataset.VirtualMount{{Path: mountPath, SubPath: "reference", Repository: source.id, Commit: "c1"}}, Items: items, Sources: fixture.resources.Sources, Delivered: true})
}

func TestSnapshotLegacyConflictsRejectEveryFileAddress(t *testing.T) {
	for _, test := range []struct{ name, sourcePath, target string }{
		{"same file", "reference/a.txt", "docs/a.txt"},
		{"source ancestor", "reference/guide", "docs/guide/a.txt"},
		{"source descendant", "reference/a.txt/child.txt", "docs/a.txt"},
	} {
		t.Run(test.name, func(t *testing.T) {
			view := legacyLayoutView(t, "docs", map[string][]byte{test.sourcePath: []byte("prefix source"), "chosen.txt": []byte("file source")}, test.target, true)
			for _, request := range []service.FileReadRequest{{Path: test.target}, {MountPath: "docs", File: test.target[len("docs/"):]}} {
				if got, err := view.Read(request); kernel.CodeOf(err) != kernel.ErrDatasetInvalid || len(got.Content) != 0 {
					t.Errorf("conflicting old pin delivered bytes through %#v: %#v %v", request, got, err)
				}
			}
			for _, request := range []service.FileDirectoryRequest{{Path: "docs"}, {MountPath: "docs"}, {Path: test.target}, {MountPath: "docs", Directory: test.target[len("docs/"):]}} {
				if _, err := view.List(request); kernel.CodeOf(err) != kernel.ErrDatasetInvalid {
					t.Errorf("conflicting old pin listed through %#v: %v", request, err)
				}
			}
		})
	}
}

func TestSnapshotFileDirectoryRejectsMixedCoordinates(t *testing.T) {
	view := legacyLayoutView(t, "docs", map[string][]byte{"reference/a.txt": []byte("prefix source"), "chosen.txt": []byte("selected")}, "outside.txt", true)
	if _, err := view.List(service.FileDirectoryRequest{MountPath: "docs", Path: "ignored"}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatal("mixed delivered and mount-relative directories were accepted", err)
	}
}

func TestSnapshotRootFileOverrideIsSharedByEveryAddressAndList(t *testing.T) {
	for _, visible := range []bool{true, false} {
		view := legacyLayoutView(t, "", map[string][]byte{"reference/shared.txt": []byte("hidden root bytes"), "reference/keep.txt": []byte("fallback bytes"), "chosen.txt": []byte("selected bytes")}, "shared.txt", visible)
		for _, request := range []service.FileReadRequest{{Path: "shared.txt"}, {File: "shared.txt"}} {
			got, err := view.Read(request)
			if visible {
				if err != nil || string(got.Content) != "selected bytes" {
					t.Errorf("root override diverged through %#v: %#v %v", request, got, err)
				}
			} else if kernel.CodeOf(err) != kernel.ErrForbidden || len(got.Content) != 0 {
				t.Errorf("hidden file fell back to excluded root bytes through %#v: %#v %v", request, got, err)
			}
		}
		page, err := view.List(service.FileDirectoryRequest{})
		want := []snapshot.DirectoryEntry{{Name: "keep.txt", Kind: "file"}}
		if visible {
			want = []snapshot.DirectoryEntry{{Name: "shared.txt", Kind: "file"}, {Name: "keep.txt", Kind: "file"}}
		}
		if err != nil || !page.Exhausted || !reflect.DeepEqual(page.Entries, want) {
			t.Errorf("root listing lost visible selection/exclusion: %#v %v want %#v", page, err, want)
		}
	}
}

func TestSnapshotHiddenMountContinuesToExcludeRootFallback(t *testing.T) {
	fixture, root := newFileHTTPFixture(t)
	root.files["c1"] = map[string][]byte{"reference/private/secret.txt": []byte("excluded fallback"), "reference/keep.txt": []byte("visible fallback")}
	items := []dataset.DatasetItem{
		{Kind: dataset.DatasetItemPrefix, Repository: root.id, Commit: "c1", Prefix: "reference"},
		{Kind: dataset.DatasetItemPrefix, Repository: "kr://hidden/source", Commit: "c1", Target: "private", Prefix: "docs"},
	}
	view := service.NewFileView(service.ViewOptions{Pin: dataset.ResolvedDataset{Items: items}, Mounts: []dataset.VirtualMount{{Repository: root.id, Commit: "c1", SubPath: "reference"}}, Sources: fixture.resources.Sources, Delivered: true})
	for _, request := range []service.FileReadRequest{{Path: "private/secret.txt"}, {File: "private/secret.txt"}, {MountPath: "private", File: "secret.txt"}} {
		if got, err := view.Read(request); kernel.CodeOf(err) != kernel.ErrForbidden || len(got.Content) != 0 {
			t.Fatalf("hidden mount reopened fallback bytes: %#v %v", got, err)
		}
	}
	page, err := view.List(service.FileDirectoryRequest{})
	if err != nil || !reflect.DeepEqual(page.Entries, []snapshot.DirectoryEntry{{Name: "keep.txt", Kind: "file"}}) {
		t.Fatalf("hidden mount exposed fallback directory: %#v %v", page, err)
	}
	for _, request := range []service.FileDirectoryRequest{{Path: "private"}, {Directory: "private"}} {
		page, err := view.List(request)
		if err != nil || len(page.Entries) != 0 {
			t.Fatalf("hidden mount descendant listed fallback children: %#v %v", page, err)
		}
	}
	if root.reads != 0 || root.scans != 0 {
		t.Fatal("hidden routes reached source bytes or a tree scan")
	}
}

func TestSnapshotLiteralAliasRejectsTraversalBeforeSourceRead(t *testing.T) {
	view := legacyLayoutView(t, "docs", map[string][]byte{"reference/a.txt": []byte("selected")}, "outside.txt", false)
	for _, relative := range []string{"../a.txt", "scratch/../../a.txt"} {
		if _, err := view.Read(service.FileReadRequest{MountPath: "docs", File: relative}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatal("mount-relative file escaped during delivered conversion", err)
		}
		if _, err := view.List(service.FileDirectoryRequest{MountPath: "docs", Directory: relative}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatal("mount-relative directory escaped during delivered conversion", err)
		}
	}
}

func TestSnapshotSemanticViewPreservesItsReadAndDirectoryPorts(t *testing.T) {
	reads, directories := 0, 0
	mount := dataset.VirtualMount{Path: "semantic", Repository: "repo", Commit: "c1", SubPath: "literal-prefix"}
	view := service.NewFileView(service.ViewOptions{Pin: dataset.ResolvedDataset{Items: []dataset.DatasetItem{{Kind: dataset.DatasetItemFile, Target: "semantic/a", File: "literal.txt"}}}, Mounts: []dataset.VirtualMount{mount}, Read: func(got dataset.VirtualMount, file string) ([]byte, error) {
		reads++
		if got != mount || file != "entity/aspect.json" {
			t.Fatalf("semantic read was translated into literal coordinates: %#v %s", got, file)
		}
		return []byte("derived bytes"), nil
	}, Directory: func(got dataset.VirtualMount, directory string, limit int, continuation string) (snapshot.DirectoryPage, error) {
		directories++
		if got != mount || directory != "entity" || continuation != "semantic-position" || limit != 3 {
			t.Fatal("semantic directory lost its own coordinates or cursor")
		}
		return snapshot.DirectoryPage{Entries: []snapshot.DirectoryEntry{{Name: "aspect.json", Kind: "file"}}, Generation: "c1", Exhausted: true}, nil
	}})
	read, err := view.Read(service.FileReadRequest{MountPath: "semantic", File: "entity/aspect.json"})
	if err != nil || string(read.Content) != "derived bytes" || reads != 1 {
		t.Fatalf("semantic read fell back to literal source: %#v %v", read, err)
	}
	page, err := view.List(service.FileDirectoryRequest{MountPath: "semantic", Directory: "entity", Limit: 3, Continuation: "semantic-position"})
	if err != nil || directories != 1 || len(page.Entries) != 1 {
		t.Fatalf("semantic directory fell back to literal source: %#v %v", page, err)
	}
}

func TestSnapshotVisibleFileNeverProbesHiddenPrefixForConflicts(t *testing.T) {
	fixture, hidden := newFileHTTPFixture(t)
	item := dataset.DatasetItem{Kind: dataset.DatasetItemFile, Target: "docs/a.json", Repository: "kr://visible/source", Commit: "c1", File: "chosen.txt"}
	prefix := dataset.DatasetItem{Kind: dataset.DatasetItemPrefix, Target: "docs", Repository: hidden.id, Commit: "c1", Prefix: "reference"}
	view := service.NewFileView(service.ViewOptions{Pin: dataset.ResolvedDataset{Items: []dataset.DatasetItem{prefix, item}}, Items: []dataset.DatasetItem{item}, Sources: fixture.resources.Sources, Delivered: true})
	if _, err := view.Read(service.FileReadRequest{Path: item.Target}); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatal("unavailable authorized collision basis was borrowed from hidden source", err)
	}
	if hidden.reads != 0 || hidden.scans != 0 {
		t.Fatal("conflict validation read hidden source content")
	}
}

func TestSnapshotTrustedProgrammaticViewReconstructsCanonicalScope(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	view := service.NewFileView(service.ViewOptions{Pin: dataset.ResolvedDataset{SetID: "old-view"}, Mounts: []dataset.VirtualMount{{Path: "docs", Repository: source.id, Commit: "c1", SubPath: "reference/"}}, Sources: fixture.resources.Sources, Delivered: true})
	read, err := view.Read(service.FileReadRequest{MountPath: "docs", File: "a.json"})
	if err != nil || string(read.Content) != string(source.files["c1"]["reference/a.json"]) {
		t.Fatalf("trusted legacy options could not reconstruct their selected subtree: %#v %v", read, err)
	}
}

func TestSnapshotDeclaredAncestorsNeverBecomeFallbackFiles(t *testing.T) {
	for _, kind := range []string{dataset.DatasetItemFile, dataset.DatasetItemPrefix} {
		for _, visible := range []bool{false, true} {
			t.Run(kind+map[bool]string{false: "/hidden", true: "/visible"}[visible], func(t *testing.T) {
				fixture, root := newFileHTTPFixture(t)
				root.files["c1"] = map[string][]byte{"reference/outer/guide": []byte("excluded ancestor bytes"), "reference/outer/keep.txt": []byte("fallback sibling")}
				prefix := dataset.DatasetItem{Kind: dataset.DatasetItemPrefix, Prefix: "reference", Repository: root.id, Commit: "c1"}
				selected := dataset.DatasetItem{Kind: kind, Target: "outer/guide/private", Prefix: "docs", File: "chosen.txt", Repository: "kr://selected/source", Commit: "c1"}
				options := service.ViewOptions{Pin: dataset.ResolvedDataset{Items: []dataset.DatasetItem{prefix, selected}}, Mounts: []dataset.VirtualMount{{SubPath: "reference", Repository: root.id, Commit: "c1"}}, Sources: fixture.resources.Sources, Delivered: true}
				if visible {
					if kind == dataset.DatasetItemFile {
						options.Items = []dataset.DatasetItem{selected}
					} else {
						options.Mounts = append(options.Mounts, dataset.VirtualMount{Path: selected.Target, SubPath: selected.Prefix, Repository: selected.Repository, Commit: selected.Commit})
					}
				}
				view := service.NewFileView(options)
				wantRead := kernel.ErrForbidden
				if visible {
					wantRead = kernel.ErrObjectRefUnresolved
					if kind == dataset.DatasetItemFile {
						wantRead = kernel.ErrDatasetInvalid
					}
				}
				for _, request := range []service.FileReadRequest{{Path: "outer/guide"}, {File: "outer/guide"}} {
					if got, err := view.Read(request); kernel.CodeOf(err) != wantRead || len(got.Content) != 0 {
						t.Errorf("declared directory returned fallback file through %#v: %#v %v", request, got, err)
					}
				}
				for _, request := range []service.FileDirectoryRequest{{Path: "outer"}, {Directory: "outer"}} {
					got, err := view.List(request)
					if visible {
						if kernel.CodeOf(err) != kernel.ErrDatasetInvalid || len(got.Entries) != 0 {
							t.Errorf("visible declared directory conflict was listed: %#v %v", got, err)
						}
					} else if err != nil || !reflect.DeepEqual(got.Entries, []snapshot.DirectoryEntry{{Name: "keep.txt", Kind: "file"}}) {
						t.Errorf("hidden selection reopened excluded ancestor: %#v %v", got, err)
					}
				}
				if !visible && (root.reads != 0 || root.scans != 0) {
					t.Fatal("hidden directory selection reached source bytes or a tree scan")
				}
				if !visible {
					root.files["c1"] = map[string][]byte{"reference/outer/guide/keep.txt": []byte("fallback sibling"), "reference/outer/guide/private/hidden.txt": []byte("excluded subtree")}
					for _, request := range []service.FileReadRequest{{Path: "outer/guide/keep.txt"}, {File: "outer/guide/keep.txt"}} {
						if got, err := view.Read(request); err != nil || string(got.Content) != "fallback sibling" {
							t.Errorf("hidden descendant removed an authorized root sibling: %#v %v", got, err)
						}
					}
					for _, request := range []service.FileDirectoryRequest{{Path: "outer/guide"}, {Directory: "outer/guide"}} {
						if got, err := view.List(request); err != nil || !reflect.DeepEqual(got.Entries, []snapshot.DirectoryEntry{{Name: "keep.txt", Kind: "file"}}) {
							t.Errorf("hidden descendant changed authorized root directory: %#v %v", got, err)
						}
					}
				}
			})
		}
	}
}
