package service

import (
	"context"
	"sync/atomic"
	"testing"

	common "kc/discovery/plugin"
	"kc/kernel"
	"kc/liveaccess"
	interfaces "kc/liveaccess/handler"
	objectmodel "kc/object"
)

type pluginInterfaceFunc func(context.Context, interfaces.InterfaceCall) (interfaces.InterfaceResult, error)

func (pluginInterfaceFunc) ValidateImplementation(context.Context, interfaces.ResolvedInterface) error {
	return nil
}
func (f pluginInterfaceFunc) Execute(ctx context.Context, call interfaces.InterfaceCall) (interfaces.InterfaceResult, error) {
	return f(ctx, call)
}

func pluginImplementation(repository kernel.RepositoryID) interfaces.ResolvedInterface {
	return interfaces.ResolvedInterface{Repository: repository, Contract: objectmodel.PinnedObjectRef{ObjectRef: objectmodel.ObjectRef{Repository: "contract-repo", Object: "interface/custom"}, Commit: "release"}, ContractDigest: "contract-digest", ImplementationSchemaStructureDigest: "schema-structure", Implementation: map[string]any{"runtime": "http", "protocol": testProtocol}}
}

func pluginInterfaceRegistry(t *testing.T, handler interfaces.InterfaceHandler) *interfaces.InterfaceRegistry {
	t.Helper()
	implementation := pluginImplementation("repo-a")
	registry := interfaces.NewInterfaceRegistry()
	if err := registry.Register(interfaces.InterfaceRegistration{Contract: implementation.Contract, ContractDigest: implementation.ContractDigest, ImplementationSchemaStructureDigest: implementation.ImplementationSchemaStructureDigest, HandlerRevision: "custom/v1", Handler: handler}); err != nil {
		t.Fatal(err)
	}
	return registry
}

func TestAccessPluginsInterfaceUsesOnlyMappedInstanceAndFixedSupport(t *testing.T) {
	var calls atomic.Int32
	configs := pluginConfigs(map[string]common.PluginInstanceConfig{"one": {Protocol: testProtocol, Runtimes: []string{"mysql-shop"}}, "two": {Protocol: testProtocol, Runtimes: []string{"http"}}})
	plugins, err := OpenPlugins(configs, []kernel.RepositoryID{"repo-a", "repo-b"}, map[string]PluginEntry{"liveaccess": {Open: func(scope common.PluginScope) (PluginContribution, error) {
		registry := pluginInterfaceRegistry(t, pluginInterfaceFunc(func(_ context.Context, call interfaces.InterfaceCall) (interfaces.InterfaceResult, error) {
			calls.Add(1)
			return interfaces.InterfaceResult{Value: scope.Instance + "/" + string(call.Implementation.Repository)}, nil
		}))
		return PluginContribution{Lookup: pluginLookupFunc(func(context.Context, liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
			return liveaccess.StateObservation{}, nil
		}), Interfaces: registry}, nil
	}}})
	if err != nil {
		t.Fatal(err)
	}
	defer plugins.Close()
	implementation := pluginImplementation("repo-b")
	registry, err := plugins.InterfaceRegistryFor(implementation)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := registry.Resolve(implementation)
	if err != nil {
		t.Fatal(err)
	}
	result, err := registration.Handler.Execute(context.Background(), interfaces.InterfaceCall{Implementation: implementation, Operation: "read"})
	if err != nil || result.Value != "two/repo-b" {
		t.Fatalf("wrong source contribution: %#v %v", result, err)
	}
	foreign := pluginImplementation("repo-a")
	if _, err := registration.Handler.Execute(context.Background(), interfaces.InterfaceCall{Implementation: foreign}); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("private source handler escaped Repo scope: %v", err)
	}
	implementation.ContractDigest = "changed-contract"
	if _, err := plugins.InterfaceRegistryFor(implementation); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("unmatched release fell back: %v", err)
	}
	unmapped := pluginImplementation("repo-b")
	unmapped.Implementation = map[string]any{"runtime": "http", "protocol": "resource-access/v2"}
	if _, err := plugins.InterfaceRegistryFor(unmapped); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || calls.Load() != 1 {
		t.Fatalf("unmapped protocol borrowed another instance's handler: %v calls=%d", err, calls.Load())
	}
}
