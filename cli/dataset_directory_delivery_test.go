package cli_test

import (
	"context"
	"encoding/json"
	inv "kc/invocation"
	serv "kc/server"
	snapshotclient "kc/snapshot/client"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"kc/client"
	apphome "kc/home"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

const (
	deliveryCatalog = "kr://delivery/catalog"
	deliveryDocs    = "kr://delivery/docs"
	deliveryData    = "kr://delivery/data"
)

// doc/design/dataset.md, KS-01/02 and AUTH-01–03: publish through the Server, then
// consume only selected subtrees under new paths with a Dataset-only grant.
// This exercises the lakeFS adapter against its in-process fixture, not FUSE
// or a deployed lakeFS service. It does not claim per-file remapping support.
func TestDatasetDirectoryDeliveryReorganizesMultipleRepositories(t *testing.T) {
	f := newDirectoryDelivery(t)
	opened := f.mounts(t, nil)
	if opened.Pin.Revision != 1 || len(opened.Pin.Repositories) != 2 || len(opened.Mounts) != 3 {
		t.Fatalf("expected one release with two repositories and three mounts: %#v", opened)
	}
	want := map[string]struct {
		repository kernel.RepositoryID
		subPath    string
		content    string
	}{
		"reference/policies": {deliveryDocs, "handbook/policies", "policy v1\n"},
		"reference/guides":   {deliveryDocs, "docs/guides", "guide v1\n"},
		"data/metrics":       {deliveryData, "semantic/metrics", "metric v1\n"},
	}
	seen := map[string]bool{}
	for _, mount := range opened.Mounts {
		expected, ok := want[mount.Path]
		if !ok || seen[mount.Path] || mount.Repository != expected.repository || mount.SubPath != expected.subPath ||
			mount.Commit == "" || mount.Commit != opened.Pin.Repositories[expected.repository] {
			t.Fatalf("wrong delivery path or source: %#v", mount)
		}
		seen[mount.Path] = true
		var directory snapshotclient.WorkspaceFileDirectoryResponse
		if err := f.reader.WorkspaceFilesService().Directory(context.Background(), snapshotclient.WorkspaceFileDirectoryRequest{
			WorkspaceFileCoordinate: f.coordinate(t, &opened.Pin), MountPath: mount.Path,
		}, snapshotclient.RequestOptions{}, &directory); err != nil {
			t.Fatal(err)
		}
		if len(directory.Entries) != 1 || directory.Entries[0].Name != "README.md" || directory.Entries[0].Kind != "file" ||
			!directory.Exhausted || directory.Continuation != "" || !reflect.DeepEqual(directory.Mount, mount) ||
			!reflect.DeepEqual(directory.Pin, opened.Pin) {
			t.Fatalf("delivery leaked siblings or lost its source: %#v", directory)
		}
		f.assertRead(t, opened.Pin, mount.Path, expected.content)
	}
	// Each source also has an unselected secret sibling. Neither source paths
	// nor mount-relative traversal may turn Dataset access into whole-repo access.
	for _, tc := range []struct {
		mount string
		file  string
		code  kernel.ErrorCode
	}{
		{"handbook/policies", "README.md", kernel.ErrForbidden},
		{"reference/policies", "../secret.txt", kernel.ErrUsageInvalid},
		{"data/metrics", "../secret.txt", kernel.ErrUsageInvalid},
	} {
		var out snapshotclient.WorkspaceFileReadResponse
		err := f.reader.WorkspaceFilesService().Read(context.Background(), snapshotclient.WorkspaceFileReadRequest{
			WorkspaceFileCoordinate: f.coordinate(t, &opened.Pin), MountPath: tc.mount, File: tc.file,
		}, snapshotclient.RequestOptions{}, &out)
		if err == nil || kernel.CodeOf(err) != tc.code || len(out.Content) != 0 {
			t.Fatalf("out-of-scope read %s/%s: response=%#v err=%v", tc.mount, tc.file, out, err)
		}
	}
}

func TestDatasetDirectoryDeliveryRepublishKeepsOldLayoutAndBytes(t *testing.T) {
	f := newDirectoryDelivery(t)
	old := f.mounts(t, nil)
	f.write(t, deliveryDocs, "policy-v2", []snapshot.TreeChange{{Path: "handbook/policies/README.md", Content: []byte("policy v2\n")}})
	// Moving source HEAD alone changes neither the release nor delivered bytes.
	if current := f.mounts(t, nil); !reflect.DeepEqual(current, old) {
		t.Fatalf("source update changed the accepted release: old=%#v current=%#v", old, current)
	}
	f.assertRead(t, old.Pin, "reference/policies", "policy v1\n")
	if err := f.publish(2, directoryDeliverySources("handbook/rules", "help/guides", "data/metrics")); err != nil {
		t.Fatal(err)
	}
	current := f.mounts(t, nil)
	if current.Pin.Revision != 2 || current.Pin.Repositories[deliveryDocs] == old.Pin.Repositories[deliveryDocs] ||
		current.Pin.Repositories[deliveryData] != old.Pin.Repositories[deliveryData] {
		t.Fatalf("new release did not adopt the exact changed source: old=%#v new=%#v", old.Pin, current.Pin)
	}
	f.assertRead(t, current.Pin, "handbook/rules", "policy v2\n")
	f.assertRead(t, current.Pin, "help/guides", "guide v1\n")
	f.assertRead(t, current.Pin, "data/metrics", "metric v1\n")
	if replay := f.mounts(t, &old.Pin); !reflect.DeepEqual(replay, old) {
		t.Fatalf("new layout rewrote old view: old=%#v replay=%#v", old, replay)
	}
	f.assertRead(t, old.Pin, "reference/policies", "policy v1\n")
	f.assertRead(t, old.Pin, "reference/guides", "guide v1\n")
	for _, tc := range []struct {
		pin   dataset.ResolvedDataset
		mount string
	}{{old.Pin, "handbook/rules"}, {current.Pin, "reference/policies"}} {
		var out snapshotclient.WorkspaceFileReadResponse
		err := f.reader.WorkspaceFilesService().Read(context.Background(), snapshotclient.WorkspaceFileReadRequest{
			WorkspaceFileCoordinate: f.coordinate(t, &tc.pin), MountPath: tc.mount, File: "README.md",
		}, snapshotclient.RequestOptions{}, &out)
		if kernel.CodeOf(err) != kernel.ErrForbidden {
			t.Fatalf("release %d accepted another release's path %s: %v", tc.pin.Revision, tc.mount, err)
		}
	}
}

// The desired boundary includes normalization inside the relative path, not
// only a leading ../. The gateway currently validates only the latter; this
// regression records the implementation gap without accepting leaked bytes.
func TestDatasetDirectoryDeliveryRejectsTraversalInsideRelativePaths(t *testing.T) {
	f := newDirectoryDelivery(t)
	opened := f.mounts(t, nil)
	for _, mount := range []string{"reference/policies", "reference/guides", "data/metrics"} {
		t.Run(mount, func(t *testing.T) {
			t.Run("read", func(t *testing.T) {
				var out snapshotclient.WorkspaceFileReadResponse
				err := f.reader.WorkspaceFilesService().Read(context.Background(), snapshotclient.WorkspaceFileReadRequest{
					WorkspaceFileCoordinate: f.coordinate(t, &opened.Pin), MountPath: mount, File: "scratch/../../secret.txt",
				}, snapshotclient.RequestOptions{}, &out)
				if kernel.CodeOf(err) != kernel.ErrUsageInvalid || len(out.Content) != 0 {
					t.Fatalf("normalized path escaped selected subtree: response=%#v err=%v", out, err)
				}
			})
			t.Run("list", func(t *testing.T) {
				var out snapshotclient.WorkspaceFileDirectoryResponse
				err := f.reader.WorkspaceFilesService().Directory(context.Background(), snapshotclient.WorkspaceFileDirectoryRequest{
					WorkspaceFileCoordinate: f.coordinate(t, &opened.Pin), MountPath: mount, Directory: "scratch/../..",
				}, snapshotclient.RequestOptions{}, &out)
				if kernel.CodeOf(err) != kernel.ErrUsageInvalid || len(out.Entries) != 0 {
					t.Fatalf("normalized directory escaped selected subtree: response=%#v err=%v", out, err)
				}
			})
		})
	}
}

func TestDatasetDirectoryDeliveryRejectsConflictsWithoutReplacingRelease(t *testing.T) {
	f := newDirectoryDelivery(t)
	accepted := f.mounts(t, nil)
	for _, tc := range []struct {
		name  string
		paths []string
	}{
		{"same target", []string{"reference/policies", "reference/guides", "reference/policies"}},
		{"nested targets", []string{"reference/policies", "reference/guides", "reference/policies/metrics"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := f.publish(2, directoryDeliverySources(tc.paths...))
			if kernel.CodeOf(err) != kernel.ErrDatasetInvalid {
				t.Fatalf("conflicting delivery paths were not rejected: %v", err)
			}
			if current := f.mounts(t, nil); !reflect.DeepEqual(current, accepted) {
				t.Fatalf("failed candidate replaced the accepted release: %#v", current)
			}
			f.assertRead(t, accepted.Pin, "reference/policies", "policy v1\n")
			f.assertRead(t, accepted.Pin, "data/metrics", "metric v1\n")
		})
	}
	// Rejected candidates must not consume revision 2 or poison later publication.
	if err := f.publish(2, directoryDeliverySources("reference/policies", "reference/guides", "data/metrics")); err != nil {
		t.Fatal(err)
	}
	if current := f.mounts(t, nil); current.Pin.Revision != 2 {
		t.Fatalf("valid retry did not publish revision 2: %#v", current)
	}
}

type directoryDelivery struct {
	cfg         apphome.DeploymentConfig
	serverURL   string
	operator    *client.Client
	reader      *client.Client
	sourceReads atomic.Int64
}

func newDirectoryDelivery(t *testing.T) *directoryDelivery {
	t.Helper()
	f := &directoryDelivery{}
	sourceRepos := map[string]bool{}
	lakefs := testkit.NewLakeFSFakeWithInterceptor(t, func(_ http.ResponseWriter, r *http.Request) bool {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if r.Method == http.MethodGet && len(parts) == 7 && parts[2] == "repositories" &&
			parts[4] == "refs" && parts[6] == "objects" && sourceRepos[parts[3]] {
			f.sourceReads.Add(1)
		}
		return false
	})
	t.Setenv("KC_LAKEFS_CREDENTIAL", lakefs.Credential())
	root := t.TempDir()
	f.cfg = apphome.DeploymentConfig{
		Version: 1, StateDir: filepath.Join(root, "state"), CacheDir: filepath.Join(root, "cache"),
		Auth: "local", BootstrapPrincipal: "agent:publisher", Stores: apphome.StoresFile{Index: "none"},
		Registry: apphome.RegistryBinding{ID: deliveryCatalog, Driver: "lakefs", DSN: lakefs.DSN(lakefs.NewRepo())},
		Repositories: []apphome.RepositoryBinding{{ID: "kr://kc/system", Driver: "lakefs", DSN: lakefs.DSN(lakefs.NewRepo())},
			{ID: deliveryDocs, Driver: "lakefs", DSN: lakefs.DSN(lakefs.NewRepo())},
			{ID: deliveryData, Driver: "lakefs", DSN: lakefs.DSN(lakefs.NewRepo())},
		},
	}
	for _, binding := range f.cfg.Repositories {
		if binding.ID == deliveryDocs || binding.ID == deliveryData {
			endpoint, err := snapshot.ParseLakeFSDSN(binding.DSN)
			if err != nil {
				t.Fatal(err)
			}
			sourceRepos[endpoint.Repository] = true
		}
	}
	if err := apphome.InitializeDeployment(f.cfg, func(dir, principal string) error {
		return inv.WriteAllow(dir, inv.AllowFile{Rules: []inv.AllowRule{
			{ID: "publisher", Principal: principal, Actions: []string{"*"}},
			{ID: "delivery-reader", Principal: "agent:consumer", Actions: []string{"dataset.resolve", "file.read"}, Dataset: "delivery"},
		}})
	}); err != nil {
		t.Fatal(err)
	}
	f.write(t, deliveryDocs, "docs-v1", []snapshot.TreeChange{
		{Path: "handbook/policies/README.md", Content: []byte("policy v1\n")},
		{Path: "docs/guides/README.md", Content: []byte("guide v1\n")},
		{Path: "handbook/secret.txt", Content: []byte("unpublished policy\n")},
		{Path: "docs/secret.txt", Content: []byte("unpublished guide\n")},
	})
	f.write(t, deliveryData, "data-v1", []snapshot.TreeChange{
		{Path: "semantic/metrics/README.md", Content: []byte("metric v1\n")},
		{Path: "semantic/secret.txt", Content: []byte("unpublished data\n")},
	})
	raw, err := json.Marshal(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "deployment.json")
	if err := os.WriteFile(configPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	handler, err := serv.HTTPHandlerFromConfig(configPath, serv.HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handler.(interface{ Close() error }).Close() })
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	f.serverURL = server.URL
	newClient := func(principal string) *client.Client {
		c, err := client.New(client.Config{BaseURL: server.URL, HTTPClient: server.Client()})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Login(context.Background(), client.LoginRequest{Identity: client.Identity{Principal: principal}}); err != nil {
			t.Fatal(err)
		}
		return c
	}
	f.operator, f.reader = newClient("agent:publisher"), newClient("agent:consumer")
	for _, repository := range []string{deliveryDocs, deliveryData} {
		if err := f.operator.SnapshotService().AttachRepository(context.Background(),
			snapshotclient.RepositoryAttachRequest{Repository: repository}, snapshotclient.RequestOptions{}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.publish(1, directoryDeliverySources("reference/policies", "reference/guides", "data/metrics")); err != nil {
		t.Fatal(err)
	}
	return f
}

func directoryDeliverySources(paths ...string) []dataset.DatasetSource {
	return []dataset.DatasetSource{
		{Repository: deliveryDocs, Selector: snapshot.DefaultRef, Path: &paths[0], SubPath: "handbook/policies"},
		{Repository: deliveryDocs, Selector: snapshot.DefaultRef, Path: &paths[1], SubPath: "docs/guides"},
		{Repository: deliveryData, Selector: snapshot.DefaultRef, Path: &paths[2], SubPath: "semantic/metrics"},
	}
}

func (f *directoryDelivery) write(t *testing.T, repository, commandID string, changes []snapshot.TreeChange) {
	t.Helper()
	opened, err := apphome.OpenDeployment(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	source, err := opened.Store.Require(kernel.RepositoryID(repository), kernel.ErrUsageInvalid)
	if err != nil {
		t.Fatal(err)
	}
	base, err := source.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := opened.TreeWriter.Commit(commandID, snapshot.TreeChangeSet{
		TargetRepository: kernel.RepositoryID(repository), TargetRef: snapshot.DefaultRef,
		BaseCommit: base, ExpectedTargetCommit: base, Changes: changes,
	}); err != nil {
		t.Fatal(err)
	}
}

func (f *directoryDelivery) publish(revision int, sources []dataset.DatasetSource) error {
	return f.operator.SnapshotService().DefineDataset(context.Background(),
		snapshotclient.DatasetRequest{Dataset: "delivery", Revision: revision, Sources: sources}, snapshotclient.RequestOptions{}, nil)
}

func (f *directoryDelivery) coordinate(t *testing.T, pin *dataset.ResolvedDataset) snapshotclient.WorkspaceFileCoordinate {
	t.Helper()
	coordinate := snapshotclient.WorkspaceFileCoordinate{Dataset: "delivery", View: "repository"}
	if pin != nil {
		raw, err := json.Marshal(pin)
		if err != nil {
			t.Fatal(err)
		}
		coordinate.Pin = raw
	}
	return coordinate
}

func (f *directoryDelivery) mounts(t *testing.T, pin *dataset.ResolvedDataset) snapshotclient.WorkspaceFileMountsResponse {
	t.Helper()
	var out snapshotclient.WorkspaceFileMountsResponse
	if err := f.reader.WorkspaceFilesService().Mounts(context.Background(), snapshotclient.WorkspaceFileMountsRequest{
		WorkspaceFileCoordinate: f.coordinate(t, pin),
	}, snapshotclient.RequestOptions{}, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (f *directoryDelivery) assertRead(t *testing.T, pin dataset.ResolvedDataset, mount, content string) {
	t.Helper()
	var out snapshotclient.WorkspaceFileReadResponse
	if err := f.reader.WorkspaceFilesService().Read(context.Background(), snapshotclient.WorkspaceFileReadRequest{
		WorkspaceFileCoordinate: f.coordinate(t, &pin), MountPath: mount, File: "README.md",
	}, snapshotclient.RequestOptions{}, &out); err != nil {
		t.Fatal(err)
	}
	if string(out.Content) != content || out.Mount.Path != mount || out.File != "README.md" || out.Offset != 0 ||
		!out.EOF || out.TotalBytes != int64(len(content)) || !reflect.DeepEqual(out.Pin, pin) ||
		out.Mount.Commit == "" || out.Mount.Commit != pin.Repositories[out.Mount.Repository] {
		t.Fatalf("wrong delivered bytes or source at %s: %#v", mount, out)
	}
}
