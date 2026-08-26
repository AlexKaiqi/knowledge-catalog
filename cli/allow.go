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
	Cmds      []string `json:"cmds"`
	Repo      string   `json:"repo,omitempty"`
	Catalog   string   `json:"catalog,omitempty"`
	Ref       string   `json:"ref,omitempty"`
	Object    string   `json:"object,omitempty"`
	Aspect    string   `json:"aspect,omitempty"`
	Workspace string   `json:"workspace,omitempty"`
}

type AllowFile struct {
	Rules []AllowRule `json:"rules"`
}

type AllowQuery struct {
	Principal string
	Cmd       string
	Repo      string
	Catalog   string
	Ref       string
	Object    string
	Aspect    string
	Workspace string
}

var writeFaces = [][]string{
	{"put", "remove", "commit"},
	{"propose"},
	{"merge"},
	{"resolve", "resolve-binding", "read", "list", "relations", "describe-schema", "search", "describe-index", "index-sync", "log", "diff", "provenance"},
	{"define-workspace", "describe-access", "retire-workspace", "register", "archive-catalog"},
	{"preview", "validate", "record-validation"},
	{"read-workspace", "read-catalog", "audit"},
	{"archive-repo"},
}

func allowPath(home string) string { return filepath.Join(home, "allow.json") }

func ReadAllow(home string) (AllowFile, error) {
	file := allowPath(home)
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return AllowFile{Rules: []AllowRule{}}, nil
	}
	var raw AllowFile
	if err := jsonfile.Read(file, &raw); err != nil {
		return AllowFile{}, err
	}
	if raw.Rules == nil {
		raw.Rules = []AllowRule{}
	}
	return raw, nil
}

func WriteAllow(home string, file AllowFile) error {
	if file.Rules == nil {
		file.Rules = []AllowRule{}
	}
	return jsonfile.Write(allowPath(home), file)
}

func faceOf(cmd string) int {
	for i, face := range writeFaces {
		for _, item := range face {
			if item == cmd {
				return i
			}
		}
	}
	return -1
}

func validateCmds(cmds []string) error {
	if len(cmds) == 0 {
		return fmt.Errorf("allow requires --cmd")
	}
	face := faceOf(cmds[0])
	if face < 0 {
		return fmt.Errorf("unknown command in --cmd: %s", cmds[0])
	}
	for _, cmd := range cmds[1:] {
		if faceOf(cmd) != face {
			return fmt.Errorf("--cmd cannot mix write surfaces: %s and %s", cmds[0], cmd)
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
		for _, cmd := range rule.Cmds {
			if cmd == q.Cmd {
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
		Cmd:       cmd,
		Repo:      repo,
		Catalog:   catalogID,
	})
	return ok
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

func authorize(home, command string, flags map[string]FlagValue) error {
	switch command {
	case "help", "init", "catalog-add", "repo-add", "status", "allow", "revoke", "allowed", "whoami", "ingest", "receipt",
		"hook-add", "hook-ls", "hook-rm", "gate-add", "gate-ls", "gate-rm":
		return nil
	case "vfs-write":
		// The target repository is only known after RouteMount runs inside
		// the body (the caller names --workspace + --path, not --repo): the same
		// reason `ingest` defers its own check to the `commit` that follows
		// it. verbVFSWrite calls authorizeRoutedWrite itself once routing
		// resolves the real target.
		return nil
	case "commit":
		// commit --workspace routes by path after the body starts,
		// same deferred check as vfs-write.
		if FlagString(flags, "workspace") != "" {
			return nil
		}
	}
	if ownerBypass(flags) {
		return nil
	}
	file, err := ReadAllow(home)
	if err != nil {
		return err
	}
	catalogID := defaultAllowCatalog(home, FlagString(flags, "catalog"))
	q := AllowQuery{
		Principal: FlagString(flags, "as"),
		Cmd:       command,
		Repo:      FlagString(flags, "repo"),
		Catalog:   catalogID,
		Ref:       FlagString(flags, "ref"),
		Object:    FlagString(flags, "object"),
		Aspect:    FlagString(flags, "aspect"),
		Workspace: workspaceIDOf(flags),
	}
	if _, ok := MatchAllow(file.Rules, q); !ok {
		return kernel.Fail(kernel.ErrForbidden, "%s is not allowed to %s", q.Principal, command)
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
