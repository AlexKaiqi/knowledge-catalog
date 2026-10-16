package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	apphome "kc/home"
	"kc/internal/testkit"
	"kc/kernel"
	serv "kc/server"
	"kc/snapshot"
)

// exampleWorld is the publication plan for the public Qinghe examples. The
// publish script and this test read the same world.json, so the plan cannot
// drift from what is verified here.
type exampleWorld struct {
	Repositories []struct {
		ID        string `json:"id"`
		Provision string `json:"provision"`
	} `json:"repositories"`
	Datasets []string      `json:"datasets"`
	Steps    []exampleStep `json:"steps"`
}

type exampleStep struct {
	Kind       string `json:"kind"`
	Repository string `json:"repository"`
	Dir        string `json:"dir"`
	CommandID  string `json:"commandId"`
	Dataset    string `json:"dataset"`
	Revision   int    `json:"revision"`
	Recipe     string `json:"recipe"`
}

func exampleWorldRoot() string {
	return filepath.Join(scenesRoot(), "service-initialized", "named-repositories-created", "qinghe-object-published", "_materials", "examples")
}

func readExampleWorld(t *testing.T) exampleWorld {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(exampleWorldRoot(), "world.json"))
	if err != nil {
		t.Fatal(err)
	}
	var world exampleWorld
	if err := json.Unmarshal(raw, &world); err != nil {
		t.Fatal(err)
	}
	return world
}

type exampleDeployment struct {
	fake     *testkit.LakeFSFake
	physical map[string]string
	stores   map[string]snapshot.Store
	url      string
	operator string
}

// The deployment binds each world repository statically. Provisioning (managed
// create or connected lakeFS) is the publish script's concern; this test proves
// the materials, raw files and Dataset revisions compose as the README claims.
func openExampleDeployment(t *testing.T, world exampleWorld) *exampleDeployment {
	t.Helper()
	root := t.TempDir()
	t.Setenv("KC_LAKEFS_CREDENTIAL", testkit.LakeFSFakeCredential)
	fake := testkit.NewLakeFSFake(t)
	cfg := apphome.DeploymentConfig{Version: 1, StateDir: filepath.Join(root, "durable"), CacheDir: filepath.Join(root, "instance"), Auth: "local", BootstrapPrincipal: "agent:operator",
		Repositories:     []apphome.RepositoryBinding{{ID: "kr://kc/system", Driver: "lakefs", DSN: fake.DSN(fake.NewRepo())}},
		Registry:         apphome.RegistryBinding{ID: "kr://kc/registry", Driver: "lakefs", DSN: fake.DSN(fake.NewRepo())},
		RepositoryAccess: []apphome.RepositoryAccess{apphome.SystemRepositoryAccess()}}
	d := &exampleDeployment{fake: fake, physical: map[string]string{}, stores: map[string]snapshot.Store{}, operator: cfg.BootstrapPrincipal}
	for _, repo := range world.Repositories {
		physical := fake.NewRepo()
		d.physical[repo.ID] = physical
		cfg.Repositories = append(cfg.Repositories, apphome.RepositoryBinding{ID: repo.ID, Driver: "lakefs", DSN: fake.DSN(physical)})
		store, err := snapshot.OpenLakeFS(kernel.RepositoryID(repo.ID), fake.DSN(physical), fake.Credential())
		if err != nil {
			t.Fatal(err)
		}
		if closer, ok := store.(io.Closer); ok {
			t.Cleanup(func() { _ = closer.Close() })
		}
		d.stores[repo.ID] = store
	}
	for _, id := range world.Datasets {
		cfg.DatasetAccess = append(cfg.DatasetAccess, apphome.DatasetAccess{ID: id, AuthenticatedActions: apphome.AuthenticatedDatasetActions()})
	}
	path := filepath.Join(root, "deployment.json")
	writeDeployment(t, path, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	body(t, deploymentCommand(t, "deployment", "system", "publish", "--config", path))
	handler, err := serv.HTTPHandlerFromConfig(path, serv.HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		server.Close()
		if closer, ok := handler.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	})
	d.url = server.URL
	return d
}

func (d *exampleDeployment) as(t *testing.T, principal string, args ...string) kcRunResult {
	t.Helper()
	return kcRemote(t, d.url, principal, args...)
}

