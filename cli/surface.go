package cli

//go:generate go test -run ^TestSystemCLIEntrypoints$ -update-system-entrypoints

import (
	"fmt"
	declarativeindexcli "kc/declarativeindex/cli"
	discoverycli "kc/discovery/cli"
	accesscli "kc/liveaccess/cli"
	objectcli "kc/object/cli"
	snapshotcli "kc/snapshot/cli"
	"strings"
)

// commandSurface is the public CLI contract. Handler is the application
// operation name, usually the public path with spaces replaced by hyphens.
// Object-plane argv dropped the `object` prefix; those handlers keep
// `object-` so HTTP and telemetry stay on the object plane. Handler is
// deliberately not an HTTP route. Action is the stable authorization and
// observability vocabulary.
type commandSurface struct {
	Handler string
	Action  string
}

var cliSurface = composeLiveAccessCLICommands(accesscli.Commands(), composeDeclarativeIndexCLICommands(declarativeindexcli.Commands(), composeDiscoveryCLICommands(discoverycli.Commands(), composeObjectCLICommands(objectcli.Commands(), composeCLICommands(snapshotcli.Commands(), map[string]commandSurface{
	"deployment init":             {"deployment-init", "deployment.init"},
	"deployment status":           {"deployment-status", "deployment.read"},
	"deployment system publish":   {"deployment-system-publish", "deployment.system.publish"},
	"deployment identity migrate": {"deployment-identity-migrate", "deployment.identity.migrate"},
	"create":                      {"create", "repositories.create"},
	"login":                       {"login", "identity.login"},
	"logout":                      {"logout", "identity.logout"},
	"whoami":                      {"whoami", "identity.read"},
	"admission show":              {"admission-show", "identity.read"},

	"diff":                         {"diff", "writer.preview"},
	"writer commit":                {"writer-commit", "writer.commit"},
	"writer head":                  {"writer-head", "writer.preview"},
	"writer receipt":               {"writer-receipt", "writer.receipt.read"},
	"governance proposal create":   {"governance-proposal-create", "governance.proposal.create"},
	"governance proposal merge":    {"governance-proposal-merge", "governance.merge"},
	"governance preview create":    {"governance-preview-create", "governance.preview.create"},
	"governance preview validate":  {"governance-preview-validate", "governance.validate"},
	"governance validation record": {"governance-validation-record", "governance.validation.record"},

	"operations hook add":        {"operations-hook-add", "operations.hooks.manage"},
	"operations hook list":       {"operations-hook-list", "operations.hooks.read"},
	"operations hook remove":     {"operations-hook-remove", "operations.hooks.manage"},
	"operations gate add":        {"operations-gate-add", "operations.gates.manage"},
	"operations gate list":       {"operations-gate-list", "operations.gates.read"},
	"operations gate remove":     {"operations-gate-remove", "operations.gates.manage"},
	"operations audit access":    {"operations-audit-access", "audit.read"},
	"operations audit trace":     {"operations-audit-trace", "audit.read"},
	"operations audit hitmap":    {"operations-audit-hitmap", "audit.read"},
	"operations feedback record": {"operations-feedback-record", "feedback.write"},
})))))

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

// objectCLIPath reports an object-plane product command. Argv dropped the
// `object` prefix; the application operation, HTTP route, and semantic
// action keep the object plane.
func objectCLIPath(path string) bool {
	surface, ok := cliSurface[path]
	return ok && strings.HasPrefix(surface.Handler, "object-")
}

func composeCLICommands(entries []snapshotcli.Command, host map[string]commandSurface) map[string]commandSurface {
	return composeModuleCommands(entries, host)
}
