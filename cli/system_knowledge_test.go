package cli_test

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	apphome "kc/home"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	serv "kc/server"
	"kc/snapshot"
)

func TestSystemKnowledgeCanBeReadThroughAuthenticatedServer(t *testing.T) {
	_, config := declaredDeployment(t, false)
	body(t, deploymentCommand(t, "deployment", "init", "--config", config))
	handler, err := serv.HTTPHandlerFromConfig(config, serv.HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer handler.(interface{ Close() error }).Close()
	server := httptest.NewServer(handler)
	defer server.Close()
	for _, tc := range []struct{ object, aspect, needle string }{
		{"kc/system", "readme", "使用指南"},
		{"kc/concept/object", "definition", "具有独立身份"},
		{"kc/interface/writer", "definition", "单仓变更"},
		{"kc/interface/cli", "definition", "分组命令入口"},
		{"kc/mechanism/governance", "definition", "合并检查（Gate）"},
		{"kc/mechanism/hook", "definition", "授权先成立"},
		{"kc/mechanism/dynamic-access", "definition", "观察（Observation）"},
		{"kc/guide/first-publication", "guide", "成功证据"},
	} {
		read := asMap(t, body(t, kcRemote(t, server.URL, "agent:reader", "read", "--repo", string(knowledge.SystemRepositoryID), "--object", tc.object, "--aspect", tc.aspect)))
		value := asMap(t, read["value"])
		if read["commit"] == "" || !strings.Contains(value["body"].(string), tc.needle) {
			t.Fatalf("system knowledge not delivered with a fixed basis: %#v", read)
		}
	}
	read := asMap(t, body(t, kcRemote(t, server.URL, "agent:reader", "read", "--repo", string(knowledge.SystemRepositoryID), "--object", "kc/capability/read-knowledge")))
	value := asMap(t, read["value"])
	for _, aspect := range []string{"definition", "usage", "entrypoints"} {
		if value[aspect] == nil {
			t.Errorf("assembled capability lacks %s: %#v", aspect, value)
		}
	}
}

func TestDeploymentRejectsDriftedSystemDescription(t *testing.T) {
	cfg, config := declaredDeployment(t, false)
	fake := testkit.NewLakeFSFake(t)
	physicalID := fake.NewRepo()
	cfg.Repositories = []apphome.RepositoryBinding{{ID: string(knowledge.SystemRepositoryID), Driver: "lakefs", DSN: fake.DSN(physicalID)}}
	writeDeployment(t, config, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", config))
	body(t, deploymentCommand(t, "deployment", "system", "publish", "--config", config))
	opened, err := apphome.OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	store, _ := opened.Store.Get(knowledge.SystemRepositoryID)
	tree, ok := snapshot.TreeReaderOf(store)
	if !ok {
		t.Fatal("declared system binding lacks a file authority")
	}
	commit, _ := store.Head(snapshot.DefaultRef)
	rel := "concepts/object/definition.md"
	raw, err := tree.ReadFile(rel, commit)
	if err != nil {
		t.Fatal(err)
	}
	altered := bytes.Replace(raw, []byte("具有独立身份"), []byte("没有独立身份"), 1)
	if bytes.Equal(raw, altered) {
		t.Fatal("tamper signal did not alter the system description")
	}
	_, err = fake.CommitFiles(physicalID, commit, []snapshot.TreeChange{{Path: rel, Content: altered}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := serv.HTTPHandlerFromConfig(config, serv.HTTPServerOptions{}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("deployment accepted drifted self-description: %v", err)
	}
}
