package cli_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	kcclient "kc/client"
	objectmodel "kc/object"
)

func TestPublishedImageDiscoveryKeepsBusinessIdentity(t *testing.T) {
	f := newPublishedObjectFixture(t, true)
	commit, definition := f.publish(t, "image")
	f.verifyDiscovery(t, "image", "image/tea-tin", "imagecatalogtoken", commit, definition)
	resources := definition["resources"].([]any)
	if len(resources) != 1 || resources[0].(map[string]any)["commit"] != f.resourceCommit {
		t.Fatalf("image did not keep its independent resource version: %#v", resources)
	}
	f.verifyResourceFile(t, f.resourceStore, f.resourceCommit, "media/tea-tin.png")

	before := asMap(t, body(t, f.consume(t, "read", testObjectReference(t, f.repository, "image/tea-tin", ""), "--commit", commit)))
	raw, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	moved := publishedCommit(t, asMap(t, body(t, f.provide(t, "writer", "put", testObjectReference(t, f.repository, "image/tea-tin", "/definition"), "--command-id", "image-description-move", "--schema-ref", "schema/published-object.definition/v1", "--path-hint", "moved/image-description.yaml", "--value", string(raw)))))
	if moved == commit {
		t.Fatal("moving the representation must create a new Snapshot version")
	}
	for _, version := range []string{commit, moved} {
		read := asMap(t, body(t, f.consume(t, "read", testObjectReference(t, f.repository, "image/tea-tin", ""), "--commit", version)))
		if read["commit"] != version || !reflect.DeepEqual(read["value"], before["value"]) {
			t.Fatalf("representation path changed object identity or pinned value: %#v", read)
		}
	}
	resolved := asMap(t, body(t, f.consume(t, "resolve", "--repo", f.repository, "--commit", moved,
		"--object", "image/tea-tin", "--aspect", "definition")))
	if resolved["commit"] != moved || resolved["objectId"] != "image/tea-tin" {
		t.Fatalf("object locator did not follow the explicit path move: %#v", resolved)
	}
	var location objectmodel.Resolution
	if err := productClient(t, f.server, "agent:object-consumer").KnowledgeService().Resolve(context.Background(),
		kcclient.KnowledgeResolveRequest{KnowledgeScope: kcclient.KnowledgeScope{Repository: f.repository, Commit: moved},
			Object: "image/tea-tin", Aspect: "definition"}, kcclient.RequestOptions{}, &location); err != nil {
		t.Fatal(err)
	}
	if location.PathHint != "moved/image-description.yaml" || location.ObjectID != "image/tea-tin" {
		t.Fatalf("typed object locator did not follow the explicit path move: %#v", location)
	}
}

func TestPublishedModuleDiscoveryKeepsDispersedResourceReferences(t *testing.T) {
	f := newPublishedObjectFixture(t, true)
	commit, definition := f.publish(t, "module")
	f.verifyDiscovery(t, "module", "module/tax-calculation", "modulecatalogtoken", commit, definition)
	resources := definition["resources"].([]any)
	if len(resources) != 3 {
		t.Fatalf("module lost its resource collection: %#v", resources)
	}
	for i, path := range []string{"src/tax/calculate.js", "test-vectors/tax.json", "legal/LICENSE.txt"} {
		resource := resources[i].(map[string]any)
		repo, version := f.resourceRepository, f.resourceCommit
		store := f.resourceStore
		if i == 2 {
			repo, version, store = f.otherRepository, f.otherCommit, f.otherStore
		}
		if resource["repository"] != repo || resource["commit"] != version || resource["path"] != path {
			t.Fatalf("module replaced a dispersed reference: %#v", resource)
		}
		f.verifyResourceFile(t, store, version, path)
	}
	expectCode(t, f.consume(t, "read", testObjectReference(t, f.otherRepository, "private/policy", "")), "FORBIDDEN")
	if after := f.head(t, f.otherRepository); after != f.otherHead {
		t.Fatalf("publishing/reading module references wrote to the other repository: %s -> %s", f.otherHead, after)
	}
}

func TestPublishedExternalTableDiscoveryDoesNotContactOfflineSource(t *testing.T) {
	f := newPublishedObjectFixture(t, true)
	commit, definition := f.publish(t, "table")
	f.verifyDiscovery(t, "table", "table/external-orders", "externalcatalogtoken", commit, definition)
	resource := definition["resources"].([]any)[0].(map[string]any)
	if resource["origin"] != f.offlineOrigin || resource["database"] != "sales" || resource["table"] != "orders" {
		t.Fatalf("external table descriptor was rewritten: %#v", resource)
	}
	if calls := f.offlineCalls.Load(); calls != 0 {
		t.Fatalf("publication/discovery contacted the unavailable table source %d times", calls)
	}
}

func TestObjectPublicationDoesNotRequireIndexOrInstalledHandler(t *testing.T) {
	f := newPublishedObjectFixture(t, false)
	commit, definition := f.publish(t, "table")
	read := asMap(t, body(t, f.consume(t, "read", testObjectReference(t, f.repository, "table/external-orders", ""), "--commit", commit)))
	if !reflect.DeepEqual(asMap(t, read["value"])["definition"], definition) {
		t.Fatalf("missing capabilities blocked fixed object read: %#v", read)
	}
	expectCode(t, f.consume(t, "search", "--repo", f.repository, "--commit", commit, "--query", "externalcatalogtoken"), "CAPABILITY_UNSATISFIED")
	f.verifyUninstalledInspection(t, "table/external-orders", commit)
	if f.offlineCalls.Load() != 0 {
		t.Fatal("basic publication/read called the external table")
	}
}
