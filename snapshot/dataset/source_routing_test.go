package dataset_test

import (
	"testing"

	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

func TestDatasetRootFallbackAndFileRoutesUseFrozenSources(t *testing.T) {
	root := &publicationSource{catalogMem: newCatalogMem("kr://routing/root")}
	pieces := &publicationSource{catalogMem: newCatalogMem("kr://routing/pieces")}
	writePublicationFiles(t, root.catalogMem, map[string]string{"docs/root.txt": "root overwritten", "docs/unclaimed.txt": "root fallback", "docs/bundle/added.txt": "hidden root"})
	writePublicationFiles(t, pieces.catalogMem, map[string]string{"chosen.txt": "explicit file", "manual/guide.txt": "mounted file"})
	service, _ := publicationService(t, root, pieces)
	def, err := service.DefineKnowledgeSet("mixed", 1, []catalog.KnowledgeSetSource{
		{Repository: root.ID(), Selector: snapshot.DefaultRef, Path: catalog.MountPath(""), SubPath: "docs"},
		{Repository: pieces.ID(), Selector: snapshot.DefaultRef, Path: catalog.MountPath("bundle"), SubPath: "manual"},
		{Repository: pieces.ID(), Selector: snapshot.DefaultRef, File: "chosen.txt", Target: "root.txt"},
		{Repository: pieces.ID(), Selector: snapshot.DefaultRef, File: "chosen.txt", Target: "bundle/added.txt"},
	})
	if err != nil {
		t.Fatal("valid root fallback and same-repository fragments were rejected", err)
	}
	pin, err := service.ResolveKnowledgeSet("mixed")
	if err != nil {
		t.Fatal(err)
	}
	writePublicationFiles(t, pieces.catalogMem, map[string]string{"chosen.txt": "later bytes", "manual/guide.txt": "later mount"})
	for _, test := range []struct {
		delivered, file, body string
		repository            kernel.RepositoryID
	}{
		{"root.txt", "chosen.txt", "explicit file", pieces.ID()},
		{"bundle/added.txt", "chosen.txt", "explicit file", pieces.ID()},
		{"bundle/guide.txt", "manual/guide.txt", "mounted file", pieces.ID()},
		{"unclaimed.txt", "docs/unclaimed.txt", "root fallback", root.ID()},
	} {
		route, err := catalog.RouteDatasetFile(pin.Items, test.delivered)
		if err != nil || route.Repository != test.repository || route.File != test.file || route.Commit != pin.Repositories[test.repository] {
			t.Fatalf("file compiler selected different source for %s: %#v %v", test.delivered, route, err)
		}
		read, err := service.ReadVirtualFileAt(def, pin, test.delivered)
		if err != nil || read.Repository != test.repository || string(read.Content) != test.body || read.Commit != route.Commit {
			t.Fatalf("legacy virtual read diverged from fixed file route for %s: %#v %v", test.delivered, read, err)
		}
	}
}

func TestDatasetRootFileOverrideRejectsSourceFileDirectoryConflicts(t *testing.T) {
	for _, test := range []struct{ source, target string }{
		{"docs/guide", "guide/page.txt"},
		{"docs/guide/child.txt", "guide"},
	} {
		root := &publicationSource{catalogMem: newCatalogMem("kr://routing/root")}
		file := newCatalogMem("kr://routing/file")
		writePublicationFiles(t, root.catalogMem, map[string]string{test.source: "root source"})
		writePublicationFiles(t, file, map[string]string{"chosen.txt": "selected"})
		service, registry := publicationService(t, root, file)
		head, _ := registry.Head()
		_, err := service.DefineKnowledgeSet("bad", 1, []catalog.KnowledgeSetSource{
			{Repository: root.ID(), Selector: snapshot.DefaultRef, Path: catalog.MountPath(""), SubPath: "docs"},
			{Repository: file.ID(), Selector: snapshot.DefaultRef, File: "chosen.txt", Target: test.target},
		})
		if kernel.CodeOf(err) != kernel.ErrKnowledgeSetInvalid {
			t.Fatalf("root file/directory conflict was accepted: %v", err)
		}
		if after, _ := registry.Head(); after != head {
			t.Fatal("invalid root override advanced publication authority")
		}
	}
}

func TestDatasetFileRouteRejectsEscapesAndUnselectedPaths(t *testing.T) {
	items := []catalog.DatasetItem{{Kind: catalog.DatasetItemFile, Target: "renamed.txt", File: "source.txt", Repository: "repo", Commit: "frozen"}}
	for _, input := range []string{"", "..", "../source.txt", "a/../../source.txt", "bad\\path", "bad\x00path"} {
		if _, err := catalog.RouteDatasetFile(items, input); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("unsafe delivered path was compiled: %q %v", input, err)
		}
	}
	if _, err := catalog.RouteDatasetFile(items, "source.txt"); kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
		t.Fatal("original source path was admitted into delivered scope", err)
	}
}

func TestDatasetMixedPublicationFailsClosedWithoutDirectoryCapability(t *testing.T) {
	prefix := newCatalogMem("kr://routing/prefix")
	file := newCatalogMem("kr://routing/file")
	writePublicationFiles(t, prefix, map[string]string{"docs/old.txt": "source"})
	writePublicationFiles(t, file, map[string]string{"chosen.txt": "selected"})
	service, registry := publicationService(t, prefix, file)
	head, _ := registry.Head()
	_, err := service.DefineKnowledgeSet("mixed", 1, []catalog.KnowledgeSetSource{
		{Repository: prefix.ID(), Selector: snapshot.DefaultRef, Path: catalog.MountPath("bundle"), SubPath: "docs"},
		{Repository: file.ID(), Selector: snapshot.DefaultRef, File: "chosen.txt", Target: "bundle/added.txt"},
	})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("unprovable mixed layout was accepted: %v", err)
	}
	if after, _ := registry.Head(); after != head {
		t.Fatal("missing directory capability advanced publication authority")
	}
}

func TestDatasetVirtualReadRequiresDeclaredDirectoryMapping(t *testing.T) {
	source := newCatalogMem("kr://routing/unmounted")
	writePublicationFiles(t, source, map[string]string{"plain.txt": "source bytes"})
	service, _ := publicationService(t, source)
	if _, err := service.DefineKnowledgeSet("unmounted", 1, []catalog.KnowledgeSetSource{{Repository: source.ID(), Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReadVirtualFile("unmounted", string(source.ID())+"/plain.txt"); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatal("coordinate-only source acquired an implicit delivered mapping", err)
	}
}

func TestSnapshotRegistryCreationRejectsForeignContentAndUnprovableRoot(t *testing.T) {
	foreign := newCatalogMem("kr://routing/foreign")
	writePublicationFiles(t, foreign, map[string]string{"foreign.txt": "unowned content"})
	unprovable := newCatalogMem("kr://routing/unprovable")
	for _, test := range []struct {
		store snapshot.Store
		code  kernel.ErrorCode
	}{
		{foreign, kernel.ErrKnowledgeSetInvalid},
		{plainTreeSnapshot{Store: unprovable, TreeStore: unprovable}, kernel.ErrCapabilityUnsatisfied},
	} {
		before, _ := test.store.Head(snapshot.DefaultRef)
		if _, err := catalog.CreateSnapshotRegistry(test.store, ""); kernel.CodeOf(err) != test.code {
			t.Fatalf("unproved authority was initialized: %v", err)
		}
		if after, _ := test.store.Head(snapshot.DefaultRef); after != before {
			t.Fatal("rejected creation advanced authority HEAD")
		}
	}
}
