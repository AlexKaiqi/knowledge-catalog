package cli_test

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"kc/client"
	"kc/kernel"
	"kc/object/reader"
	serv "kc/server"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

func TestTemporaryObjectReadBasisKeepsFileScopeAndCurrentAuthorization(t *testing.T) {
	for _, variant := range []struct {
		name     string
		label    string
		revision int
	}{{"anonymous-zero-revision", "", 0}, {"caller-label-is-not-a-published-grant", "published-name", 7}} {
		t.Run(variant.name, func(t *testing.T) {
			home, url, repo, stop := objectReferenceServerLifecycle(t)
			admin := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
			member, root := ordersReference(t, repo, "/columns/id"), ordersReference(t, repo, "")
			body(t, admin("writer", "put", member, "--repo", repo, "--kind", "Member", "--command-id", "temporary-public", "--path-hint", "public/id.yaml", "--value", `{"type":"string"}`))
			body(t, admin("writer", "put", ordersReference(t, repo, "/columns/secret"), "--repo", repo, "--command-id", "temporary-private", "--path-hint", "private/secret.yaml", "--value", `{"type":"private"}`))
			body(t, admin("dataset", "define", "--dataset", "published-name", "--revision", "1", "--source", repo))
			grant := asMap(t, body(t, admin("grant", "add", "--principal", "consumer", "--repo", repo, "--action", "file.read")))
			def := dataset.Dataset{SetID: variant.label, Revision: variant.revision, Sources: []dataset.DatasetSource{{Repository: kernel.RepositoryID(repo), Selector: snapshot.DefaultRef, Path: dataset.MountPath("visible"), SubPath: "public"}}}
			consumer := productClient(t, url, "consumer")
			var first map[string]any
			if err := consumer.Objects().Read(context.Background(), client.ObjectReadRequest{ObjectDatasetIdentity: client.ObjectDatasetIdentity{Definition: &def}, Reference: root}, client.RequestOptions{}, &first); err != nil {
				t.Fatal(err)
			}
			basis, ok := first["basis"].(string)
			if !ok || basis == "" {
				t.Fatalf("temporary definition supplied no read basis: %#v", first)
			}
			store, err := reader.NewReadBasisStore(filepath.Join(home, "read-bases.db"))
			if err != nil {
				t.Fatal(err)
			}
			saved, err := store.Resolve(context.Background(), basis)
			if err != nil || !saved.Temporary || saved.Dataset != variant.label || saved.Revision != variant.revision {
				t.Fatalf("temporary definition was mistaken for a publication: %#v %v", saved, err)
			}
			body(t, admin("writer", "put", member, "--repo", repo, "--command-id", "temporary-advance", "--value", `{"type":"number"}`))
			stop()
			handler := serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{})
			if closer, ok := handler.(interface{ Close() error }); ok {
				t.Cleanup(func() { _ = closer.Close() })
			}
			reopened := httptest.NewServer(handler)
			t.Cleanup(reopened.Close)
			url = reopened.URL
			replay := asMap(t, body(t, kcRemote(t, reopened.URL, "consumer", "read", root, "--basis", basis)))
			columns := asMap(t, asMap(t, replay["value"])["columns"])
			if len(columns) != 1 || asMap(t, columns["id"])["type"] != "string" || replay["commit"] != first["commit"] || asMap(t, replay["state"])["digest"] != asMap(t, first["state"])["digest"] {
				t.Fatalf("temporary replay expanded the file scope or followed HEAD: %#v", replay)
			}
			expectCode(t, kcRemote(t, reopened.URL, "consumer", "read", root, "--repo", repo), string(kernel.ErrForbidden))
			body(t, admin("grant", "remove", "--id", grant["id"].(string)))
			body(t, admin("grant", "add", "--principal", "consumer", "--dataset", "published-name", "--action", "file.read"))
			expectCode(t, kcRemote(t, reopened.URL, "consumer", "read", root, "--basis", basis), string(kernel.ErrForbidden))
		})
	}
}
