package invocation

import (
	"sort"
	"strings"

	"kc/invoke"
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
		base base-rev bulk registry-dir changeset checkouts-dir clear
		client-id cmd command-id commit config contains content continuation credential-file database dir driver dsn eq
		recall
		evidence-id evidence-ref exclude exists expected expected-file file filter-on-behalf-of filter-principal filter-request-id
		direction from from-repo gt gte help home host id if-absent if-digest in include into index
		input input-dataset-version implementation kind layer limit link listen lt lte match match-mode max-hops
		member message metadata min-hops mine missing mode name namespace neq object oauth2-base on on-behalf-of origin-kind out overlay
		operation outcome offset length parent-span-id path path-hint payload phase pin port prefix
		principal produced-at profile projections-dir query read
		ref reference relation-type remove repo repos-dir repository request-id revision rerank-model rerank-timeout
		role root run schema-ref server service-client-id service-client-secret service-principal shape since sort source source-ref source-revision span-id store to token topic
		trace-id until url user value value-source wait dataset-file dataset
		file-source
	`)
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[name] = struct{}{}
	}
	return out
}()

var serveFlags = FlagNames("auth auth-admin auth-hmac-secret auth-url config help listen rerank-model rerank-timeout service-client-id service-client-secret service-principal")
var serveOnlyFlags = FlagNames("auth auth-admin auth-hmac-secret auth-url listen rerank-model rerank-timeout service-client-id service-client-secret service-principal")

func FlagNames(names string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, name := range strings.Fields(names) {
		out[name] = struct{}{}
	}
	return out
}

func RejectUnknownFlags(flags map[string]invoke.FlagValue) error {
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

func RejectServeFlags(flags map[string]invoke.FlagValue) error {
	return RejectFlagsOutside(flags, serveFlags, "kc serve")
}

func RejectServeOnlyFlags(flags map[string]invoke.FlagValue) error {
	for name := range flags {
		if _, serverOnly := serveOnlyFlags[name]; serverOnly {
			return kernel.Fail(kernel.ErrUsageInvalid, "flag --%s is only valid for kc serve", name)
		}
	}
	return nil
}

func RejectFlagsOutside(flags map[string]invoke.FlagValue, allowed map[string]struct{}, scope string) error {
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