// commitRawFiles mirrors the integrator committing original bytes to its own
// lakeFS repository. Unchanged trees are skipped, as the publish script does.
func (d *exampleDeployment) commitRawFiles(t *testing.T, repository, dir string) bool {
	t.Helper()
	store := d.stores[repository]
	head, err := store.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	tree, ok := store.(snapshot.TreeReader)
	if !ok {
		t.Fatalf("lakeFS store must expose literal tree reads")
	}
	var changes []snapshot.TreeChange
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if current, err := tree.ReadFile(rel, head); err == nil && bytes.Equal(current, raw) {
			return nil
		}
		changes = append(changes, snapshot.TreeChange{Path: rel, Content: raw})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) == 0 {
		return false
	}
	if _, err := d.fake.CommitFiles(d.physical[repository], head, changes); err != nil {
		t.Fatal(err)
	}
	return true
}

func (d *exampleDeployment) publish(t *testing.T, world exampleWorld, attach bool) int {
	t.Helper()
	root := exampleWorldRoot()
	if attach {
		for _, repo := range world.Repositories {
			body(t, d.as(t, d.operator, "attach", "--repo", repo.ID))
		}
	}
	// The plan is linear. Writer receipts and accepted Dataset revisions are
	// verifiable on the Server; raw file commits are not. Publication resumes
	// after the last verifiable step, so an earlier raw tree is never replayed
	// over a later correction.
	done := func(step exampleStep) bool {
		switch step.Kind {
		case "commit":
			return d.as(t, d.operator, "writer", "receipt", "--command-id", step.CommandID).Status == 0
		case "define":
			return d.as(t, d.operator, "dataset", "show", step.Dataset, "--revision", strconv.Itoa(step.Revision)).Status == 0
		}
		return false
	}
	resume := 0
	for i, step := range world.Steps {
		if done(step) {
			resume = i + 1
		}
	}
	for _, step := range world.Steps[resume:] {
		switch step.Kind {
		case "files":
			d.commitRawFiles(t, step.Repository, filepath.Join(root, filepath.FromSlash(step.Dir)))
		case "commit":
			result := d.as(t, d.operator, "writer", "commit", "--command-id", step.CommandID, "--repo", step.Repository, "--dir", filepath.Join(root, filepath.FromSlash(step.Dir)))
			if result.Status != 0 {
				t.Fatalf("step %s: status %d stdout %s", step.CommandID, result.Status, result.Stdout)
			}
		case "define":
			body(t, d.as(t, d.operator, "dataset", "define", step.Dataset, "--revision", strconv.Itoa(step.Revision), "--file", filepath.Join(root, filepath.FromSlash(step.Recipe))))
		default:
			t.Fatalf("unknown world step kind %q", step.Kind)
		}
	}
	return resume
}

