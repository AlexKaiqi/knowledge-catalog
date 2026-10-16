package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	indexcli "kc/declarativeindex/capability"
	declarative "kc/declarativeindex/plugin"
	retrieval "kc/discovery"
	discoveryclient "kc/discovery/client"
	consumerplugin "kc/discovery/plugin"
	index "kc/discovery/projection"
	"kc/internal/telemetry"
	"kc/internal/testkit"
	inv "kc/invocation"
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/snapshot"
)

type capabilityHTTPTransport struct {
	client    *http.Client
	principal string
}

func (tr capabilityHTTPTransport) Do(ctx context.Context, _ string, request *http.Request) (*http.Response, error) {
	request = request.WithContext(ctx)
	request.Header.Set("X-Kc-As", tr.principal)
	return tr.client.Do(request)
}

func TestPluginCapabilityHTTPUsesInstalledManifestAndReadOnlyWriterTrial(t *testing.T) {
	s := testkit.NewSetup(t, "kr://trial/public")
	declaration := testkit.IndexMember("name", "text", []string{"text"}, "schema/service#/fields/name")
	_, err := s.Writer.Commit("seed-trial", objectmodel.CommitChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/service"}, Value: map[string]any{"entity": "Service", "fields": map[string]any{"name": map[string]any{"type": "string"}}}}, declaration,
	}})
	if err != nil {
		t.Fatal(err)
	}
	private := testkit.NewSetup(t, "kr://trial/private-holder")
	if _, err := private.Writer.Commit("private-schema", objectmodel.CommitChangeSet{TargetRepository: private.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: private.RootCommitID, ExpectedTargetCommit: private.RootCommitID, Operations: testkit.PutEntity("schema/hidden", map[string]any{"entity": "Hidden", "fields": map[string]any{"secret": map[string]any{"type": "string"}}}, "")}); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Add(private.Repo.Snapshot()); err != nil {
		t.Fatal(err)
	}
	before := testkit.MustHead(t, s.Repo, snapshot.DefaultRef)
	dir := t.TempDir()
	if err := inv.WriteAllow(dir, inv.AllowFile{Version: inv.AllowVersion, Rules: []inv.AllowRule{
		{ID: "inventory-reader", Principal: "reader", Actions: []string{"inventory.read"}},
		{ID: "trial", Principal: "reader", Repo: string(s.RepositoryID), Actions: []string{"object.read"}},
		{ID: "inventory", Principal: "inventory-only", Actions: []string{"inventory.read"}},
	}}); err != nil {
		t.Fatal(err)
	}
	var engineCalls atomic.Int64
	manifest := indexcli.DefaultManifest("embed/test-hash@1")
	entry := declarative.Entry(declarative.Options{Directory: filepath.Join(dir, "projection"), Manifest: &manifest,
		Engine: func(string) index.EngineOpener {
			return func(string, kernel.RepositoryID) (index.Engine, error) {
				engineCalls.Add(1)
				return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "trial must not open an engine")
			}
		}, Lookup: func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			return s.Reader.Require(id, kernel.ErrObjectRefUnresolved)
		},
	})
	plugins, err := consumerplugin.OpenIndexPlugins(consumerplugin.PluginsConfig{"declarative-index": {Instances: map[string]consumerplugin.PluginInstanceConfig{"configured": {Catalog: declarative.Catalog}}}}, consumerplugin.PluginScope{Repositories: []kernel.RepositoryID{s.RepositoryID}}, map[string]consumerplugin.IndexPluginEntry{"declarative-index": entry})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = plugins.Close() })
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "capability-contract"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })
	ws := &Home{Dir: dir, Store: s.Store, Reader: s.Reader, Writer: s.Writer, Index: plugins, Plugins: plugins}
	facade := &httpFacade{home: dir, readHome: ws, runtime: runtime, options: HTTPServerOptions{AuthMode: "local"}}
	mux := http.NewServeMux()
	facade.registerServiceRoutes(mux)
	network := httptest.NewServer(mux)
	t.Cleanup(network.Close)
	remote, err := discoveryclient.New(discoveryclient.Config{BaseURL: network.URL, Transport: capabilityHTTPTransport{network.Client(), "reader"}})
	if err != nil {
		t.Fatal(err)
	}
	var help indexcli.PluginAffordance
	if err := remote.PluginHelp(context.Background(), "declarative-index", discoveryclient.RequestOptions{}, &help); err != nil || !strings.Contains(help.Usage, "embed/test-hash@1") {
		t.Fatalf("help did not use the installed Entry's configuration: %#v %v", help, err)
	}
	var manifests []retrieval.IndexInstanceManifest
	if err := remote.PluginCapabilities(context.Background(), "declarative-index", discoveryclient.RequestOptions{}, &manifests); err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 1 || manifests[0].Instance != "configured" || manifests[0].Manifest.Digest != manifest.Digest || manifests[0].Manifest.Builds[5].Options[0].Values[0] != "embed/test-hash@1" {
		t.Fatalf("describe substituted a compiled default for the installed instance: %#v", manifests)
	}
	// Physical engine configuration is only ever named as not declarable.
	declarable := manifests[0].Manifest
	declarable.Closed = nil
	exposed, _ := json.Marshal(declarable)
	usage := []string{}
	for _, line := range strings.Split(help.Usage, "\n") {
		if !strings.Contains(line, "不开放") {
			usage = append(usage, line)
		}
	}
	for _, physical := range []string{"mapping", "script", "shard", "replica", "pipeline"} {
		if strings.Contains(strings.ToLower(string(exposed)), physical) || strings.Contains(strings.ToLower(strings.Join(usage, "\n")), physical) {
			t.Fatalf("describe/help exposes %s as declarable: manifest=%s usage=%s", physical, exposed, help.Usage)
		}
		if !slices.Contains(manifests[0].Manifest.Closed, physical) && !slices.Contains(manifests[0].Manifest.Closed, physical+"s") {
			t.Fatalf("manifest does not state %s as not declarable: %#v", physical, manifests[0].Manifest.Closed)
		}
	}
	trialMembers := func(t *testing.T, operations ...objectmodel.Operation) []retrieval.DeclarationTrialMember {
		t.Helper()
		documents := make([]retrieval.DeclarationDocument, len(operations))
		for i, op := range operations {
			content, err := repofile.Serialize(op.Address, "", op.SchemaRef, nil, op.Value)
			if err != nil {
				t.Fatal(err)
			}
			documents[i] = retrieval.DeclarationDocument{Path: "index/" + op.Address.MemberKey + ".yaml", Content: content}
		}
		var out retrieval.DeclarationTrialResult
		err := remote.TrialDeclarations(context.Background(), "declarative-index", retrieval.DeclarationTrialRequest{Repository: s.RepositoryID, Commit: before, Documents: documents}, discoveryclient.RequestOptions{}, &out)
		if err != nil || len(out.Members) != len(operations) || out.Commit != before {
			t.Fatalf("trial: %#v %v", out, err)
		}
		return out.Members
	}
	trial := func(t *testing.T, op objectmodel.Operation) retrieval.DeclarationTrialMember {
		t.Helper()
		return trialMembers(t, op)[0]
	}
	member := trial(t, declaration)
	if member.Status != "supported" || member.Rebuild == nil || *member.Rebuild {
		t.Fatalf("unchanged member: %#v", member)
	}
	t.Run("mixed-members", func(t *testing.T) {
		bad := testkit.IndexMember("bad", "text", []string{"text"}, "schema/service#/fields/absent")
		members := trialMembers(t, declaration, bad)
		if members[0].Member != declaration.Address || members[0].Status != "supported" || members[0].Reason != "" || members[0].Instance != "configured" {
			t.Fatalf("valid member inherited a sibling's failure: %#v", members)
		}
		_, _, writerErr := s.Writer.CheckIndexDeclaration(objectmodel.ChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
			BaseCommit: before, ExpectedTargetCommit: before, Operations: []objectmodel.Operation{bad}}, bad.Address)
		if members[1].Member != bad.Address || members[1].Status != retrieval.DeclarationInvalid || writerErr == nil || members[1].Reason != writerErr.Error() {
			t.Fatalf("invalid member did not preserve its own Writer verdict: %#v writer=%v", members, writerErr)
		}
	})
	t.Run("inherited-schema", func(t *testing.T) {
		inherited := declaration
		inherited.SchemaRef = ""
		_, _, writerErr := s.Writer.CheckIndexDeclaration(objectmodel.ChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
			BaseCommit: before, ExpectedTargetCommit: before, Operations: []objectmodel.Operation{inherited}}, inherited.Address)
		if writerErr != nil {
			t.Fatalf("Writer refused inherited Schema: %v", writerErr)
		}
		member := trial(t, inherited)
		if member.Status != "supported" || member.Reason != "" || member.Rebuild == nil || *member.Rebuild {
			t.Fatalf("HTTP trial differs from Writer on the same update file: %#v", member)
		}
	})
	declaration.Value.(map[string]any)["inputs"].([]any)[0].(map[string]any)["boost"] = 3
	member = trial(t, declaration)
	if member.Status != retrieval.DeclarationUnsupported || !strings.Contains(member.Reason, "scoring weights") || member.Rebuild == nil || *member.Rebuild {
		t.Fatalf("scoring-only change must retain no-rebuild meaning while actual support remains unavailable: %#v", member)
	}
	declaration.Value.(map[string]any)["build"].(map[string]any)["analyzer"] = "not-installed"
	member = trial(t, declaration)
	if member.Status != retrieval.DeclarationUnsupported || !strings.Contains(member.Reason, "not-installed") {
		t.Fatalf("unsupported option was accepted: %#v", member)
	}
	delete(declaration.Value.(map[string]any)["build"].(map[string]any), "analyzer")
	declaration.Value.(map[string]any)["inputs"].([]any)[0].(map[string]any)["from"] = "schema/service#/fields/absent"
	member = trial(t, declaration)
	_, writerErr := s.Writer.Commit("rejected-trial", objectmodel.CommitChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: before, ExpectedTargetCommit: before, Operations: []objectmodel.Operation{declaration}})
	if member.Status != retrieval.DeclarationInvalid || writerErr == nil || member.Reason != writerErr.Error() {
		t.Fatalf("trial/Writer mismatch: %#v writer=%v", member, writerErr)
	}
	declaration.Value.(map[string]any)["inputs"].([]any)[0].(map[string]any)["from"] = "schema/service#/fields/name"
	for _, kind := range []string{"engine", "mapping"} {
		body := declaration.Value.(map[string]any)
		if kind == "engine" {
			body["engine"] = "opensearch"
		} else {
			body["build"].(map[string]any)["mapping"] = map[string]any{"type": "text"}
		}
		member = trial(t, declaration)
		_, writerErr = s.Writer.Commit("rejected-"+kind, objectmodel.CommitChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: before, ExpectedTargetCommit: before, Operations: []objectmodel.Operation{declaration}})
		if member.Status != retrieval.DeclarationInvalid || writerErr == nil || member.Reason != writerErr.Error() || member.Code != string(kernel.CodeOf(writerErr)) {
			t.Fatalf("%s trial/Writer mismatch: %#v writer=%v", kind, member, writerErr)
		}
		delete(body, "engine")
		delete(body["build"].(map[string]any), "mapping")
	}
	declaration.Value.(map[string]any)["inputs"].([]any)[0].(map[string]any)["from"] = "schema/hidden#/fields/secret"
	content, _ := repofile.Serialize(declaration.Address, "", declaration.SchemaRef, nil, declaration.Value)
	input, _ := json.Marshal(retrieval.DeclarationTrialRequest{Repository: s.RepositoryID, Documents: []retrieval.DeclarationDocument{{Path: "name.yaml", Content: content}}})
	for _, tc := range []struct {
		principal, body, query string
		status                 int
	}{
		{"reader", string(input), "", 403}, {"inventory-only", string(input), "", 403}, {"", string(input), "", 401},
		{"reader", `{"repository":"kr://trial/public","unknown":true}`, "", 400},
		{"reader", string(input), "?repo=elsewhere", 400}, {"reader", `{`, "", 400},
	} {
		request, _ := http.NewRequest(http.MethodPost, network.URL+"/object/v1/plugins/declarative-index/declarations:trial"+tc.query, strings.NewReader(tc.body))
		request.Header.Set("X-Kc-As", tc.principal)
		response, err := network.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if response.StatusCode != tc.status {
			t.Fatalf("refusal status=%d want=%d body=%s", response.StatusCode, tc.status, body)
		}
		if response.StatusCode == 403 && strings.Contains(string(body), "private-holder") {
			t.Fatalf("private Schema holder leaked: %s", body)
		}
	}
	if facade.workersStarted || plugins.Running() || engineCalls.Load() != 0 || testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != before {
		t.Fatal("describe/trial activated maintenance or mutated authority/index")
	}
	// Legacy lazy handlers must fail clearly rather than activate providers to
	// manufacture an explanation. Production serving already owns its root.
	cold := &httpFacade{home: dir, runtime: runtime, options: HTTPServerOptions{AuthMode: "local"}}
	request := httptest.NewRequest(http.MethodGet, "/object/v1/plugins/declarative-index/capabilities", nil)
	request.SetPathValue("name", "declarative-index")
	request.Header.Set("X-Kc-As", "reader")
	response := httptest.NewRecorder()
	cold.pluginCapabilities(response, request)
	if !strings.Contains(response.Body.String(), "PRECONDITION_FAILED") || cold.readHome != nil || cold.workersStarted {
		t.Fatalf("cold explanation silently initialized runtime: %s", response.Body.String())
	}
}
