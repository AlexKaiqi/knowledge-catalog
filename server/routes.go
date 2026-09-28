// Package server owns the service access surface: the method+pattern closed
// set, its handler registrations, authorization enforcement, and telemetry.
// CAP-ORG-01 B4 lands the contract first; the cli service handlers move in
// B5-B7 (API-01: CLI and HTTP keep running the same typed application
// executors). See docs/reviewed/capability-organization.md §3.2.
package server

import "sort"

// Route is one formal entry of the service surface: an HTTP method plus a
// ServeMux pattern. The set is closed and must stay in exact agreement with
// httpsurface.Patterns(); routes_anchor_test.go enforces the reconciliation
// in both directions.
type Route struct {
	Method  string
	Pattern string
}

var routes = []Route{
	{Method: "DELETE", Pattern: "/admin/v1/grants/{grant}"},
	{Method: "DELETE", Pattern: "/catalog/v1/catalogs/{catalog}/repositories/{repository}"},
	{Method: "DELETE", Pattern: "/catalog/v1/repositories/{repository}/shares/{share}"},
	{Method: "DELETE", Pattern: "/operations/v1/gates/{gate}"},
	{Method: "DELETE", Pattern: "/operations/v1/hooks/{hook}"},
	{Method: "GET", Pattern: "/admin/v1/grants"},
	{Method: "GET", Pattern: "/assets/console.js"},
	{Method: "GET", Pattern: "/assets/repository.js"},
	{Method: "GET", Pattern: "/catalog/v1/catalogs"},
	{Method: "GET", Pattern: "/catalog/v1/catalogs/{catalog}"},
	{Method: "GET", Pattern: "/catalog/v1/catalogs/{catalog}/audit"},
	{Method: "GET", Pattern: "/catalog/v1/catalogs/{catalog}/datasets"},
	{Method: "GET", Pattern: "/catalog/v1/catalogs/{catalog}/datasets/{dataset}"},
	{Method: "GET", Pattern: "/catalog/v1/catalogs/{catalog}/repositories"},
	{Method: "GET", Pattern: "/catalog/v1/repositories"},
	{Method: "GET", Pattern: "/catalog/v1/repositories/{repository}"},
	{Method: "GET", Pattern: "/catalog/v1/repositories/{repository}/connection"},
	{Method: "GET", Pattern: "/catalog/v1/repositories/{repository}/shares"},
	{Method: "GET", Pattern: "/console"},
	{Method: "GET", Pattern: "/health"},
	{Method: "GET", Pattern: "/identity/v1/admission"},
	{Method: "GET", Pattern: "/identity/v1/auth"},
	{Method: "GET", Pattern: "/identity/v1/whoami"},
	{Method: "GET", Pattern: "/livez"},
	{Method: "GET", Pattern: "/metrics"},
	{Method: "GET", Pattern: "/operations/v1/gates"},
	{Method: "GET", Pattern: "/operations/v1/hooks"},
	{Method: "GET", Pattern: "/operations/v1/stores"},
	{Method: "GET", Pattern: "/readyz"},
	{Method: "GET", Pattern: "/readyz/{surface}"},
	{Method: "GET", Pattern: "/repositories/{repository}"},
	{Method: "GET", Pattern: "/ui/"},
	{Method: "GET", Pattern: "/writer/v1/receipts/{command}"},
	{Method: "GET", Pattern: "/writer/v1/repositories/{repository}/head"},
	{Method: "POST", Pattern: "/admin/v1/grants"},
	{Method: "POST", Pattern: "/catalog/v1/catalogs/{catalog}/archive"},
	{Method: "POST", Pattern: "/catalog/v1/catalogs/{catalog}/datasets"},
	{Method: "POST", Pattern: "/catalog/v1/catalogs/{catalog}/datasets/{dataset}/check"},
	{Method: "POST", Pattern: "/catalog/v1/catalogs/{catalog}/datasets/{dataset}/resolve"},
	{Method: "POST", Pattern: "/catalog/v1/catalogs/{catalog}/datasets/{dataset}/retire"},
	{Method: "POST", Pattern: "/catalog/v1/catalogs/{catalog}/datasets:resolve"},
	{Method: "POST", Pattern: "/catalog/v1/catalogs/{catalog}/repositories"},
	{Method: "POST", Pattern: "/catalog/v1/catalogs/{catalog}/repositories/{repository}/archive"},
	{Method: "POST", Pattern: "/catalog/v1/catalogs/{catalog}/repositories:connect"},
	{Method: "POST", Pattern: "/catalog/v1/catalogs/{catalog}/repositories:create"},
	{Method: "POST", Pattern: "/catalog/v1/repositories"},
	{Method: "POST", Pattern: "/catalog/v1/repositories/{repository}/connection:check"},
	{Method: "POST", Pattern: "/catalog/v1/repositories/{repository}/connection:rotate"},
	{Method: "POST", Pattern: "/catalog/v1/repositories/{repository}/shares"},
	{Method: "POST", Pattern: "/dataset-files/v1/file:read"},
	{Method: "POST", Pattern: "/dataset-files/v1/mounts:list"},
	{Method: "POST", Pattern: "/dataset-files/v1/tree:list"},
	{Method: "POST", Pattern: "/governance/v1/previews"},
	{Method: "POST", Pattern: "/governance/v1/previews:validate"},
	{Method: "POST", Pattern: "/governance/v1/proposals"},
	{Method: "POST", Pattern: "/governance/v1/proposals:merge"},
	{Method: "POST", Pattern: "/governance/v1/validations"},
	{Method: "POST", Pattern: "/identity/v1/authorize"},
	{Method: "POST", Pattern: "/identity/v1/authorize:poll"},
	{Method: "POST", Pattern: "/identity/v1/token"},
	{Method: "POST", Pattern: "/knowledge/v1/bindings:resolve"},
	{Method: "POST", Pattern: "/knowledge/v1/log:query"},
	{Method: "POST", Pattern: "/knowledge/v1/objects:read"},
	{Method: "POST", Pattern: "/knowledge/v1/objects:resolve"},
	{Method: "POST", Pattern: "/knowledge/v1/provenance:describe"},
	{Method: "POST", Pattern: "/knowledge/v1/relations:query"},
	{Method: "POST", Pattern: "/knowledge/v1/rerank"},
	{Method: "POST", Pattern: "/knowledge/v1/resources:access"},
	{Method: "POST", Pattern: "/knowledge/v1/schemas:describe"},
	{Method: "POST", Pattern: "/knowledge/v1/schemas:list"},
	{Method: "POST", Pattern: "/knowledge/v1/search"},
	{Method: "POST", Pattern: "/knowledge/v1/search:rerank"},
	{Method: "POST", Pattern: "/knowledge/v1/traverse:query"},
	{Method: "POST", Pattern: "/operations/v1/access-log:query"},
	{Method: "POST", Pattern: "/operations/v1/access-specs:describe"},
	{Method: "POST", Pattern: "/operations/v1/feedback"},
	{Method: "POST", Pattern: "/operations/v1/gates"},
	{Method: "POST", Pattern: "/operations/v1/hitmap:query"},
	{Method: "POST", Pattern: "/operations/v1/hooks"},
	{Method: "POST", Pattern: "/operations/v1/projections:describe"},
	{Method: "POST", Pattern: "/operations/v1/projections:notice"},
	{Method: "POST", Pattern: "/operations/v1/projections:sync"},
	{Method: "POST", Pattern: "/operations/v1/refine-log:query"},
	{Method: "POST", Pattern: "/operations/v1/rerank-training:query"},
	{Method: "POST", Pattern: "/operations/v1/retrieval-log:query"},
	{Method: "POST", Pattern: "/operations/v1/retrieval-training:query"},
	{Method: "POST", Pattern: "/operations/v1/traces:query"},
	{Method: "POST", Pattern: "/writer/v1/repositories/{repository}/commits"},
}

// Routes returns the closed route set in stable (pattern, method) order. The
// service access surface has exactly these entries and no others.
func Routes() []Route {
	out := append([]Route{}, routes...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pattern != out[j].Pattern {
			return out[i].Pattern < out[j].Pattern
		}
		return out[i].Method < out[j].Method
	})
	return out
}
