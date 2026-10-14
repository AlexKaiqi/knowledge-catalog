package cli_test

import (
	"context"
	"encoding/json"
	snapshotclient "kc/snapshot/client"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	kcclient "kc/client"
	serv "kc/server"
	"kc/snapshot/dataset"
)

// DATASET-02（mvp-acceptance.md 服务版采用）：真实部署入口上的发布管线恢复。
// 三个断言：准备阶段失败保留已接受版本；替换实例后数据集状态、授权与回执
// 存活且同一 pin 继续服务；内容派生 command-id 幂等重放，有效 v2 在恢复后
// 正常发布。投影准备失败（索引）不在本用例范围，此处走
// 发布管线的准备阶段（来源冻结）失败。
func TestDatasetPublishingRecoveryKeepsAcceptedVersion(t *testing.T) {
	cfg, path := declaredDeployment(t, true)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	var server *httptest.Server
	var handler http.Handler
	start := func() {
		var err error
		handler, err = serv.HTTPHandlerFromConfig(path, serv.HTTPServerOptions{})
		if err != nil {
			t.Fatal(err)
		}
		server = httptest.NewServer(handler)
	}
	stop := func() {
		server.Close()
		if closer, ok := handler.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	start()
	defer func() {
		if server != nil {
			stop()
		}
	}()
	call := func(args ...string) kcRunResult { return kcRemote(t, server.URL, "agent:operator", args...) }
	readerClient := func() *kcclient.Client {
		c, err := kcclient.New(kcclient.Config{BaseURL: server.URL})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Login(context.Background(), kcclient.LoginRequest{Identity: kcclient.Identity{Principal: "agent:reader"}}); err != nil {
			t.Fatal(err)
		}
		return c
	}
	coordinate := func(pin *dataset.ResolvedDataset) snapshotclient.WorkspaceFileCoordinate {
		c := snapshotclient.WorkspaceFileCoordinate{Dataset: "incident", View: "repository"}
		if pin != nil {
			raw, err := json.Marshal(pin)
			if err != nil {
				t.Fatal(err)
			}
			c.Pin = raw
		}
		return c
	}
	// Writer commits wrap plain files with object frontmatter (provenance
	// embeds the source path), so the serving assertion is byte-stability: the
	// accepted version must return IDENTICAL bytes across a failed prepare,
	// instance replacement and a later version advance.
	baseline := map[string]string{}
	assertDelivered := func(c *kcclient.Client, pin *dataset.ResolvedDataset, file, marker string) {
		t.Helper()
		var out snapshotclient.WorkspaceFileReadResponse
		if err := c.WorkspaceFilesService().Read(context.Background(), snapshotclient.WorkspaceFileReadRequest{
			WorkspaceFileCoordinate: coordinate(pin), Path: file,
		}, snapshotclient.RequestOptions{}, &out); err != nil {
			t.Fatalf("delivered read %s: %v", file, err)
		}
		got := string(out.Content)
		if !strings.Contains(got, marker) {
			t.Fatalf("delivered bytes at %s lost the marker %q: %q", file, marker, got)
		}
		if first, ok := baseline[file]; ok {
			if first != got {
				t.Fatalf("delivered bytes drifted at %s:\nfirst=%q\nnow=%q", file, first, got)
			}
		} else {
			baseline[file] = got
		}
	}

	body(t, call("attach", "--repo", "kr://recover/source"))
	materials := t.TempDir()
	writeSourceTree(t, materials, "note/recover.md", "durable\n")
	body(t, call("writer", "commit", "--command-id", "recovery-write", "--repo", "kr://recover/source", "--dir", materials))
	body(t, call("dataset", "define", "incident", "--revision", "1", "--source", "kr://recover/source@notes@note"))
	body(t, call("grant", "add", "--principal", "agent:reader", "--action", "dataset.resolve", "--dataset", "incident"))
	body(t, call("grant", "add", "--principal", "agent:reader", "--action", "file.read", "--dataset", "incident"))
	reader := readerClient()
	accepted := &dataset.ResolvedDataset{}
	{
		var mounts snapshotclient.WorkspaceFileMountsResponse
		if err := reader.WorkspaceFilesService().Mounts(context.Background(), snapshotclient.WorkspaceFileMountsRequest{
			WorkspaceFileCoordinate: coordinate(nil),
		}, snapshotclient.RequestOptions{}, &mounts); err != nil {
			t.Fatal(err)
		}
		if len(mounts.Mounts) != 1 || mounts.Pin.Revision != 1 {
			t.Fatalf("accepted v1 must serve one mount: %#v", mounts)
		}
		*accepted = mounts.Pin
	}
	assertDelivered(reader, accepted, "notes/recover.md", "durable")

	// 准备阶段失败：来源未在 Catalog 登记 → 冻结失败拒绝，已接受版本继续服务。
	expectCode(t, call("dataset", "define", "incident", "--revision", "2", "--source", "kr://recover/unconfigured@notes@note"), "DATASET_INVALID")
	{
		var mounts snapshotclient.WorkspaceFileMountsResponse
		if err := reader.WorkspaceFilesService().Mounts(context.Background(), snapshotclient.WorkspaceFileMountsRequest{
			WorkspaceFileCoordinate: coordinate(nil),
		}, snapshotclient.RequestOptions{}, &mounts); err != nil {
			t.Fatal(err)
		}
		if mounts.Pin.Revision != 1 || mounts.Pin.PinID != accepted.PinID {
			t.Fatalf("failed prepare must keep the accepted version: %#v", mounts.Pin)
		}
	}
	assertDelivered(reader, accepted, "notes/recover.md", "durable")

	// 替换实例：耐久状态保留，派生缓存清空。
	oldHead := body(t, call("writer", "head", "--repo", "kr://recover/source"))
	catalogState := body(t, call("show"))
	grants := body(t, call("grant", "list"))
	stop()
	server = nil
	preserved := map[string][]byte{}
	for _, name := range []string{"allow.json", "writer.db", "system.jsonl", "access.jsonl"} {
		raw, err := os.ReadFile(filepath.Join(cfg.StateDir, name))
		if err != nil {
			t.Fatal(err)
		}
		preserved[name] = raw
	}
	if err := os.RemoveAll(cfg.CacheDir); err != nil {
		t.Fatal(err)
	}
	cfg.CacheDir = filepath.Join(filepath.Dir(cfg.CacheDir), "replacement-instance")
	writeDeployment(t, path, cfg)
	start()
	for name, want := range preserved {
		got, err := os.ReadFile(filepath.Join(cfg.StateDir, name))
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("startup rewrote %s: %v", name, err)
		}
	}
	if got := body(t, call("writer", "head", "--repo", "kr://recover/source")); !reflect.DeepEqual(got, oldHead) {
		t.Fatal("recovery changed object HEAD")
	}
	if got := body(t, call("show")); !reflect.DeepEqual(got, catalogState) {
		t.Fatal("Catalog/dataset state lost on replacement")
	}
	if got := body(t, call("grant", "list")); !reflect.DeepEqual(got, grants) {
		t.Fatal("grants lost on replacement")
	}

	// 同一 pin 继续服务；重投递 v1 被版本单调守卫拒绝（dataset U5 回滚守卫），版本保持 v1。
	recovered := readerClient()
	assertDelivered(recovered, accepted, "notes/recover.md", "durable")
	expectCode(t, call("dataset", "define", "incident", "--revision", "1", "--source", "kr://recover/source@notes@note"), "NON_FAST_FORWARD")
	{
		var mounts snapshotclient.WorkspaceFileMountsResponse
		if err := recovered.WorkspaceFilesService().Mounts(context.Background(), snapshotclient.WorkspaceFileMountsRequest{
			WorkspaceFileCoordinate: coordinate(nil),
		}, snapshotclient.RequestOptions{}, &mounts); err != nil {
			t.Fatal(err)
		}
		if mounts.Pin.Revision != 1 || mounts.Pin.PinID != accepted.PinID {
			t.Fatalf("refused re-define must keep the accepted version: %#v", mounts.Pin)
		}
	}

	// 恢复后生命周期继续：新写入 + --base-rev 前进到 v2，旧 pin 不漂移。
	writeSourceTree(t, materials, "note/second.md", "second\n")
	body(t, call("writer", "commit", "--command-id", "recovery-write-2", "--repo", "kr://recover/source", "--dir", materials))
	body(t, call("dataset", "define", "incident", "--revision", "2", "--source", "kr://recover/source@notes@note", "--base-rev", string(accepted.Repositories["kr://recover/source"])))
	var after snapshotclient.WorkspaceFileMountsResponse
	if err := recovered.WorkspaceFilesService().Mounts(context.Background(), snapshotclient.WorkspaceFileMountsRequest{
		WorkspaceFileCoordinate: coordinate(nil),
	}, snapshotclient.RequestOptions{}, &after); err != nil {
		t.Fatal(err)
	}
	if after.Pin.Revision != 2 {
		t.Fatalf("valid v2 must publish after recovery: %#v", after.Pin)
	}
	assertDelivered(recovered, &after.Pin, "notes/second.md", "second")
	assertDelivered(recovered, accepted, "notes/recover.md", "durable")
}

func writeSourceTree(t *testing.T, root, file, content string) {
	t.Helper()
	path := filepath.Join(root, file)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
