package cli

import (
	"fmt"
	"strings"
)

// commandSurface is the public CLI contract. Handler is the application
// operation name: the public path with spaces replaced by hyphens. It is
// deliberately not an HTTP route. Action is the stable authorization and
// observability vocabulary.
type commandSurface struct {
	Handler string
	Action  string
}

var cliSurface = map[string]commandSurface{
	"local init":                      {"local-init", "local.init"},
	"local status":                    {"local-status", "local.status"},
	"local catalog attach":            {"local-catalog-attach", "local.catalog.attach"},
	"local repository attach":         {"local-repository-attach", "local.repository.attach"},
	"local store show":                {"local-store-show", "local.store.read"},
	"local store set":                 {"local-store-set", "local.store.manage"},
	"local grant bootstrap":           {"local-grant-bootstrap", "local.grant.bootstrap"},
	"local system publish":            {"local-system-publish", "local.system.publish"},
	"local workspace overlay":         {"local-workspace-overlay", "local.workspace.overlay"},
	"login":                           {"login", "identity.login"},
	"logout":                          {"logout", "identity.logout"},
	"whoami":                          {"whoami", "identity.read"},
	"admin grant add":                 {"admin-grant-add", "admin.grants.manage"},
	"admin grant remove":              {"admin-grant-remove", "admin.grants.manage"},
	"admin grant list":                {"admin-grant-list", "admin.grants.read"},
	"catalog list":                    {"catalog-list", "catalog.read"},
	"catalog show":                    {"catalog-show", "catalog.read"},
	"catalog audit":                   {"catalog-audit", "catalog.audit.read"},
	"catalog archive":                 {"catalog-archive", "catalog.manage"},
	"catalog repo list":               {"catalog-repo-list", "catalog.read"},
	"catalog repo register":           {"catalog-repo-register", "catalog.repositories.manage"},
	"catalog repo archive":            {"catalog-repo-archive", "catalog.repositories.manage"},
	"workspace list":                  {"workspace-list", "catalog.read"},
	"workspace show":                  {"workspace-show", "catalog.read"},
	"workspace define":                {"workspace-define", "workspace.manage"},
	"workspace retire":                {"workspace-retire", "workspace.manage"},
	"workspace pin":                   {"workspace-pin", "workspace.resolve"},
	"workspace check":                 {"workspace-check", "workspace.resolve"},
	"knowledge search":                {"knowledge-search", "knowledge.search"},
	"knowledge read":                  {"knowledge-read", "knowledge.read"},
	"knowledge resolve":               {"knowledge-resolve", "knowledge.read"},
	"knowledge relations":             {"knowledge-relations", "knowledge.relations"},
	"knowledge provenance":            {"knowledge-provenance", "knowledge.provenance"},
	"knowledge log":                   {"knowledge-log", "knowledge.history.read"},
	"knowledge schema describe":       {"knowledge-schema-describe", "knowledge.schema.read"},
	"knowledge schema list":           {"knowledge-schema-list", "knowledge.schema.read"},
	"knowledge binding show":          {"knowledge-binding-show", "knowledge.binding.resolve"},
	"knowledge access":                {"knowledge-access", "resource.access"},
	"knowledge invoke":                {"knowledge-invoke", "resource.access"},
	"pack":                            {"pack", "writer.preview"},
	"writer put":                      {"writer-put", "writer.commit"},
	"writer remove":                   {"writer-remove", "writer.commit"},
	"writer commit":                   {"writer-commit", "writer.commit"},
	"writer head":                     {"writer-head", "writer.preview"},
	"writer receipt":                  {"writer-receipt", "writer.receipt.read"},
	"governance proposal create":      {"governance-proposal-create", "governance.proposal.create"},
	"governance proposal merge":       {"governance-proposal-merge", "governance.merge"},
	"governance preview create":       {"governance-preview-create", "governance.preview.create"},
	"governance preview validate":     {"governance-preview-validate", "governance.validate"},
	"governance validation record":    {"governance-validation-record", "governance.validation.record"},
	"operations projection describe":  {"operations-projection-describe", "projection.read"},
	"operations projection sync":      {"operations-projection-sync", "projection.manage"},
	"operations projection notice":    {"operations-projection-notice", "projection.manage"},
	"operations access-spec describe": {"operations-access-spec-describe", "knowledge.access.describe"},
	"operations hook add":             {"operations-hook-add", "operations.hooks.manage"},
	"operations hook list":            {"operations-hook-list", "operations.hooks.read"},
	"operations hook remove":          {"operations-hook-remove", "operations.hooks.manage"},
	"operations gate add":             {"operations-gate-add", "operations.gates.manage"},
	"operations gate list":            {"operations-gate-list", "operations.gates.read"},
	"operations gate remove":          {"operations-gate-remove", "operations.gates.manage"},
	"operations audit access":         {"operations-audit-access", "audit.read"},
	"operations audit trace":          {"operations-audit-trace", "audit.read"},
	"operations audit hitmap":         {"operations-audit-hitmap", "audit.read"},
	"operations feedback record":      {"operations-feedback-record", "feedback.write"},
}

func resolveCLICommand(first string, args []string) (commandSurface, []string, error) {
	parts := append([]string{first}, args...)
	for n := len(parts); n > 0; n-- {
		path := strings.Join(parts[:n], " ")
		if surface, ok := cliSurface[path]; ok {
			return surface, parts[n:], nil
		}
	}
	return commandSurface{}, nil, fmt.Errorf("unknown command %s", strings.Join(parts, " "))
}

func actionOf(command string, flags map[string]FlagValue) string {
	if action := strings.TrimSpace(FlagString(flags, "_action")); action != "" {
		return action
	}
	return "internal." + command
}
