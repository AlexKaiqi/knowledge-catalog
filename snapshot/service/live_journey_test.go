package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"kc/identity"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	snapshotcli "kc/snapshot/cli"
	"kc/snapshot/client"
	"kc/snapshot/dataset"
	"kc/snapshot/service"
)

type liveJourneyTransport struct{ principal string }

func (transport liveJourneyTransport) Do(ctx context.Context, _ string, request *http.Request) (*http.Response, error) {
	request = request.Clone(ctx)
	request.Header.Set("X-Kc-As", transport.principal)
	return http.DefaultClient.Do(request)
}

// This independent journey uses the deployed lakeFS data plane, with no Home,
// Invocation, Knowledge or search adapter behind the Snapshot Handler.
func TestLiveSnapshotPublicationJourney(t *testing.T) {
	origin := strings.TrimSpace(os.Getenv("KC_SCENE_LIVE_LAKEFS_URL"))
	if origin == "" {
		t.Skip("set KC_SCENE_LIVE_LAKEFS_URL for the real Snapshot journey")
	}
	credential := strings.TrimSpace(os.Getenv("KC_LAKEFS_CREDENTIAL"))
	live := testkit.NewLakeFSLive(t, origin, credential, os.Getenv("KC_SCENE_LIVE_LAKEFS_STORAGE"))
	sourceName, authorityName := live.NewRepo(), live.NewRepo()
	source, err := snapshot.OpenLakeFS("kr://live/source", live.DSN(sourceName), credential)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := snapshot.OpenLakeFS("kr://live/registry", live.DSN(authorityName), credential)
	if err != nil {
		t.Fatal(err)
	}
	files, ok := snapshot.TreeStoreOf(source)
	if !ok {
		t.Fatal("deployed authority has no literal files")
	}
	write := func(body []byte) kernel.CommitID {
		base := testkit.MustHead(t, source, snapshot.DefaultRef)
		commit, err := files.ApplyTreeCommit(snapshot.TreeChangeSet{TargetRepository: source.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Changes: []snapshot.TreeChange{{Path: "plain.bin", Content: body}}})
		if err != nil {
			t.Fatal(err)
		}
		return commit
	}
	first := []byte{'v', '1', 0, 255}
	firstCommit := write(first)
	sources := snapshot.NewRegistry()
	if err := sources.Add(source); err != nil {
		t.Fatal(err)
	}
	registry, err := dataset.CreateSnapshotRegistry(authority, "")
	if err != nil {
		t.Fatal(err)
	}
	cat, err := dataset.NewService(sources, registry)
	if err != nil {
		t.Fatal(err)
	}
	rules := &datasetHTTPRules{rules: []access.Rule{
		{ID: "manager", Principal: "manager", Actions: []string{"inventory.read", "registry.audit.read", "repositories.manage", "dataset.manage", "admin.grants.manage", access.RepositoryMetadataRead, access.RepositoryMetadataWrite}},
		{ID: "consumer", Principal: "consumer", Dataset: "delivery", Actions: []string{"dataset.resolve", "file.read"}},
	}}
	resources := service.Resources{Datasets: cat, Sources: sources, Access: access.NewService(rules), NativeRepositories: map[string]kernel.RepositoryID{sourceName: source.ID()}}
	handler := service.NewHandler(service.Config{Identity: service.IdentityOptions{Local: true}, NativeOrigin: origin, NativeCredential: func(context.Context) (string, error) { return credential, nil }, Acquire: func(context.Context, identity.HTTPIdentity, string) (service.Resources, func(), error) {
		return resources, func() {}, nil
	}})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	remote := func(principal string) client.Client {
		c, err := client.New(client.Config{BaseURL: server.URL, Transport: liveJourneyTransport{principal}})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	manager, consumer := remote("manager"), remote("consumer")
	ctx, options := context.Background(), client.RequestOptions{}
	executor := snapshotcli.New(manager, snapshotcli.Context{Principal: "manager"})
	if _, err := executor.Execute(ctx, "attach", map[string]any{"repo": string(source.ID())}, options); err != nil {
		t.Fatal(err)
	}
	if _, err := executor.Execute(ctx, "show", nil, options); err != nil {
		t.Fatal(err)
	}
	definition := client.KnowledgeSetRequest{Dataset: "delivery", Revision: 1, Metadata: map[string]string{"description": "可选资源属性", "owner": "data-team"}, Sources: []dataset.KnowledgeSetSource{{Repository: source.ID(), Selector: snapshot.DefaultRef, File: "plain.bin", Target: "docs/data.bin"}}}
	before := testkit.MustHead(t, authority, snapshot.DefaultRef)
	testkit.ExpectCode(t, manager.SnapshotService().DefineKnowledgeSet(ctx, definition, options, nil), kernel.ErrForbidden)
	if testkit.MustHead(t, authority, snapshot.DefaultRef) != before || len(cat.DumpState().KnowledgeSets) != 0 {
		t.Fatal("source permission denial published a Dataset")
	}
	var grant struct {
		ID string `json:"id"`
	}
	if err := manager.AdminService().AddGrant(ctx, client.GrantRequest{Principal: "manager", Actions: []string{access.RepositoryDatasetPublish}, Repository: string(source.ID()), Dataset: "delivery", Ref: snapshot.DefaultRef}, options, &grant); err != nil {
		t.Fatal(err)
	}
	if grant.ID == "" {
		t.Fatal("source grant has no durable rule identity")
	}
	if err := manager.SnapshotService().DefineKnowledgeSet(ctx, definition, options, nil); err != nil {
		t.Fatal(err)
	}
	if err := manager.SnapshotService().SetRepositoryMetadata(ctx, sourceName, map[string]string{"owner": "source-team", "description": "原生 lakeFS 属性"}, options); err != nil {
		t.Fatal(err)
	}
	metadata, err := manager.SnapshotService().RepositoryMetadata(ctx, sourceName, options)
	if err != nil || metadata["owner"] != "source-team" {
		t.Fatalf("native metadata = %v: %v", metadata, err)
	}
	read := func(revision int, want []byte, commit kernel.CommitID) {
		t.Helper()
		var pin dataset.ResolvedKnowledgeSet
		if err := consumer.SnapshotService().ResolveKnowledgeSet(ctx, "delivery", client.KnowledgeSetResolveRequest{Revision: &revision}, options, &pin); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(pin)
		if err != nil {
			t.Fatal(err)
		}
		coordinate := client.WorkspaceFileCoordinate{Dataset: "delivery", Pin: encoded}
		var directory client.WorkspaceFileDirectoryResponse
		if err := consumer.WorkspaceFilesService().Directory(ctx, client.WorkspaceFileDirectoryRequest{WorkspaceFileCoordinate: coordinate, Path: "docs", Limit: 1}, options, &directory); err != nil || len(directory.Entries) != 1 || directory.Entries[0].Name != "data.bin" {
			t.Fatalf("delivered tree = %+v: %v", directory, err)
		}
		var result client.WorkspaceFileReadResponse
		if err := consumer.WorkspaceFilesService().Read(ctx, client.WorkspaceFileReadRequest{WorkspaceFileCoordinate: coordinate, Path: "docs/data.bin", Length: 4096}, options, &result); err != nil || !reflect.DeepEqual(result.Content, want) || result.Pin.Repositories[source.ID()] != commit {
			t.Fatalf("fixed read = %+v: %v", result, err)
		}
	}
	read(1, first, firstCommit)
	testkit.ExpectCode(t, consumer.SnapshotService().ShowDataset(ctx, "delivery", client.DatasetShowOptions{}, options, nil), kernel.ErrForbidden)
	second := []byte("version-two")
	secondCommit := write(second)
	verifyLivePublicationRecovery(t, source, origin, sourceName, credential, secondCommit, firstCommit)
	read(1, first, firstCommit)
	definition.Revision, definition.Metadata = 2, map[string]string{"owner": "new-owner"}
	if err := manager.SnapshotService().DefineKnowledgeSet(ctx, definition, options, nil); err != nil {
		t.Fatal(err)
	}
	// Reopen the real registry instead of replaying client summaries.
	reopened, err := dataset.OpenSnapshotRegistry(authority, "")
	if err != nil {
		t.Fatal(err)
	}
	resources.Datasets, err = dataset.NewService(sources, reopened)
	if err != nil {
		t.Fatal(err)
	}
	read(1, first, firstCommit)
	read(2, second, secondCommit)
	var old map[string]any
	revision := 1
	if err := manager.SnapshotService().ShowDataset(ctx, "delivery", client.DatasetShowOptions{Revision: &revision}, options, &old); err != nil || old["metadata"].(map[string]any)["owner"] != "data-team" {
		t.Fatalf("historical metadata = %v: %v", old, err)
	}
	if _, err := executor.Execute(ctx, "registry audit", map[string]any{"limit": 20}, options); err != nil {
		t.Fatal(err)
	}
	if err := manager.AdminService().RemoveGrant(ctx, grant.ID, options, nil); err != nil {
		t.Fatal(err)
	}
	definition.Revision = 3
	before = testkit.MustHead(t, authority, snapshot.DefaultRef)
	testkit.ExpectCode(t, manager.SnapshotService().DefineKnowledgeSet(ctx, definition, options, nil), kernel.ErrForbidden)
	if testkit.MustHead(t, authority, snapshot.DefaultRef) != before {
		t.Fatal("revoked source permission advanced registry")
	}
	read(1, first, firstCommit)
	if _, err := executor.Execute(ctx, "detach", map[string]any{"repo": string(source.ID())}, options); err != nil {
		t.Fatal(err)
	}
	var pin dataset.ResolvedKnowledgeSet
	err = consumer.SnapshotService().ResolveKnowledgeSet(ctx, "delivery", client.KnowledgeSetResolveRequest{}, options, &pin)
	if err == nil {
		t.Fatal("detached source remained consumable")
	}
}

func verifyLivePublicationRecovery(t *testing.T, source snapshot.Store, origin, nativeName, credential string, target, candidate kernel.CommitID) {
	t.Helper()
	// This fixture has one publisher, stopped between synchronous writes. The
	// private operator creates the interrupted publisher's actual hidden ref.
	lock := "kc-publish-lock-" + string(kernel.CanonicalDigest(string(source.ID()) + "\x00main"))[:24]
	body, err := json.Marshal(map[string]any{"name": lock, "source": candidate, "hidden": true})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, origin+"/api/v1/repositories/"+nativeName+"/branches", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	user, secret, _ := strings.Cut(credential, ":")
	request.SetBasicAuth(user, secret)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create interrupted coordinator HTTP %d", response.StatusCode)
	}
	testkit.ExpectCode(t, snapshot.RecoverLakeFSPublication(source, snapshot.DefaultRef, candidate, candidate), kernel.ErrNonFastForward)
	for range 2 {
		if err := snapshot.RecoverLakeFSPublication(source, snapshot.DefaultRef, target, candidate); err != nil {
			t.Fatal(err)
		}
		if testkit.MustHead(t, source, snapshot.DefaultRef) != target {
			t.Fatal("real recovery changed the source target")
		}
	}
}
