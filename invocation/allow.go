package invocation

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"kc/catalog"
	kchome "kc/home"
	"kc/internal/jsonfile"
	"kc/invoke"
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
	Dataset   string   `json:"dataset,omitempty"`
	// ShareID and SharedBy distinguish delegated repository consumption from
	// ordinary administrator rules; repository share revoke only matches these.
	ShareID  string `json:"shareId,omitempty"`
	SharedBy string `json:"sharedBy,omitempty"`
}

type AllowFile struct {
	Version int         `json:"version"`
	Rules   []AllowRule `json:"rules"`
	// InitialGrants records completed policy applications, independently of
	// revocable rules. A provisioning retry must never restore a revoked rule.
	InitialGrants map[string]kernel.Digest `json:"initialGrants,omitempty"`
	// IdentityMigrations is preserved by grant/revoke and records one explicit
	// legacy principal migration together with its current-rule rewrite.
	IdentityMigrations map[string]IdentityMigrationReceipt `json:"identityMigrations,omitempty"`
	Admissions         map[string]AdmissionReceipt         `json:"admissions,omitempty"`
}

type AllowQuery struct {
	Principal string
	Action    string
	Repo      string
	Catalog   string
	Ref       string
	Object    string
	Aspect    string
	Dataset   string
}

const AllowVersion = 2

var legacyActions = map[string]string{
	"put": "writer.commit", "remove": "writer.commit", "commit": "writer.commit",
	"propose": "governance.proposal.create", "preview": "governance.preview.create",
	"validate": "governance.validate", "record-validation": "governance.validation.record", "merge": "governance.merge",
	"resolve": "dataset.resolve", "resolve-object": "knowledge.read", "resolve-binding": "knowledge.binding.resolve",
	"read": "knowledge.read", "relations": "knowledge.relations", "describe-schema": "knowledge.schema.read",
	"search": "knowledge.search", "log": "knowledge.history.read", "provenance": "knowledge.provenance",
	"describe-index": "projection.read", "index-sync": "projection.manage", "index-notify": "projection.manage", "describe-access": "knowledge.access.describe",
	"register":        "catalog.repositories.manage",
	"archive-catalog": "catalog.manage", "archive-repo": "catalog.repositories.manage",
	"read-workspace": "file.read", "read-catalog": "catalog.read", "audit": "audit.read",
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
	for i := range raw.Rules {
		if len(raw.Rules[i].Actions) == 0 && len(raw.Rules[i].Cmds) > 0 {
			for _, cmd := range raw.Rules[i].Cmds {
				if action := legacyActions[cmd]; action != "" {
					raw.Rules[i].Actions = appendUnique(raw.Rules[i].Actions, action)
				}
			}
		}
		raw.Rules[i].Actions = migrateLegacyDatasetActions(raw.Rules[i].Actions)
	}
	raw.Version = AllowVersion
	return raw, nil
}

func WriteAllow(home string, file AllowFile) error {
	if file.Rules == nil {
		file.Rules = []AllowRule{}
	}
	file.Version = AllowVersion
	for i := range file.Rules {
		file.Rules[i].Cmds = nil
	}
	return jsonfile.Write(AllowPath(home), file)
}

func appendUnique(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func (r *AllowRule) UnmarshalJSON(data []byte) error {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return err
	}
	if _, ok := keys["kset"]; ok {
		return fmt.Errorf("retired field kset")
	}
	type wire AllowRule
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*r = AllowRule(parsed)
	for _, action := range r.Actions {
		if strings.HasPrefix(action, "kset.") {
			return fmt.Errorf("retired action %s", action)
		}
	}
	r.Actions = migrateLegacyDatasetActions(r.Actions)
	return nil
}

func migrateLegacyDatasetActions(actions []string) []string {
	out := make([]string, 0, len(actions))
	seen := map[string]struct{}{}
	for _, action := range actions {
		if action == "dataset.consume" {
			action = "file.read"
		}
		if _, ok := seen[action]; ok {
			continue
		}
		seen[action] = struct{}{}
		out = append(out, action)
	}
	if len(out) == 0 {
		return actions
	}
	return out
}

func ValidateActions(actions []string) error {
	if len(actions) == 0 {
		return fmt.Errorf("grant add requires --action")
	}
	for _, action := range actions {
		if !strings.Contains(action, ".") || strings.ContainsAny(action, " /?#") {
			return fmt.Errorf("invalid semantic action %s", action)
		}
		if strings.HasPrefix(action, "kset.") || action == "dataset.consume" {
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
			if ActionMatches(action, q.Action) {
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
		if rule.Dataset != "" && rule.Dataset != q.Dataset {
			continue
		}
		return rule, true
	}
	return AllowRule{}, false
}

func PrincipalAllowed(home, as, cmd, repo, catalogID string, opened ...*kchome.Home) bool {
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
		Catalog:   catalogID,
	})
	if ok {
		return true
	}
	return RepositoryAuthenticatedAllowed(home, as, action, repo, opened...)
}

