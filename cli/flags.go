package cli

import (
	"sort"
	"strings"

	"kc/kernel"
)

// knownFlags is the transport-wide vocabulary. Individual verb validation
// still owns combinations and required operands, but an unrecognized spelling
// must never be silently accepted by either argv or HTTP JSON. Internal stamps
// use a leading underscore and are added only after ingress.
var knownFlags = func() map[string]struct{} {
	names := strings.Fields(`
		action activity-ref actor-ref algorithm-hash algorithm-model algorithm-spec
		app-name as aspect auth auth-admin auth-hmac-secret auth-login auth-provider auth-subject auth-url
		base base-rev candidate catalog catalogs-dir changeset checkouts-dir clear
		client-id cmd command-id commit content continuation database dir driver dsn eq
		evidence-id evidence-ref exclude exists expected file filter-on-behalf-of filter-principal
		direction from from-repo gt gte help home host id if-absent if-digest in include index
		input input-workspace-version kind layer limit link listen lt lte match match-mode
		member message missing mode namespace neq object oauth2-base on on-behalf-of origin-kind out
		operation outcome parent-span-id path path-hint payload phase pin port prefix preview
		principal produced-at profile projections-dir proposal proposal-id query read
		ref relation-type remove repo repos-dir repository request-id require resource-access-url revision rerank-model rerank-timeout
		role root run schema-ref server service-client-id service-client-secret service-principal since sort source source-ref span-id suite target to token topic
		trace-id until url user validation value value-source wait workspace
	`)
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[name] = struct{}{}
	}
	return out
}()

var serveFlags = flagNames("auth auth-admin auth-hmac-secret auth-url help home listen resource-access-url rerank-model rerank-timeout service-client-id service-client-secret service-principal")
var serveOnlyFlags = flagNames("auth auth-admin auth-hmac-secret auth-url listen resource-access-url rerank-model rerank-timeout service-client-id service-client-secret service-principal")

func flagNames(names string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, name := range strings.Fields(names) {
		out[name] = struct{}{}
	}
	return out
}

func rejectUnknownFlags(flags map[string]FlagValue) error {
	unknown := make([]string, 0)
	for name := range flags {
		if strings.HasPrefix(name, "_") {
			continue
		}
		if _, ok := knownFlags[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return kernel.Fail(kernel.ErrUsageInvalid, "unknown flag --%s", unknown[0])
}

func rejectServeFlags(flags map[string]FlagValue) error {
	return rejectFlagsOutside(flags, serveFlags, "kc serve")
}

func rejectServeOnlyFlags(flags map[string]FlagValue) error {
	for name := range flags {
		if _, serverOnly := serveOnlyFlags[name]; serverOnly {
			return kernel.Fail(kernel.ErrUsageInvalid, "flag --%s is only valid for kc serve", name)
		}
	}
	return nil
}

func rejectFlagsOutside(flags map[string]FlagValue, allowed map[string]struct{}, scope string) error {
	invalid := make([]string, 0)
	for name := range flags {
		if strings.HasPrefix(name, "_") {
			continue
		}
		if _, ok := allowed[name]; !ok {
			invalid = append(invalid, name)
		}
	}
	if len(invalid) == 0 {
		return nil
	}
	sort.Strings(invalid)
	return kernel.Fail(kernel.ErrUsageInvalid, "flag --%s is not valid for %s", invalid[0], scope)
}
