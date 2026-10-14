package cli_test

import (
	"context"
	"encoding/json"
	snapshotclient "kc/snapshot/client"
	"net/http/httptest"
	"os"
	"testing"

	"kc/client"
	"kc/internal/testkit"
	"kc/kernel"
	serv "kc/server"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

// The consumer crosses the typed Client and actual Server without a host
// bridge, project context or FUSE. The explicit adapter suite also runs this
// journey against its required live lakeFS authority.
func TestConsumerUsesActualServerContract(t *testing.T) {
	isolateClientCredentials(t)
	t.Setenv("KC_DATASET", "")
	t.Setenv("KC_CATALOG", "")
	home := testkit.TempDir(t)
	const cat, repo, principal, admin = "kr://probe/catalog", "kr://probe/docs", "consumer", "operator"
	body(t, kc(home, "local", "init"))
	if os.Getenv("KC_REQUIRE_LIVE_ADAPTERS") == "1" {
		if testing.Short() {
			t.Fatal("the required live consumer contract cannot run in the short suite")
		}
		live := testkit.NewLakeFSLive(t, os.Getenv("KC_SCENE_LIVE_LAKEFS_URL"), os.Getenv("KC_LAKEFS_CREDENTIAL"), os.Getenv("KC_SCENE_LIVE_LAKEFS_STORAGE"))
		seedRepo(t, home, repo, "--driver", "lakefs", "--dsn", live.DSN(live.NewRepo()))
	} else {
		seedRepo(t, home, repo)
	}
	body(t, kc(home, "local", "grant", "bootstrap", "--principal", admin))
	handler := serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	invoke := func(args ...string) kcRunResult { return kcRemote(t, server.URL, admin, args...) }
	body(t, invoke("writer", "put", testObjectReference(t, repo, "schema/note", ""), "--command-id", "schema-probe", "--value", `{"entity":"Note","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}`))
	body(t, invoke("writer", "put", testObjectReference(t, repo, "note/one", ""), "--command-id", "note-probe", "--schema-ref", "schema/note", "--value", `{"body":"consumed through the actual Server"}`))
	body(t, invoke("dataset", "define", "--dataset", "named", "--revision", "1", "--source", repo))
	body(t, invoke("grant", "add", "--principal", principal, "--action", "inventory.read,dataset.resolve"))
	readGrant := asMap(t, body(t, invoke("grant", "add", "--principal", principal, "--action", "file.read")))
	body(t, invoke("grant", "add", "--principal", principal, "--repo", repo, "--action", "object.read"))
	consumer := productClient(t, server.URL, principal)
	ctx, options := context.Background(), client.RequestOptions{}
	var inventory map[string]any
	if err := consumer.SnapshotService().Show(ctx, snapshotclient.RequestOptions{RequestID: options.RequestID}, &inventory); err != nil {
		t.Fatal(err)
	}
	if _, present := inventory["catalogId"]; present {
		t.Fatalf("inventory exposes Catalog identity: %#v", inventory)
	}
	found := false
	var repositories struct {
		Repositories []struct {
			ID string `json:"id"`
		} `json:"repositories"`
	}
	if err := consumer.SnapshotService().Repositories(ctx, snapshotclient.RequestOptions{RequestID: options.RequestID}, &repositories); err != nil {
		t.Fatal(err)
	}
	found = false
	for _, item := range repositories.Repositories {
		found = found || item.ID == repo
	}
	if !found {
		t.Fatalf("actual Server inventory omitted the source repository: %#v", repositories)
	}
	var sets struct {
		Datasets []struct {
			ID           string   `json:"id"`
			Repositories []string `json:"repositories"`
		} `json:"datasets"`
	}
	if err := consumer.SnapshotService().Datasets(ctx, snapshotclient.RequestOptions{RequestID: options.RequestID}, &sets); err != nil {
		t.Fatal(err)
	}
	if len(sets.Datasets) != 1 || sets.Datasets[0].ID != "named" || len(sets.Datasets[0].Repositories) != 1 || sets.Datasets[0].Repositories[0] != repo {
		t.Fatalf("actual Server named Dataset inventory drifted: %#v", sets)
	}
	var detail struct {
		Sources []dataset.DatasetSource `json:"sources"`
	}
	if err := consumer.SnapshotService().Dataset(ctx, "named", snapshotclient.RequestOptions{RequestID: options.RequestID}, &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Sources) != 1 || detail.Sources[0].Selector != snapshot.DefaultRef {
		t.Fatalf("omitted source selector did not publish the default ref: %#v", detail)
	}
	var named dataset.ResolvedDataset
	if err := consumer.SnapshotService().ResolveDataset(ctx, "named", snapshotclient.DatasetResolveRequest{}, snapshotclient.RequestOptions{RequestID: options.RequestID}, &named); err != nil {
		t.Fatal(err)
	}
	if named.SetID != "named" || named.PinID == "" || len(named.Repositories) != 1 || named.Repositories[repo] == "" {
		t.Fatalf("named resolve omitted its fixed coordinates: %#v", named)
	}
	definition := dataset.Dataset{Revision: 1, Sources: []dataset.DatasetSource{{Repository: repo, Selector: snapshot.DefaultRef}}}
	var selected dataset.ResolvedDataset
	if err := consumer.SnapshotService().ResolveDefinition(ctx, snapshotclient.DatasetRequest{Revision: definition.Revision, Sources: definition.Sources}, snapshotclient.RequestOptions{RequestID: options.RequestID}, &selected); err != nil {
		t.Fatal(err)
	}
	if selected.PinID == "" || len(selected.Repositories) != 1 || selected.Repositories[repo] == "" || selected.Repositories[repo] != named.Repositories[repo] {
		t.Fatalf("selected definition resolve changed membership or basis: named=%#v selected=%#v", named, selected)
	}
	marshalPin := func(pin dataset.ResolvedDataset) json.RawMessage {
		t.Helper()
		raw, err := json.Marshal(pin)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	fixed := client.ObjectReadRequest{ObjectDatasetIdentity: client.ObjectDatasetIdentity{Definition: &definition, Pin: marshalPin(selected)}, Reference: testObjectReference(t, repo, "note/one", "")}
	body(t, invoke("writer", "put", testObjectReference(t, repo, "note/one", ""), "--command-id", "note-moved", "--schema-ref", "schema/note", "--value", `{"body":"new branch body after pin"}`))
	assertRead := func(request client.ObjectReadRequest, commit kernel.CommitID, want string) {
		t.Helper()
		var value map[string]any
		if err := consumer.Objects().Read(ctx, request, options, &value); err != nil {
			t.Fatal(err)
		}
		if value["reference"] != request.Reference || value["commit"] != string(commit) || asMap(t, value["value"])["body"] != want {
			t.Fatalf("actual Server fixed read changed commit or Canonical body: %#v", value)
		}
	}
	assertRead(fixed, selected.Repositories[repo], "consumed through the actual Server")
	assertRead(client.ObjectReadRequest{ObjectDatasetIdentity: client.ObjectDatasetIdentity{Dataset: "named", Pin: marshalPin(named)}, Reference: testObjectReference(t, repo, "note/one", "")}, named.Repositories[repo], "consumed through the actual Server")
	var latest map[string]any
	if err := consumer.Objects().Read(ctx, client.ObjectReadRequest{Reference: testObjectReference(t, repo, "note/one", ""), Ref: snapshot.DefaultRef}, options, &latest); err != nil {
		t.Fatal(err)
	}
	if latest["commit"] == string(selected.Repositories[repo]) || asMap(t, latest["value"])["body"] != "new branch body after pin" {
		t.Fatalf("the authority did not advance after pin: %#v", latest)
	}
	var rejected any
	stranger := productClient(t, server.URL, "stranger")
	if err := stranger.Objects().Read(ctx, fixed, options, &rejected); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("fixed pin authorized another principal: result=%#v err=%v", rejected, err)
	}
	body(t, invoke("grant", "remove", "--id", readGrant["id"].(string)))
	if err := consumer.Objects().Read(ctx, fixed, options, &rejected); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("fixed pin bypassed current file.read grant revocation: result=%#v err=%v", rejected, err)
	}
	t.Log("PASS actual Server inventory -> named resolve -> selected definition pin -> fixed structured read; current authorization, no FUSE")
}
