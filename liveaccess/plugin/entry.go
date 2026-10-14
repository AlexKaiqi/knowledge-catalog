// Package plugin assembles the LiveAccess request contribution for a fixed
// Repo scope. It owns the actual adapter construction and static explanation.
package plugin

import (
	"context"
	"net/http"
	"reflect"
	"time"

	common "kc/discovery/adoption"
	"kc/interfaces"
	"kc/kernel"
	"kc/liveaccess"
	accesscli "kc/liveaccess/cli"
	"kc/liveaccess/service"
)

type Options struct {
	// OpenLookup may construct an embedding adapter and immediately register
	// owned resources through scope.Own. Nil uses resource-access/v1 HTTP.
	OpenLookup        func(common.PluginScope) (liveaccess.StateLookup, error)
	Interfaces        *interfaces.InterfaceRegistry
	CredentialOrigins []string
}

func Entry(options Options) service.PluginEntry {
	affordance := accesscli.Affordance()
	return service.PluginEntry{Affordance: affordance, Open: func(scope common.PluginScope) (service.PluginContribution, error) {
		var lookup liveaccess.StateLookup
		if options.OpenLookup != nil {
			var err error
			lookup, err = options.OpenLookup(scope)
			if err != nil {
				return service.PluginContribution{}, err
			}
		} else {
			transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
			if defaults, ok := http.DefaultTransport.(*http.Transport); ok {
				transport = defaults.Clone()
			}
			scope.Own(func() error { transport.CloseIdleConnections(); return nil })
			var err error
			lookup, err = liveaccess.NewHTTPStateLookupWithOptions(liveaccess.HTTPStateLookupOptions{Client: &http.Client{Transport: transport, Timeout: 30 * time.Second}, CredentialOrigins: options.CredentialOrigins})
			if err != nil {
				return service.PluginContribution{}, err
			}
		}
		if lookup == nil || nilLookup(lookup) {
			return service.PluginContribution{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "LiveAccess adapter is unavailable")
		}
		accessor, _ := lookup.(liveaccess.ResourceOperationAccessor)
		allowed := map[kernel.RepositoryID]bool{}
		for _, repo := range scope.Repositories {
			allowed[repo] = true
		}
		contribution := service.PluginContribution{Lookup: scopedLookup{lookup: lookup, allowed: allowed}, Interfaces: options.Interfaces.Snapshot()}
		if accessor != nil {
			contribution.Accessor = scopedAccessor{accessor: accessor, allowed: allowed}
		}
		return contribution, nil
	}}
}

func nilLookup(lookup liveaccess.StateLookup) bool {
	value := reflect.ValueOf(lookup)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

type scopedLookup struct {
	lookup  liveaccess.StateLookup
	allowed map[kernel.RepositoryID]bool
}

func (s scopedLookup) LookupState(ctx context.Context, request liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
	if !s.allowed[request.Binding.Repository] {
		return liveaccess.StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Repository is outside this plugin instance")
	}
	return s.lookup.LookupState(ctx, request)
}

type scopedAccessor struct {
	accessor liveaccess.ResourceOperationAccessor
	allowed  map[kernel.RepositoryID]bool
}

func (s scopedAccessor) AccessResource(ctx context.Context, request liveaccess.ResourceOperationRequest) (any, error) {
	if !s.allowed[request.Descriptor.Repository] {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Repository is outside this plugin instance")
	}
	return s.accessor.AccessResource(ctx, request)
}
