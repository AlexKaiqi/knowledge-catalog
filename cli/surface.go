package cli

import (
	"fmt"
	"strings"
)

// commandSurface is the public CLI contract. Handler is an application
// operation name internal to this package; it is deliberately not an HTTP
// route. Action is the stable authorization and observability vocabulary.
type commandSurface struct {
	Handler string
	Action  string
}

var cliSurface = map[string]commandSurface{
	"local init":                     {"init", "local.init"},
	"local status":                   {"status", "local.status"},
	"local catalog attach":           {"catalog-add", "local.catalog.attach"},
	"local repository attach":        {"repo-add", "local.repository.attach"},
	"local store show":               {"store-ls", "local.store.read"},
	"local store set":                {"store-set", "local.store.manage"},
	"local grant bootstrap":          {"bootstrap-grant", "local.grant.bootstrap"},
	"local system publish":           {"system-publish", "local.system.publish"},
	"local workspace overlay":        {"overlay", "local.workspace.overlay"},
	"login":                          {"login", "identity.login"},
	"logout":                         {"logout", "identity.logout"},
	"identity whoami":                {"whoami", "identity.read"},
	"admin grant add":                {"allow", "admin.grants.manage"},
	"admin grant remove":             {"revoke", "admin.grants.manage"},
	"admin grant list":               {"allowed", "admin.grants.read"},
	"catalog list":                   {"catalog-list", "catalog.read"},
	"catalog show":                   {"catalog-show", "catalog.read"},
	"catalog audit":                  {"audit", "catalog.audit.read"},
	"catalog archive":                {"archive-catalog", "catalog.manage"},
	"catalog repository list":        {"catalog-repositories", "catalog.read"},
	"catalog repository register":    {"register", "catalog.repositories.manage"},
	"catalog repository archive":     {"archive-repo", "catalog.repositories.manage"},
	"catalog workspace list":         {"catalog-workspaces", "catalog.read"},
	"catalog workspace show":         {"catalog-workspace", "catalog.read"},
	"catalog workspace define":       {"define-workspace", "workspace.manage"},
	"catalog workspace retire":       {"retire-workspace", "workspace.manage"},
	"catalog workspace resolve":      {"resolve", "workspace.resolve"},
	"catalog workspace check":        {"check-workspace", "workspace.resolve"},
	"knowledge search":               {"search", "knowledge.search"},
	"knowledge read":                 {"read", "knowledge.read"},
	"knowledge resolve":              {"resolve-object", "knowledge.read"},
	"knowledge relations":            {"relations", "knowledge.relations"},
	"knowledge provenance":           {"provenance", "knowledge.provenance"},
	"knowledge log":                  {"log", "knowledge.history.read"},
	"knowledge schema describe":      {"describe-schema", "knowledge.schema.read"},
	"knowledge schema browse":        {"browse-schemas", "knowledge.schema.read"},
	"knowledge binding resolve":      {"resolve-binding", "knowledge.binding.resolve"},
	"writer put":                     {"put", "writer.commit"},
	"writer remove":                  {"remove", "writer.commit"},
	"writer commit":                  {"commit", "writer.commit"},
	"writer ingest":                  {"ingest", "writer.preview"},
	"writer head":                    {"writer-head", "writer.preview"},
	"writer receipt":                 {"receipt", "writer.receipt.read"},
	"governance proposal create":     {"propose", "governance.proposal.create"},
	"governance proposal merge":      {"merge", "governance.merge"},
	"governance preview create":      {"preview", "governance.preview.create"},
	"governance preview validate":    {"validate", "governance.validate"},
	"governance validation record":   {"record-validation", "governance.validation.record"},
	"operations projection describe": {"describe-index", "projection.read"},
	"operations projection sync":     {"index-sync", "projection.manage"},
	"operations access describe":     {"describe-access", "knowledge.access.describe"},
	"operations hook add":            {"hook-add", "operations.hooks.manage"},
	"operations hook list":           {"hook-ls", "operations.hooks.read"},
	"operations hook remove":         {"hook-rm", "operations.hooks.manage"},
	"operations gate add":            {"gate-add", "operations.gates.manage"},
	"operations gate list":           {"gate-ls", "operations.gates.read"},
	"operations gate remove":         {"gate-rm", "operations.gates.manage"},
	"operations audit access":        {"access-log", "audit.read"},
	"operations audit trace":         {"trace", "audit.read"},
	"operations audit hitmap":        {"hitmap", "audit.read"},
	"operations feedback record":     {"record-feedback", "feedback.write"},
	"resource access":                {"resource-access", "resource.access"},
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
