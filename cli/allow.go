package cli

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"kc/internal/jsonfile"
	"kc/kernel"
)

type AllowRule struct {
	ID        string   `json:"id"`
	Principal string   `json:"principal"`
	Actions   []string `json:"actions,omitempty"`
	Cmds      []string `json:"cmds,omitempty"` // legacy on-read migration only
	Repo      string   `json:"repo,omitempty"`
	Catalog   string   `json:"catalog,omitempty"`
	Ref       string   `json:"ref,omitempty"`
	Object    string   `json:"object,omitempty"`
	Aspect    string   `json:"aspect,omitempty"`
	Workspace string   `json:"workspace,omitempty"`
}

type AllowFile struct {
	Version int         `json:"version"`
	Rules   []AllowRule `json:"rules"`
}

type AllowQuery struct {
	Principal string
	Action    string
	Repo      string
	Catalog   string
	Ref       string
	Object    string
	Aspect    string
	Workspace string
}

const allowVersion = 2

var legacyActions = map[string]string{
	"put": "writer.commit", "remove": "writer.commit", "commit": "writer.commit",
	"propose": "governance.proposal.create", "preview": "governance.preview.create",
	"validate": "governance.validate", "record-validation": "governance.validation.record", "merge": "governance.merge",
	"resolve": "workspace.resolve", "resolve-binding": "knowledge.binding.resolve",
	"read": "knowledge.read", "relations": "knowledge.relations", "describe-schema": "knowledge.schema.read",
	"search": "knowledge.search", "log": "knowledge.history.read", "provenance": "knowledge.provenance",
	"describe-index": "projection.read", "index-sync": "projection.manage", "describe-access": "knowledge.access.describe",
	"diff": "maintenance.object.diff", "checkout": "maintenance.workspace.checkout", "inspect": "maintenance.workspace.inspect",
	"define-workspace": "workspace.manage", "retire-workspace": "workspace.manage", "register": "catalog.repositories.manage",
	"archive-catalog": "catalog.manage", "archive-repo": "catalog.repositories.manage",
	"read-workspace": "workspace.consume", "read-catalog": "catalog.read", "audit": "audit.read",
	"vfs-read": "file.read", "vfs-list": "file.read", "vfs-write": "writer.commit",
}

func allowPath(home string) string { return filepath.Join(home, "allow.json") }

func ReadAllow(home string) (AllowFile, error) {
	file := allowPath(home)
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return AllowFile{Version: allowVersion, Rules: []AllowRule{}}, nil
	}
	var raw AllowFile
	if err := jsonfile.Read(file, &raw); err != nil {
		return AllowFile{}, err
	}
	if raw.Rules == nil {
		raw.Rules = []AllowRule{}
	}
	for i := range raw.Rules {
		if len(raw.Rules[i].Actions) == 0 && len(raw.Rules[i].Cmds) > 0 {
			for _, cmd := range raw.Rules[i].Cmds {
				if action := legacyActions[cmd]; action != "" {
					raw.Rules[i].Actions = appendUnique(raw.Rules[i].Actions, action)
				}
			}
		}
	}
	raw.Version = allowVersion
	return raw, nil
}

func WriteAllow(home string, file AllowFile) error {
	if file.Rules == nil {
		file.Rules = []AllowRule{}
	}
	file.Version = allowVersion
	for i := range file.Rules {
		file.Rules[i].Cmds = nil
	}
	return jsonfile.Write(allowPath(home), file)
}

