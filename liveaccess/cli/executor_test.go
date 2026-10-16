package cli_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"kc/kernel"
	accesscli "kc/liveaccess/cli"
	objectclient "kc/object/client"
)

type recordingClient struct {
	bindings []accesscli.BindingRequest
	accesses []accesscli.AccessRequest
	options  accesscli.RequestOptions
}

func (c *recordingClient) ResolveBinding(_ context.Context, request accesscli.BindingRequest, options accesscli.RequestOptions, output any) error {
	c.bindings = append(c.bindings, request)
	c.options = options
	*output.(*any) = map[string]any{"binding": request.Object}
	return nil
}

func (c *recordingClient) AccessResource(_ context.Context, request accesscli.AccessRequest, options accesscli.RequestOptions, output any) error {
	c.accesses = append(c.accesses, request)
	c.options = options
	*output.(*any) = map[string]any{"value": request.Object}
	return nil
}

func TestLiveAccessCLIUsesFixedBasisAndAdoptsTransportResult(t *testing.T) {
	remote := &recordingClient{}
	scope := objectclient.ObjectScope{Dataset: "selected", Pin: json.RawMessage(`{"revision":3}`)}
	executor := accesscli.New(remote, accesscli.Context{Scope: scope})
	options := accesscli.RequestOptions{RequestID: "request-3"}
	flags := map[string]any{"object": "service/orders", "aspect": "health", "member": "current"}
	output, err := executor.Execute(context.Background(), "binding show", flags, options)
	if err != nil || !reflect.DeepEqual(output, map[string]any{"binding": "service/orders"}) || !reflect.DeepEqual(remote.bindings[0].ObjectScope, scope) {
		t.Fatalf("fixed binding request/result changed: %#v %#v %v", remote.bindings, output, err)
	}
	output, err = executor.Execute(context.Background(), "access", flags, options)
	if err != nil || !reflect.DeepEqual(output, map[string]any{"value": "service/orders"}) || remote.accesses[0].Aspect != "health" || remote.accesses[0].Member != "current" || !reflect.DeepEqual(remote.accesses[0].ObjectScope, scope) {
		t.Fatalf("access request/result changed: %#v %#v %v", remote.accesses, output, err)
	}
	_, err = executor.Execute(context.Background(), "invoke", map[string]any{"repo": "source-repo", "commit": "fixed", "object": "service/orders", "implementation": "live", "operation": "read", "input": `{"amount":1234567890123456789}`}, options)
	last := remote.accesses[1]
	if err != nil || last.Repository != "source-repo" || last.Commit != "fixed" || last.Dataset != "" || len(last.Pin) != 0 || last.Implementation != "live" || last.Operation != "read" || string(last.Input) != `{"amount":1234567890123456789}` || remote.options.RequestID != "request-3" {
		t.Fatalf("invoke selected basis/input changed: %#v %v", last, err)
	}
}

func TestLiveAccessCLIHelpIsStaticAndOwnedDeclarationsAreIndependent(t *testing.T) {
	executor := accesscli.New(nil, accesscli.Context{})
	for _, entry := range accesscli.Commands() {
		value, err := executor.Execute(context.Background(), entry.Path, map[string]any{"help": true}, accesscli.RequestOptions{})
		if err != nil || !strings.Contains(value.(string), entry.Usage) {
			t.Fatalf("nil-client help failed for %s: %#v %v", entry.Path, value, err)
		}
	}
	entries := accesscli.Commands()
	entries[0].Flags[0] = "mutated"
	entries[0].Groups[0] = "mutated"
	if fresh := accesscli.Commands(); fresh[0].Flags[0] == "mutated" || fresh[0].Groups[0] == "mutated" {
		t.Fatal("host mutated module declarations")
	}
	for _, expected := range []string{"LookupState", "StateObservation", "null", "凭证", "审计", "requestId", "原位置", "可信", "operations audit access"} {
		if !strings.Contains(accesscli.Affordance().Usage, expected) {
			t.Fatalf("static explanation missing %s", expected)
		}
	}
}

func TestLiveAccessCLIHelpUsesDeploymentAndUnitDeclarations(t *testing.T) {
	usage := accesscli.Affordance().Usage
	for _, expected := range []string{"部署装配", "不绑定仓或 Dataset", "方面或成员", "读到相应单元时生效", "descriptorRef", "runtime", "protocol", "仅 identity 为 shared", "按索引声明", "按查询条件补判"} {
		if !strings.Contains(usage, expected) {
			t.Errorf("plugin help omits declaration model %q", expected)
		}
	}
	for _, entry := range accesscli.Commands() {
		usage += "\n" + entry.Usage
	}
	for _, retired := range []string{"来源 Repo 配置本插件", "Dataset 复用来源配置", "origin 来自所选 Domain Schema", "schema describe 核对 origin"} {
		if strings.Contains(usage, retired) {
			t.Errorf("LiveAccess help retains retired model %q", retired)
		}
	}
}

func TestLiveAccessCLIRejectsInvalidRequestsBeforeTransport(t *testing.T) {
	remote := &recordingClient{}
	executor := accesscli.New(remote, accesscli.Context{})
	for _, test := range []struct {
		path  string
		flags map[string]any
	}{
		{"access", map[string]any{"repo": "repo", "object": "object", "aspect": "health", "operation": "write"}},
		{"invoke", map[string]any{"repo": "repo", "object": "object", "operation": "read", "input": "{}", "aspect": "health"}},
		{"invoke", map[string]any{"repo": "repo", "object": "object", "operation": "read", "input": "{} {}"}},
		{"binding show", map[string]any{"repo": "repo", "dataset": "set", "object": "object", "aspect": "health"}},
		{"access", map[string]any{"dataset": "set", "commit": "foreign", "object": "object", "aspect": "health"}},
		{"access", map[string]any{"repo": "repo", "object": "object"}},
		{"access", map[string]any{"repo": "repo", "object": "object", "aspect": []string{"one", "two"}}},
	} {
		if _, err := executor.Execute(context.Background(), test.path, test.flags, accesscli.RequestOptions{}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("invalid %s accepted: %#v %v", test.path, test.flags, err)
		}
	}
	if len(remote.bindings) != 0 || len(remote.accesses) != 0 {
		t.Fatal("invalid request reached source transport")
	}
}

func TestLiveAccessCLIShapeKeepsNullAndObservationBasis(t *testing.T) {
	value := map[string]any{"observations": []any{map[string]any{"value": nil, "basis": map[string]any{"sourceRevision": "observed"}}, map[string]any{"value": "other"}}, "bindings": []any{map[string]any{"schemaRef": "schema/health", "address": map[string]any{"objectId": "service/orders", "aspectName": "health"}}}}
	shaped := accesscli.Shape("access", value)
	if !reflect.DeepEqual(shaped, map[string]any{"value": nil, "basis": map[string]any{"sourceRevision": "observed"}, "schemaRef": "schema/health", "objectId": "service/orders", "aspectName": "health"}) {
		t.Fatalf("access presentation changed: %#v", shaped)
	}
	if !reflect.DeepEqual(accesscli.Shape("invoke", value), value) {
		t.Fatal("invoke result unexpectedly reshaped")
	}
}