func (d *exampleDeployment) readDelivered(t *testing.T, principal, dataset string, revision int, path string) []byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), filepath.Base(path))
	receipt := asMap(t, body(t, d.as(t, principal, "files", "read", "--dataset", dataset, "--revision", strconv.Itoa(revision), "--path", path, "--out", out)))
	if receipt["dataset"] != dataset || receipt["revision"] != float64(revision) || receipt["path"] != path {
		t.Fatalf("download changed the selected Dataset, revision or path: %#v", receipt)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestQingheExampleWorldPublishesPublicDatasets(t *testing.T) {
	world := readExampleWorld(t)
	d := openExampleDeployment(t, world)
	if resumed := d.publish(t, world, true); resumed != 0 {
		t.Fatalf("a fresh deployment must publish the whole plan, resumed at %d", resumed)
	}
	root := exampleWorldRoot()
	reader := "agent:anyone"

	for _, revision := range []int{1, 2, 3} {
		read := asMap(t, body(t, d.as(t, reader, "read", "urn:kc:object:media-asset/autumn-launch.cut-15s#/definition",
			"--dataset", "autumn-launch-kit", "--revision", strconv.Itoa(revision))))
		definition := asMap(t, read["value"])
		if definition["kind"] != "video" {
			t.Fatalf("revision %d must describe the published video: %#v", revision, definition)
		}
		subtitleSource := "r1"
		if revision > 1 {
			subtitleSource = "r2"
		}
		for _, resource := range []struct{ field, sourceRevision, filename string }{
			{"file", "r1", "cut-15s.mp4"},
			{"cover", "r1", "cover.png"},
			{"subtitles", subtitleSource, "subtitles.zh.vtt"},
		} {
			path, ok := definition[resource.field].(string)
			if !ok || path == "" {
				t.Fatalf("revision %d must explain its %s file: %#v", revision, resource.field, definition)
			}
			expected, err := os.ReadFile(filepath.Join(root, "files", "media", resource.sourceRevision, "assets", "autumn-launch", resource.filename))
			if err != nil {
				t.Fatal(err)
			}
			got := d.readDelivered(t, reader, "autumn-launch-kit", revision, path)
			if !bytes.Equal(got, expected) {
				t.Fatalf("revision %d must deliver the original %s bytes named by its definition: got %d want %d", revision, resource.field, len(got), len(expected))
			}
			if resource.field == "subtitles" && ((revision == 1 && !bytes.Contains(got, []byte("28 元"))) ||
				(revision > 1 && (!bytes.Contains(got, []byte("32 元")) || bytes.Contains(got, []byte("28 元"))))) {
				t.Fatalf("revision %d must retain its published subtitle correction: %s", revision, got)
			}
		}
	}
	if got := string(d.readDelivered(t, reader, "autumn-launch-kit", 3, "assets/autumn-launch/cut-15s.vision.yaml")); !strings.Contains(got, "media-asset/autumn-launch.cut-15s") {
		t.Fatalf("revision 3 must add the vision aspect: %s", got)
	}
	expectCode(t, d.as(t, reader, "files", "read", "--dataset", "autumn-launch-kit", "--revision", "2", "--path", "assets/autumn-launch/cut-15s.vision.yaml"), "OBJECT_REF_UNRESOLVED")

	v1 := string(d.readDelivered(t, reader, "qinghe-analysis", 1, "metrics/shop.gmv/caliber.yaml"))
	v2 := string(d.readDelivered(t, reader, "qinghe-analysis", 2, "metrics/shop.gmv/caliber.yaml"))
	if !strings.Contains(v1, `"revision": "1"`) || !strings.Contains(v2, `"revision": "2"`) || !strings.Contains(v2, "UNPAID") {
		t.Fatalf("analysis revisions must pin different GMV calibers:\nv1=%s\nv2=%s", v1, v2)
	}
	if got := string(d.readDelivered(t, reader, "store-oncall", 1, "services/order-service/runbook.yaml")); !strings.Contains(got, `"revision": "1"`) {
		t.Fatalf("oncall revision 1 must keep runbook v1: %s", got)
	}
	if got := string(d.readDelivered(t, reader, "store-oncall", 2, "services/order-service/runbook.yaml")); !strings.Contains(got, `"revision": "2"`) {
		t.Fatalf("oncall revision 2 must deliver runbook v2: %s", got)
	}
	script, err := os.ReadFile(filepath.Join(root, "files", "store-ops", "r1", "tools", "order-cli", "order-cli.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if got := d.readDelivered(t, reader, "store-oncall", 2, "tools/order-cli/order-cli.sh"); !bytes.Equal(got, script) {
		t.Fatalf("oncall Dataset must deliver the tool script unchanged")
	}

	if got := string(d.readDelivered(t, reader, "qinghe-analysis", 2, "tables/orders.schema.yaml")); !strings.Contains(got, "table/shop.orders") {
		t.Fatalf("analysis Dataset must deliver warehouse table metadata: %s", got)
	}
	product := asMap(t, body(t, d.as(t, reader, "read", testObjectReference(t, "product", "product/qinghe.osmanthus-oolong", ""), "--dataset", "autumn-launch-kit")))
	if product["reference"] != testObjectReference(t, "product", "product/qinghe.osmanthus-oolong", "") {
		t.Fatalf("public Dataset must resolve product objects: %#v", product)
	}
	expectCode(t, d.as(t, reader, "read", testObjectReference(t, "product", "product/qinghe.osmanthus-oolong", ""), "--repo", "product"), "FORBIDDEN")
	expectCode(t, d.as(t, reader, "dataset", "define", "store-oncall", "--revision", "3", "--file", filepath.Join(root, "datasets", "store-oncall.yaml")), "FORBIDDEN")

	// Re-running the whole plan is a no-op: fixed command IDs replay, unchanged
	// raw trees are skipped and accepted revisions are not redefined.
	mediaHead, err := d.stores["media"].Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	if resumed := d.publish(t, world, false); resumed != len(world.Steps) {
		t.Fatalf("a published world must resume past every step, resumed at %d", resumed)
	}
	if after, err := d.stores["media"].Head(snapshot.DefaultRef); err != nil || after != mediaHead {
		t.Fatalf("replay must not recommit raw files: %s -> %s (%v)", mediaHead, after, err)
	}
	if got := string(d.readDelivered(t, reader, "autumn-launch-kit", 1, "assets/autumn-launch/subtitles.zh.vtt")); !strings.Contains(got, "28 元") {
		t.Fatalf("replaying the plan must not move pinned revisions: %s", got)
	}
}
