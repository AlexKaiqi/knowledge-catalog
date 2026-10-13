package dataset_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

func newMetadataDataset(t *testing.T) (catalog.Service, catalog.Registry, *catalogMem, *catalogMem) {
	t.Helper()
	authority, registry := createCatalogRegistry(t, "kr://metadata/registry")
	source := newCatalogMem("kr://metadata/source")
	head, err := source.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: source.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head,
		Changes: []snapshot.TreeChange{{Path: "visible.txt", Content: []byte("selected")}, {Path: "private.txt", Content: []byte("private")}},
	}); err != nil {
		t.Fatal(err)
	}
	stores := snapshot.NewRegistry()
	if err := stores.Add(source); err != nil {
		t.Fatal(err)
	}
	service, err := catalog.NewService(stores, registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RegisterRepository(source.ID()); err != nil {
		t.Fatal(err)
	}
	return service, registry, authority, source
}

func publishMetadataDataset(t *testing.T, service catalog.Service, revision int, metadata map[string]string) catalog.KnowledgeSet {
	t.Helper()
	def, err := service.PrepareKnowledgeSet("delivery", revision, []catalog.KnowledgeSetSource{{
		Repository: "kr://metadata/source", Selector: snapshot.DefaultRef, File: "visible.txt", Target: "public.txt",
	}})
	if err != nil {
		t.Fatal(err)
	}
	def.Metadata = metadata
	accepted, err := service.PublishKnowledgeSet(def)
	if err != nil {
		t.Fatal(err)
	}
	return accepted
}

func TestDatasetMetadataPublicationRecoveryAndVersionIsolation(t *testing.T) {
	service, registry, authority, source := newMetadataDataset(t)
	old := publishMetadataDataset(t, service, 1, map[string]string{"description": "daily delivery", "custom": "42"})
	view := service.ReadView(nil)
	head, _ := registry.Head()
	paths, err := authority.ListFiles(kernel.CommitID(head))
	if err != nil {
		t.Fatal(err)
	}
	var release string
	for _, path := range paths {
		if path != "registry.yaml" && path != catalog.KnowledgeSetYAML("delivery") {
			release = path
		}
	}
	acceptedBytes, err := authority.ReadFile(release, kernel.CommitID(head))
	if err != nil {
		t.Fatal(err)
	}
	latest := publishMetadataDataset(t, service, 2, map[string]string{"description": "revised delivery", "path": "private.txt"})
	if !reflect.DeepEqual(old.Items, latest.Items) || catalog.DatasetPathAllowed(latest.Items, source.ID(), "private.txt") {
		t.Fatal("metadata changed the published file scope", latest)
	}
	latest.Metadata["description"] = "attempted overwrite"
	if _, err := service.PublishKnowledgeSet(latest); kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("same-revision metadata rewrite accepted: %v", err)
	}
	currentHead, _ := registry.Head()
	stillAccepted, err := authority.ReadFile(release, kernel.CommitID(currentHead))
	if err != nil || !bytes.Equal(acceptedBytes, stillAccepted) {
		t.Fatal("metadata update rewrote an accepted release", err)
	}
	reopened, err := catalog.OpenSnapshotRegistry(authority, "")
	if err != nil {
		t.Fatal(err)
	}
	stores := snapshot.NewRegistry()
	if err := stores.Add(source); err != nil {
		t.Fatal(err)
	}
	recovered, err := catalog.NewService(stores, reopened)
	if err != nil {
		t.Fatal(err)
	}
	for revision, want := range map[int]string{1: "daily delivery", 2: "revised delivery"} {
		def, err := recovered.DatasetVersion("delivery", revision)
		if err != nil || def.Metadata["description"] != want {
			t.Fatalf("revision %d metadata lost after restart: %#v %v", revision, def, err)
		}
		detail, err := catalog.ManagementView(recovered.DumpState(), "delivery", revision, catalog.ManagementOptions{})
		if err != nil || detail["metadata"].(map[string]string)["description"] != want {
			t.Fatalf("history metadata does not use the selected version: %#v %v", detail, err)
		}
	}
	fixed, err := view.Set("delivery")
	if err != nil || fixed.Revision != 1 || fixed.Metadata["description"] != "daily delivery" {
		t.Fatalf("read view metadata moved with publication: %#v %v", fixed, err)
	}
	cleared := publishMetadataDataset(t, recovered, 3, nil)
	if _, present := catalog.PublicKnowledgeSet(cleared)["metadata"]; present {
		t.Fatal("omitted metadata was inherited or synthesized")
	}
}

