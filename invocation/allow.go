package invocation

import (
	"fmt"
	"kc/snapshot"
	"os"
	"path/filepath"
	"strings"

	kchome "kc/home"
	"kc/internal/jsonfile"
	"kc/invoke"
	"kc/kernel"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
)

type AllowRule = access.Rule

type AllowFile struct {
	Version int         `json:"version"`
	Rules   []AllowRule `json:"rules"`
	// InitialGrants records completed policy applications, independently of
	// revocable rules. A provisioning retry must never restore a revoked rule.
	InitialGrants map[string]kernel.Digest    `json:"initialGrants,omitempty"`
	Admissions    map[string]AdmissionReceipt `json:"admissions,omitempty"`
}

type AllowQuery = access.Query

const AllowVersion = 2

// commandActions maps a bare command name to the semantic action it requires.
var commandActions = map[string]string{
	"put": "writer.commit", "remove": "writer.commit", "commit": "writer.commit",
	"branch": "writer.branch", "merge": "writer.merge",
	"resolve": "dataset.resolve", "resolve-object": "object.read", "resolve-binding": "object.binding.resolve",
	"read": "object.read", "relations": "object.relations", "describe-schema": "object.schema.read",
	"search": "file.read", "log": "object.history.read", "provenance": "object.provenance",
	"describe-index": "projection.read", "index-sync": "projection.manage", "index-notify": "projection.manage", "describe-access": "object.access.describe",
	"register":       "repositories.manage",
	"archive-repo":   "repositories.manage",
	"read-workspace": "file.read", "read-catalog": "inventory.read", "audit": "audit.read",
	"vfs-read": "file.read", "vfs-list": "file.read", "vfs-write": "writer.commit",
}

func AllowPath(home string) string { return filepath.Join(home, "allow.json") }

func ReadAllow(home string) (AllowFile, error) {
	file := AllowPath(home)
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return AllowFile{Version: AllowVersion, Rules: []AllowRule{}}, nil
	}
	var raw AllowFile
	if err := jsonfile.Read(file, &raw); err != nil {
		return AllowFile{}, err
	}
	if raw.Rules == nil {
		raw.Rules = []AllowRule{}
	}
	raw.Version = AllowVersion
	return raw, nil
}

func WriteAllow(home string, file AllowFile) error {
	if file.Rules == nil {
		file.Rules = []AllowRule{}
	}
	file.Version = AllowVersion
	return jsonfile.Write(AllowPath(home), file)
}

// UpdateAllow derives the next policy from the current file while holding the
// policy file's read-modify-write lock, so concurrent grant, revoke, share and
// provisioning updates cannot drop each other. update may refuse by returning
// an error; nothing is written then.
func UpdateAllow(home string, update func(*AllowFile) error) error {
	return jsonfile.Locked(AllowPath(home), func(string) error {
		file, err := ReadAllow(home)
		if err != nil {
			return err
		}
		if err := update(&file); err != nil {
			return err
		}
		return WriteAllow(home, file)
	})
}

func ValidateActions(actions []string) error { return access.ValidateActions(actions) }

func MatchAllow(rules []AllowRule, q AllowQuery) (AllowRule, bool) {
	return access.Match(rules, q)
}

func PrincipalAllowed(home, as, cmd, repo string, opened ...*kchome.Home) bool {
	if as == "" {
		return true
	}
	file, err := ReadAllow(home)
	if err != nil {
		return false
	}
	action := normalizeAction(cmd)
	_, ok := MatchAllow(file.Rules, AllowQuery{
		Principal: as,
		Action:    action,
		Repo:      repo,
	})
	if ok {
		return true
	}
	return RepositoryAuthenticatedAllowed(home, as, action, repo, opened...)
}

// withDatasetDefaults returns the rules one authorization decision sees: the
// durable grants plus the deployment's authenticated Dataset defaults for this
// verified principal. Derived rules are scoped to their Dataset, are never
// listed or written back, and an unreadable declaration adds nothing.
func withDatasetDefaults(home, principal string, rules []AllowRule) []AllowRule {
	if principal == "" {
		return rules
	}
	declared, err := kchome.ReadDatasetAccess(home)
	if err != nil || len(declared) == 0 {
		return rules
	}
	out := append(make([]AllowRule, 0, len(rules)+len(declared)), rules...)
	for _, item := range declared {
		out = append(out, AllowRule{
			ID:        "dataset-access:" + item.ID,
			Principal: principal,
			Dataset:   item.ID,
			Actions:   append([]string(nil), item.AuthenticatedActions...),
		})
	}
	return out
}

