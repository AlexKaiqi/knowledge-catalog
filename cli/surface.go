package cli

import (
	"fmt"
	"strings"
)

// commandSurface is the public CLI contract. Handler is the application
// operation name, usually the public path with spaces replaced by hyphens.
// Knowledge-plane argv dropped the `knowledge` prefix; those handlers keep
// `knowledge-` so HTTP and telemetry stay on the knowledge plane. Handler is
// deliberately not an HTTP route. Action is the stable authorization and
// observability vocabulary.
type commandSurface struct {
	Handler string
	Action  string
}

var cliSurface = map[string]commandSurface{
	"deployment init":                 {"deployment-init", "deployment.init"},
	"deployment status":               {"deployment-status", "deployment.read"},
	"deployment system publish":       {"deployment-system-publish", "deployment.system.publish"},
	"deployment identity migrate":     {"deployment-identity-migrate", "deployment.identity.migrate"},
	"dataset overlay":                 {"dataset-overlay", "dataset.overlay"},
	"attach":                          {"attach", "catalog.repositories.manage"},
	"create":                          {"create", "catalog.repositories.create"},
	"detach":                          {"detach", "catalog.repositories.manage"},
	"show":                            {"show", "catalog.read"},
	"login":                           {"login", "identity.login"},
	"logout":                          {"logout", "identity.logout"},
	"whoami":                          {"whoami", "identity.read"},
	"admission show":                  {"admission-show", "identity.read"},
	"grant add":                       {"grant-add", "admin.grants.manage"},
	"grant remove":                    {"grant-remove", "admin.grants.manage"},
	"grant list":                      {"grant-list", "admin.grants.read"},
	"catalog list":                    {"catalog-list", "catalog.read"},
	"catalog use":                     {"catalog-use", "catalog.read"},
	"catalog audit":                   {"catalog-audit", "catalog.audit.read"},
	"catalog archive":                 {"catalog-archive", "catalog.manage"},
	"dataset define":                  {"dataset-define", "dataset.manage"},
	"dataset clone":                   {"dataset-clone", "file.read"},
	"dataset retire":                  {"dataset-retire", "dataset.manage"},
	"search":                          {"knowledge-search", "knowledge.search"},
	"read":                            {"knowledge-read", "knowledge.read"},
	"resolve":                         {"knowledge-resolve", "knowledge.read"},
	"relations":                       {"knowledge-relations", "knowledge.relations"},
	"traverse":                        {"knowledge-traverse", "knowledge.traverse"},
	"provenance":                      {"knowledge-provenance", "knowledge.provenance"},
	"log":                             {"knowledge-log", "knowledge.history.read"},
	"schema describe":                 {"knowledge-schema-describe", "knowledge.schema.read"},
	"schema list":                     {"knowledge-schema-list", "knowledge.schema.read"},
	"binding show":                    {"knowledge-binding-show", "knowledge.binding.resolve"},
	"access":                          {"knowledge-access", "resource.access"},
	"invoke":                          {"knowledge-invoke", "resource.access"},
	"diff":                            {"diff", "writer.preview"},
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

// incompletePublicFamily reports a known command-family prefix that is not
// itself a leaf.
func incompletePublicFamily(parts []string) (string, bool) {
	if len(parts) == 0 {
		return "", false
	}
	path := strings.Join(parts, " ")
	if _, ok := cliSurface[path]; ok {
		return "", false
	}
	prefix := path + " "
	for command := range cliSurface {
		if strings.HasPrefix(command, prefix) {
			return path, true
		}
	}
	return "", false
}

// knowledgeCLIPath reports a knowledge-plane product command. Argv dropped the
// `knowledge` prefix; the application operation, HTTP route, and semantic
// action keep the knowledge plane.
func knowledgeCLIPath(path string) bool {
	surface, ok := cliSurface[path]
	return ok && strings.HasPrefix(surface.Handler, "knowledge-")
}

func actionOf(command string, flags map[string]FlagValue) string {
	if action := strings.TrimSpace(FlagString(flags, "_action")); action != "" {
		return action
	}
	return "internal." + command
}