func appendUnique(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func validateActions(actions []string) error {
	if len(actions) == 0 {
		return fmt.Errorf("grant add requires --action")
	}
	for _, action := range actions {
		if !strings.Contains(action, ".") || strings.ContainsAny(action, " /?#") {
			return fmt.Errorf("invalid semantic action %s", action)
		}
	}
	return nil
}

func matchGlob(pat, value string) bool {
	if pat == "" {
		return true
	}
	ok, err := path.Match(pat, value)
	return err == nil && ok
}

func MatchAllow(rules []AllowRule, q AllowQuery) (AllowRule, bool) {
	for _, rule := range rules {
		if rule.Principal != q.Principal {
			continue
		}
		hit := false
		for _, action := range rule.Actions {
			if actionMatches(action, q.Action) {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		if rule.Repo != "" && rule.Repo != q.Repo {
			continue
		}
		if rule.Catalog != "" && rule.Catalog != q.Catalog {
			continue
		}
		if rule.Ref != "" && rule.Ref != q.Ref {
			continue
		}
		if !matchGlob(rule.Object, q.Object) {
			continue
		}
		if rule.Aspect != "" && rule.Aspect != q.Aspect {
			continue
		}
		if rule.Workspace != "" && rule.Workspace != q.Workspace {
			continue
		}
		return rule, true
	}
	return AllowRule{}, false
}

func PrincipalAllowed(home, as, cmd, repo, catalogID string) bool {
	if as == "" {
		return true
	}
	file, err := ReadAllow(home)
	if err != nil {
		return false
	}
	_, ok := MatchAllow(file.Rules, AllowQuery{
		Principal: as,
		Action:    normalizeAction(cmd),
		Repo:      repo,
		Catalog:   catalogID,
	})
	return ok
}

func actionMatches(granted, requested string) bool {
	if granted == "*" {
		return true
	}
	if granted == requested {
		return true
	}
	if strings.HasSuffix(granted, ".*") && strings.HasPrefix(requested, strings.TrimSuffix(granted, "*")) {
		return true
	}
	if granted == "workspace.consume" {
		return strings.HasPrefix(requested, "knowledge.") || requested == "file.read" || requested == "workspace.resolve"
	}
	return false
}

func normalizeAction(value string) string {
	if strings.Contains(value, ".") {
		return value
	}
	if action := legacyActions[value]; action != "" {
		return action
	}
	return value
}

func ownerBypass(flags map[string]FlagValue) bool {
	return FlagString(flags, "as") == ""
}

// defaultAllowCatalog uses the home's first Catalog when --catalog is omitted.
//
// Args:
//
//	home: workspace directory.
//	catalogID: explicit --catalog, or empty.
//
// Returns:
//
//	catalogID if set; otherwise Catalogs[0].ID; empty if the workspace has no catalog.
func defaultAllowCatalog(home, catalogID string) string {
	if catalogID != "" {
		return catalogID
	}
	if ws, err := ReadHome(home); err == nil && len(ws.Catalogs) > 0 {
		return ws.Catalogs[0].ID
	}
	return ""
}

func authorize(home, command string, flags map[string]FlagValue, observe authorizationObserver) (authErr error) {
	defer observeAuthorizationResult(observe, &authErr)()
	action := normalizeAction(command)
	switch action {
	case "help", "identity.read":
		return nil
	case "writer.commit":
		// commit --workspace routes by path after the body starts,
		if FlagString(flags, "workspace") != "" {
			return nil
		}
	}
	if ownerBypass(flags) {
		return nil
	}
	if strings.HasPrefix(action, "local.") {
		return kernel.Fail(kernel.ErrForbidden, "%s is not allowed to %s", FlagString(flags, "as"), action)
	}
	file, err := ReadAllow(home)
	if err != nil {
		return err
	}
	catalogID := defaultAllowCatalog(home, FlagString(flags, "catalog"))
	q := AllowQuery{
		Principal: FlagString(flags, "as"),
		Action:    action,
		Repo:      FlagString(flags, "repo"),
		Catalog:   catalogID,
		Ref:       FlagString(flags, "ref"),
		Object:    FlagString(flags, "object"),
		Aspect:    FlagString(flags, "aspect"),
		Workspace: workspaceIDOf(flags),
	}
	if _, ok := MatchAllow(file.Rules, q); !ok {
		return kernel.Fail(kernel.ErrForbidden, "%s is not allowed to %s", q.Principal, action)
	}
	return nil
}

// authorizationFlags derives scopes that are already fixed by a stored
// protocol object. The caller should not have to repeat these coordinates,
// and an untrusted repeated value must not be able to change the authorization
// target. Unknown proposal/Preview IDs intentionally stay unscoped and
// therefore fail closed for non-owners before revealing control-plane state.
func authorizationFlags(cx *invocation) map[string]FlagValue {
	if cx == nil || cx.WS == nil {
		return cx.Flags
	}
	derived := make(map[string]FlagValue, len(cx.Flags)+2)
	for name, value := range cx.Flags {
		derived[name] = value
	}
	switch cx.Command {
	case "merge":
		proposal, ok := cx.WS.Control.Proposals[cx.flag("proposal")]
		if !ok {
			return cx.Flags
		}
		derived["repo"] = string(proposal.TargetRepository)
		derived["ref"] = proposal.TargetRef
		return derived
	case "validate", "record-validation":
		preview, ok := cx.WS.Control.Previews[cx.flag("preview")]
		if !ok {
			return cx.Flags
		}
		derived["workspace"] = preview.WorkspaceID
		return derived
	default:
		return cx.Flags
	}
}

func nextRuleID(rules []AllowRule) string {
	return fmt.Sprintf("alw_%d", len(rules)+1)
}

func splitCmds(raw string) []string {
	parts := strings.Split(raw, ",")
	out := []string{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