func RepositoryAuthenticatedAllowed(home, as, action, repo string, opened ...*kchome.Home) bool {
	if as == "" || repo == "" || action == "" {
		return false
	}
	if len(opened) > 0 && opened[0] != nil && opened[0].Deployment != nil {
		if kchome.RepositoryAllowsAuthenticated(*opened[0].Deployment, repo, action) {
			return true
		}
	}
	file, err := kchome.ReadRepositoryAccess(home)
	if err != nil {
		return false
	}
	return file.Allows(repo, action)
}

func InventoryReadAllowed(home, as string, opened ...*kchome.Home) bool {
	if PrincipalAllowed(home, as, "inventory.read", "", opened...) {
		return true
	}
	return as != "" && len(opened) > 0 && opened[0] != nil && opened[0].Deployment != nil
}

func ActionMatches(granted, requested string) bool {
	return access.ActionMatches(granted, requested)
}

func normalizeAction(value string) string {
	if strings.Contains(value, ".") {
		return value
	}
	if action := commandActions[value]; action != "" {
		return action
	}
	return value
}

func OwnerBypass(flags map[string]invoke.FlagValue) bool {
	return invoke.FlagString(flags, "as") == ""
}

func AuthorizeInventory(home string, flags map[string]invoke.FlagValue, observe invoke.AuthorizationObserver, opened ...*kchome.Home) (authErr error) {
	defer observeAuthorizationResult(observe, &authErr)()
	if InventoryReadAllowed(home, invoke.FlagString(flags, "as"), opened...) {
		return nil
	}
	return kernel.Fail(kernel.ErrForbidden, "%s is not allowed to inventory.read", invoke.FlagString(flags, "as"))
}

func Authorize(home, command string, flags map[string]invoke.FlagValue, observe invoke.AuthorizationObserver, opened ...*kchome.Home) (authErr error) {
	defer observeAuthorizationResult(observe, &authErr)()
	action := normalizeAction(command)
	if strings.HasPrefix(action, "object.") && invoke.FlagString(flags, "repo") == "" {
		if err := HoistTaskPinDefinition(flags); err != nil {
			return err
		}
		definition := SuppliedDataset(flags)
		if definition != nil && invoke.DatasetOf(flags) != "" {
			if action == "object.search" || action == "object.rerank" {
				return kernel.Fail(kernel.ErrForbidden, "a caller label cannot select a published Workspace while supplying a temporary definition")
			}
			return kernel.Fail(kernel.ErrUsageInvalid, "choose a named --dataset or a temporary definition")
		}
		if err := PrepareObjectPinContext(flags); err != nil {
			return err
		}
	}
	switch action {
	case "help", "identity.read", "identity.admission.request":
		return nil
	case "writer.commit":
		// commit --dataset routes by path after the body starts,
		if invoke.FlagString(flags, "dataset") != "" {
			return nil
		}
	}
	if OwnerBypass(flags) {
		return nil
	}
	if strings.HasPrefix(action, "local.") {
		return kernel.Fail(kernel.ErrForbidden, "%s is not allowed to %s", invoke.FlagString(flags, "as"), action)
	}
	file, err := ReadAllow(home)
	if err != nil {
		return err
	}
	q := AllowQuery{
		Principal: invoke.FlagString(flags, "as"),
		Action:    action,
		Repo:      invoke.FlagString(flags, "repo"),
		Ref:       invoke.FlagString(flags, "ref"),
		Object:    invoke.FlagString(flags, "object"),
		Aspect:    invoke.FlagString(flags, "aspect"),
		Dataset:   invoke.DatasetOf(flags),
	}
	file.Rules = withDatasetDefaults(home, q.Principal, file.Rules)
	definition := SuppliedDataset(flags)
	var ws *kchome.Home
	if len(opened) > 0 {
		ws = opened[0]
	}
	if action == "dataset.resolve" && definition != nil && q.Repo == "" {
		if workspaceScopeAllowed(file.Rules, q, definition) {
			return nil
		}
		return kernel.Fail(kernel.ErrForbidden, "%s is not allowed to dataset.resolve for every selected member", q.Principal)
	}
	if err := AuthorizeWorkspaceObjectDefinition(file.Rules, q, definition); err != ErrNotWorkspaceObject {
		if kernel.CodeOf(err) == kernel.ErrForbidden && definition == nil {
			if _, allowed, memberErr := namedDatasetMemberConsumption(home, flags, ws); memberErr != nil {
				return memberErr
			} else if allowed {
				return nil
			}
		}
		return err
	}
	if _, ok := MatchAllow(file.Rules, q); !ok {
		if action == "file.read" && definition == nil {
			if _, allowed, memberErr := namedDatasetMemberConsumption(home, flags, ws); memberErr != nil {
				return memberErr
			} else if allowed {
				return nil
			}
		}
		if action == "inventory.read" && InventoryReadAllowed(home, q.Principal, ws) {
			return nil
		}
		if RepositoryAuthenticatedAllowed(home, q.Principal, action, q.Repo, ws) {
			return nil
		}
		return kernel.Fail(kernel.ErrForbidden, "%s is not allowed to %s", q.Principal, action)
	}
	return nil
}

