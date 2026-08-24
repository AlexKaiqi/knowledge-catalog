package connectorhost

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestResourceAccessUsesActiveGenerationAndRecordsAgentTrace(t *testing.T) {
	repo := copyTestRepo(t)
	manifestPath := filepath.Join(repo, "connectors", "file-observer", "connector.yaml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest = append(manifest, []byte(`  access:
    protocol: resource-access/v1
    command: [sh, access.sh]
    operations: [status, lookup]
    timeout: 5s
`)...)
	if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	accessPath := filepath.Join(repo, "connectors", "file-observer", "access.sh")
	if err := os.WriteFile(accessPath, []byte("#!/bin/sh\ncat >/dev/null\nprintf '%s\\n' '{\"cut\":\"revision-2\",\"records\":[{\"traceId\":\"trace-1\"}]}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := HostConfig{RepoPath: repo, KCURL: "http://kc.invalid"}
	if err := store.SaveConfig(config); err != nil {
		t.Fatal(err)
	}
	host := NewHost(store, config, KCClient{BaseURL: config.KCURL})
	state, err := host.Activate(context.Background(), "file-observer")
	if err != nil {
		t.Fatal(err)
	}
	request := AccessRequest{
		Descriptor: ResourceDescriptorCoordinate{
			ObjectID: "resource/traces/payment-api", Repository: "kr://demo/public/facts", Commit: "abc123",
		},
		Runtime: "file-observer", Protocol: "resource-access/v1", Operation: "lookup",
		Input: json.RawMessage(`{"traceId":"trace-1"}`),
	}
	identity := AccessIdentity{Principal: "consumer", Agent: "dsh-loom", Session: "session-7", RequestID: "ask-1"}
	response, err := host.Access(context.Background(), request, identity)
	if err != nil {
		t.Fatal(err)
	}
	if response.TraceID == "" || response.Generation != state.ActiveGeneration || response.Descriptor.Commit != "abc123" {
		t.Fatalf("unexpected access response: %#v", response)
	}
	var result map[string]any
	if err := json.Unmarshal(response.Result, &result); err != nil || result["cut"] != "revision-2" {
		t.Fatalf("unexpected access result: %s %v", response.Result, err)
	}
	traces, err := store.AccessTraces(10)
	if err != nil || len(traces) != 1 {
		t.Fatalf("access traces: %#v %v", traces, err)
	}
	trace := traces[0]
	if trace.Identity.Principal != "consumer" || trace.Identity.Agent != "dsh-loom" || trace.Identity.Session != "session-7" || trace.Descriptor.Commit != "abc123" || trace.ResultDigest == "" {
		t.Fatalf("trace lost closed-loop coordinates: %#v", trace)
	}

	_, err = host.Access(context.Background(), AccessRequest{
		Descriptor: request.Descriptor, Runtime: request.Runtime, Protocol: request.Protocol, Operation: "delete",
	}, AccessIdentity{Principal: "consumer", RequestID: "ask-2"})
	if err == nil {
		t.Fatal("undeclared access operation should fail")
	}
	traces, _ = store.AccessTraces(10)
	if len(traces) != 2 || traces[0].Error == "" {
		t.Fatalf("failed access was not traced: %#v", traces)
	}
}