func TestDatasetMetadataMapsAreIndependent(t *testing.T) {
	service, _, _, _ := newMetadataDataset(t)
	input := map[string]string{"description": "accepted"}
	accepted := publishMetadataDataset(t, service, 1, input)
	input["description"] = "changed input"
	accepted.Metadata["description"] = "changed result"
	readers := map[string]func() catalog.KnowledgeSet{
		"set":              func() catalog.KnowledgeSet { def, _ := service.Set("delivery"); return def },
		"version":          func() catalog.KnowledgeSet { def, _ := service.DatasetVersion("delivery", 1); return def },
		"current state":    func() catalog.KnowledgeSet { return service.DumpState().KnowledgeSets[0] },
		"historical state": func() catalog.KnowledgeSet { return service.DumpState().DatasetVersions[0] },
		"read view":        func() catalog.KnowledgeSet { def, _ := service.ReadView(nil).Set("delivery"); return def },
	}
	for name, read := range readers {
		def := read()
		if def.Metadata["description"] != "accepted" {
			t.Fatalf("%s aliases publication input or result", name)
		}
		def.Metadata["description"] = name
		if read().Metadata["description"] != "accepted" {
			t.Fatalf("%s exposes an accepted map", name)
		}
	}
	state := service.DumpState()
	normalized := catalog.NormalizeState(state)
	normalized.KnowledgeSets[0].Metadata["description"] = "normalized current"
	normalized.DatasetVersions[0].Metadata["description"] = "normalized history"
	service.LoadState(state)
	state.KnowledgeSets[0].Metadata["description"] = "load input"
	state.DatasetVersions[0].Metadata["description"] = "load history"
	def, _ := service.Set("delivery")
	historical, _ := service.DatasetVersion("delivery", 1)
	if def.Metadata["description"] != "accepted" || historical.Metadata["description"] != "accepted" {
		t.Fatal("NormalizeState or LoadState shared a metadata map")
	}
	for _, public := range []map[string]any{catalog.PublicKnowledgeSet(def), catalog.PublicKnowledgeSets([]catalog.KnowledgeSet{def})[0]} {
		public["metadata"].(map[string]string)["description"] = "summary"
	}
	detail, err := catalog.ManagementView(catalog.State{KnowledgeSets: []catalog.KnowledgeSet{def}}, "delivery", 0, catalog.ManagementOptions{})
	if err != nil {
		t.Fatal(err)
	}
	detail["metadata"].(map[string]string)["description"] = "management"
	if def.Metadata["description"] != "accepted" {
		t.Fatal("public view exposes definition metadata")
	}
}

func TestDatasetMetadataPublicationFailureKeepsAcceptedState(t *testing.T) {
	service, registry, authority, source := newMetadataDataset(t)
	publishMetadataDataset(t, service, 1, map[string]string{"description": "accepted"})
	before := catalog.NormalizeState(service.DumpState())
	head, _ := registry.Head()
	sourceHead, _ := source.Head(snapshot.DefaultRef)
	candidate, err := service.PrepareKnowledgeSet("delivery", 2, before.KnowledgeSets[0].Sources)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Metadata = map[string]string{"description": "candidate"}
	authority.applyErr = errors.New("registry write failed")
	if _, err := service.PublishKnowledgeSet(candidate); err == nil {
		t.Fatal("failed metadata publication reported success")
	}
	if after := catalog.NormalizeState(service.DumpState()); !reflect.DeepEqual(after, before) {
		t.Fatal("failed metadata publication changed accepted state", after)
	}
	if after, _ := registry.Head(); after != head {
		t.Fatal("failed metadata publication advanced registry HEAD")
	}
	authority.applyErr = nil
	if _, err := service.PublishKnowledgeSet(candidate); err != nil {
		t.Fatalf("metadata publication could not resume: %v", err)
	}
	if after, _ := source.Head(snapshot.DefaultRef); after != sourceHead {
		t.Fatal("metadata publication wrote source files")
	}
}

func TestDatasetRegistryRejectsAcceptedMetadataRewriteAndRemoval(t *testing.T) {
	service, registry, _, _ := newMetadataDataset(t)
	publishMetadataDataset(t, service, 1, map[string]string{"description": "accepted"})
	head, _ := registry.Head()
	for _, operation := range []string{"rewrite", "remove"} {
		state := service.DumpState()
		if operation == "rewrite" {
			state.KnowledgeSets[0].Metadata["description"] = "rewritten"
			state.DatasetVersions[0].Metadata["description"] = "rewritten"
		} else {
			state.DatasetVersions = nil
		}
		if err := registry.Save(state, operation, "", "", ""); kernel.CodeOf(err) != kernel.ErrNonFastForward {
			t.Fatalf("%s of accepted release must fail: %v", operation, err)
		}
		if after, _ := registry.Head(); after != head {
			t.Fatal("rejected metadata mutation advanced the authority")
		}
	}
}

func TestDatasetMetadataJSONRequiresStringValues(t *testing.T) {
	for _, metadata := range []string{`{"description":42}`, `{"description":true}`, `{"description":{"nested":"value"}}`, `{"description":null}`, `{"duplicate":"one","duplicate":"two"}`, `[]`, `null`} {
		var definition catalog.KnowledgeSet
		if err := catalog.DecodeJSON([]byte(`{"metadata":`+metadata+`}`), &definition); err == nil {
			t.Fatalf("accepted non-string metadata: %s", metadata)
		}
		var strings map[string]string
		if err := catalog.DecodeJSON([]byte(metadata), &strings); err == nil {
			t.Fatalf("string-map decoder accepted invalid metadata: %s", metadata)
		}
	}
	def := catalog.KnowledgeSet{Metadata: map[string]string{"number": "42", "custom": "", "description": "delivery"}}
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	var decoded catalog.KnowledgeSet
	if err := catalog.DecodeJSON(raw, &decoded); err != nil || !reflect.DeepEqual(decoded.Metadata, def.Metadata) {
		t.Fatalf("string metadata did not round-trip: %#v %v", decoded, err)
	}
}