func RepositoryAuthenticatedAllowed(home, as, action, repo string, opened ...*kchome.Home) bool {
	if as == "" || repo == "" || action == "" {
		return false
	}
	if len(opened) > 0 && opened[0] != nil && opened[0].Deployment != nil {
		if opened[0].Deployment.RepositoryAllowsAuthenticated(repo, action) {
			return true
		}
	}
	file, err := kchome.ReadRepositoryAccess(home)
	if err != nil {
		return false
	}
	return file.Allows(repo, action)
}

func CatalogReadAllowed(home, as, catalogID string, opened ...*kchome.Home) bool {
	if PrincipalAllowed(home, as, "catalog.read", "", catalogID) {
		return true
	}
	if as == "" || catalogID == "" {
		return false
	}
	var ws *kchome.Home
	if len(opened) > 0 {
		ws = opened[0]
	}
	if ws == nil || ws.Deployment == nil {
		return false
	}
	return !ws.Deployment.CatalogIsPrivate(catalogID)
}

func ActionMatches(granted, requested string) bool {
	if granted == "*" {
		return true
	}
	if granted == requested {
		return true
	}
	if strings.HasSuffix(granted, ".*") && strings.HasPrefix(requested, strings.TrimSuffix(granted, "*")) {
		return true
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

func OwnerBypass(flags map[string]invoke.FlagValue) bool {
	return invoke.FlagString(flags, "as") == ""
}

// DefaultAllowCatalog uses the home's first Catalog when --catalog is omitted.
//
// Args:
//
//	home: workspace directory.
//	catalogID: explicit --catalog, or empty.
//
// Returns:
//
//	catalogID if set; otherwise Catalogs[0].ID; empty if the workspace has no catalog.
func DefaultAllowCatalog(home, catalogID string) string {
	if catalogID != "" {
		return catalogID
	}
	if ws, err := kchome.ReadHome(home); err == nil && len(ws.Catalogs) > 0 {
		return ws.Catalogs[0].ID
	}
	return ""
}

// authorizeCatalogInventory allows DiscoverCatalogs when the principal can
// catalog.read at least one Catalog. The list body then hides the rest.
func AuthorizeCatalogInventory(home string, flags map[string]invoke.FlagValue, observe invoke.AuthorizationObserver, opened ...*kchome.Home) (authErr error) {
	defer observeAuthorizationResult(observe, &authErr)()
	if OwnerBypass(flags) {
		return nil
	}
	var file kchome.HomeFile
	var err error
	if len(opened) > 0 && opened[0] != nil {
		file = opened[0].File
	} else {
		file, err = kchome.ReadHome(home)
	}
	if err != nil {
		return err
	}
	as := invoke.FlagString(flags, "as")
	for _, item := range file.Catalogs {
		if CatalogReadAllowed(home, as, item.ID, opened...) {
			return nil
		}
	}
	return kernel.Fail(kernel.ErrForbidden, "%s is not allowed to catalog.read", as)
}

func Authorize(home, command string, flags map[string]invoke.FlagValue, observe invoke.AuthorizationObserver, opened ...*kchome.Home) (authErr error) {
	defer observeAuthorizationResult(observe, &authErr)()
	action := normalizeAction(command)
	if strings.HasPrefix(action, "knowledge.") && invoke.FlagString(flags, "repo") == "" {
		if err := HoistTaskPinDefinition(flags); err != nil {
			return err
		}
		definition := SuppliedKnowledgeSet(flags)
		if definition != nil && invoke.DatasetOf(flags) != "" {
			if action == "knowledge.search" || action == "knowledge.rerank" {
				return kernel.Fail(kernel.ErrForbidden, "a caller label cannot select a published Workspace while supplying a temporary definition")
			}
			return kernel.Fail(kernel.ErrUsageInvalid, "choose a named --dataset or a temporary definition")
		}
		if err := PrepareKnowledgePinContext(flags); err != nil {
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
	catalogID := invoke.FlagString(flags, "catalog")
	if catalogID == "" {
		catalogID = invoke.FlagString(flags, "_default-catalog")
	}
	catalogID = DefaultAllowCatalog(home, catalogID)
	q := AllowQuery{
		Principal: invoke.FlagString(flags, "as"),
		Action:    action,
		Repo:      invoke.FlagString(flags, "repo"),
		Catalog:   catalogID,
		Ref:       invoke.FlagString(flags, "ref"),
		Object:    invoke.FlagString(flags, "object"),
		Aspect:    invoke.FlagString(flags, "aspect"),
		Dataset:   invoke.DatasetOf(flags),
	}
	definition := SuppliedKnowledgeSet(flags)
	var ws *kchome.Home
	if len(opened) > 0 {
		ws = opened[0]
	}
	if IsCatalogDiscovery(flags) && (action == "dataset.resolve" || action == "knowledge.search") {
		if !CatalogReadAllowed(home, q.Principal, q.Catalog, ws) {
			return kernel.Fail(kernel.ErrForbidden, "%s is not allowed to catalog.read", q.Principal)
		}
		return nil
	}
	if action == "dataset.resolve" && definition != nil && q.Repo == "" {
		if workspaceScopeAllowed(file.Rules, q, definition) {
			return nil
		}
		return kernel.Fail(kernel.ErrForbidden, "%s is not allowed to dataset.resolve for every selected member", q.Principal)
	}
	if err := AuthorizeWorkspaceKnowledgeDefinition(file.Rules, q, definition); err != ErrNotWorkspaceKnowledge {
		return err
	}
	if _, ok := MatchAllow(file.Rules, q); !ok {
		if action == "catalog.read" && CatalogReadAllowed(home, q.Principal, q.Catalog, ws) {
			return nil
		}
		if RepositoryAuthenticatedAllowed(home, q.Principal, action, q.Repo, ws) {
			return nil
		}
		return kernel.Fail(kernel.ErrForbidden, "%s is not allowed to %s", q.Principal, action)
	}
	return nil
}

var ErrNotWorkspaceKnowledge = fmt.Errorf("not workspace knowledge")

// AuthorizeWorkspaceKnowledge is the named-dataset gate: file.read on the
// Dataset admits knowledge interpretation of listed files. It does not admit
// --repo knowledge.* or writer.*, and it does not imply dataset.resolve.
func AuthorizeWorkspaceKnowledge(rules []AllowRule, q AllowQuery, temporary ...bool) error {
	var definition *catalog.KnowledgeSet
	if len(temporary) > 0 && temporary[0] {
		definition = &catalog.KnowledgeSet{}
	}
	return AuthorizeWorkspaceKnowledgeDefinition(rules, q, definition)
}

func workspaceScopeAllowed(rules []AllowRule, q AllowQuery, definition *catalog.KnowledgeSet) bool {
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

func AuthorizeWorkspaceKnowledgeDefinition(rules []AllowRule, q AllowQuery, definition *catalog.KnowledgeSet) error {
	hasDefinition := definition != nil
	if (q.Dataset == "" && !hasDefinition) || q.Repo != "" || !strings.HasPrefix(q.Action, "knowledge.") {
		return ErrNotWorkspaceKnowledge
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

// authorizationFlags derives scopes that are already fixed by a stored
// protocol object. The caller should not have to repeat these coordinates,
// and an untrusted repeated value must not be able to change the authorization
// target. Unknown proposal/Preview IDs intentionally stay unscoped and
// therefore fail closed for non-owners before revealing control-plane state.
func AuthorizationFlags(cx *Invocation) map[string]invoke.FlagValue {
	if cx == nil || cx.WS == nil {
		return cx.Flags
	}
	derived := make(map[string]invoke.FlagValue, len(cx.Flags)+2)
	for name, value := range cx.Flags {
		derived[name] = value
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
	case "governance-proposal-merge":
		proposal, ok := cx.WS.Control.Proposals[cx.Flag("proposal")]
		if !ok {
			return cx.Flags
		}
		derived["repo"] = string(proposal.TargetRepository)
		derived["ref"] = proposal.TargetRef
		return derived
	case "governance-preview-validate", "governance-validation-record":
		preview, ok := cx.WS.Control.Previews[cx.Flag("preview")]
		if !ok {
			return cx.Flags
		}
		derived["dataset"] = preview.SetID
		return derived
	default:
		return cx.Flags
	}
}

func NextRuleID(rules []AllowRule) string {
	return fmt.Sprintf("alw_%d", len(rules)+1)
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
	Catalog   string   `json:"catalog"`
	Actions   []string `json:"actions"`
}

// IdentityMigrationReceipt is preserved by grant/revoke and records one
// explicit legacy principal migration. The operator verifies the issuer and
// immutable subject independently; matching usernames alone never authorizes
// migration.
type IdentityMigrationReceipt struct {
	Digest        kernel.Digest `json:"digest"`
	MigratedRules int           `json:"migratedRules"`
}