var ErrNotWorkspaceObject = fmt.Errorf("not workspace object")

// AuthorizeWorkspaceObject is the named-dataset gate: file.read on the
// Dataset admits object interpretation of listed files. It does not admit
// --repo object.* or writer.*, and it does not imply dataset.resolve.
func AuthorizeWorkspaceObject(rules []AllowRule, q AllowQuery, temporary ...bool) error {
	var definition *dataset.Dataset
	if len(temporary) > 0 && temporary[0] {
		definition = &dataset.Dataset{}
	}
	return AuthorizeWorkspaceObjectDefinition(rules, q, definition)
}

func workspaceScopeAllowed(rules []AllowRule, q AllowQuery, definition *dataset.Dataset) bool {
	if definition != nil {
		// Temporary recipe labels never identify a published grant scope.
		q.Dataset = ""
	}
	if _, ok := MatchAllow(rules, q); ok {
		return true
	}
	if definition == nil || len(definition.Sources) == 0 || q.Dataset != "" {
		return false
	}
	for _, source := range definition.Sources {
		member := q
		member.Repo = string(source.Repository)
		if member.Repo == "" {
			return false
		}
		if _, ok := MatchAllow(rules, member); !ok {
			return false
		}
	}
	return true
}

func AuthorizeWorkspaceObjectDefinition(rules []AllowRule, q AllowQuery, definition *dataset.Dataset) error {
	hasDefinition := definition != nil
	if (q.Dataset == "" && !hasDefinition) || q.Repo != "" || !strings.HasPrefix(q.Action, "object.") {
		return ErrNotWorkspaceObject
	}
	if hasDefinition {
		q.Dataset = ""
	}
	consumeQ := q
	consumeQ.Action = "file.read"
	if !workspaceScopeAllowed(rules, consumeQ, definition) {
		return kernel.Fail(kernel.ErrForbidden, "%s is not allowed to file.read", q.Principal)
	}
	return nil
}

// AuthorizationFlags derives scopes that are already fixed by a stored
// protocol object. The caller should not have to repeat these coordinates,
// and an untrusted repeated value must not be able to change the authorization
// target. Branch and merge are scoped to the ref they create or advance.
func AuthorizationFlags(cx *Invocation) map[string]invoke.FlagValue {
	if cx == nil || cx.WS == nil {
		return cx.Flags
	}
	bindNamedConsumptionScope(cx)
	derived := make(map[string]invoke.FlagValue, len(cx.Flags)+2)
	for name, value := range cx.Flags {
		derived[name] = value
	}
	if aspect := cx.Flag(referenceAspectFlag); aspect != "" {
		derived["aspect"] = aspect
	}
	switch cx.Command {
	case "writer-receipt":
		// A command's authority is fixed by its durable ledger entry. The
		// caller cannot widen or substitute that scope with request flags.
		delete(derived, "repo")
		delete(derived, "ref")
		if cx.WS.Commands != nil {
			if entry, ok := cx.WS.Commands.Lookup(cx.Flag("command-id")); ok {
				derived["repo"] = entry.Request.RepositoryID
				derived["ref"] = entry.Request.TargetRef
			}
		}
		return derived
	case "writer-branch":
		// The new branch is the authorization target; a repeated --ref
		// cannot redirect the grant to another ref.
		derived["ref"] = BranchRef(cx.Flag("name"))
		return derived
	case "writer-merge":
		// Merge authority is the target ref it advances, never the source.
		derived["ref"] = snapshot.RefOrDefault(BranchRef(cx.Flag("into")))
		return derived
	default:
		return derived
	}
}

func SplitCmds(raw string) []string {
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

// AdmissionReceipt survives ordinary rule revocation. Repeating admission
// must report its original decision without recreating any removed rule.
type AdmissionReceipt struct {
	Principal string   `json:"principal"`
	Actions   []string `json:"actions"`
}
