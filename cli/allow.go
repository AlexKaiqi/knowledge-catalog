package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"kc/catalog"
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

const allowVersion = 2

var legacyActions = map[string]string{
	"put": "writer.commit", "remove": "writer.commit", "commit": "writer.commit",
	"propose": "governance.proposal.create", "preview": "governance.preview.create",
	"validate": "governance.validate", "record-validation": "governance.validation.record", "merge": "governance.merge",
	"resolve": "dataset.resolve", "resolve-object": "knowledge.read", "resolve-binding": "knowledge.binding.resolve",
	"read": "knowledge.read", "relations": "knowledge.relations", "describe-schema": "knowledge.schema.read",
	"search": "knowledge.search", "log": "knowledge.history.read", "provenance": "knowledge.provenance",
	"describe-index": "projection.read", "index-sync": "projection.manage", "index-notify": "projection.manage", "describe-access": "knowledge.access.describe",
	"register": "catalog.repositories.manage",
	"archive-catalog": "catalog.manage", "archive-repo": "catalog.repositories.manage",
	"read-workspace": "file.read", "read-catalog": "catalog.read", "audit": "audit.read",
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
		raw.Rules[i].Actions = migrateLegacyDatasetActions(raw.Rules[i].Actions)
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

func validateActions(actions []string) error {
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
		if rule.Dataset != "" && rule.Dataset != q.Dataset {
			continue
		}
		return rule, true
	}
	return AllowRule{}, false
}

func PrincipalAllowed(home, as, cmd, repo, catalogID string, opened ...*Home) bool {
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
	return repositoryAuthenticatedAllowed(home, as, action, repo, opened...)
}

func repositoryAuthenticatedAllowed(home, as, action, repo string, opened ...*Home) bool {
	if as == "" || repo == "" || action == "" {
		return false
	}
	if len(opened) > 0 && opened[0] != nil && opened[0].Deployment != nil {
		if opened[0].Deployment.RepositoryAllowsAuthenticated(repo, action) {
			return true
		}
	}
	file, err := ReadRepositoryAccess(home)
	if err != nil {
		return false
	}
	return file.Allows(repo, action)
}

func catalogReadAllowed(home, as, catalogID string, opened ...*Home) bool {
	if PrincipalAllowed(home, as, "catalog.read", "", catalogID) {
		return true
	}
	if as == "" || catalogID == "" {
		return false
	}
	var ws *Home
	if len(opened) > 0 {
		ws = opened[0]
	}
	if ws == nil || ws.Deployment == nil {
		return false
	}
	return !ws.Deployment.CatalogIsPrivate(catalogID)
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

// authorizeCatalogInventory allows DiscoverCatalogs when the principal can
// catalog.read at least one Catalog. The list body then hides the rest.
func authorizeCatalogInventory(home string, flags map[string]FlagValue, observe authorizationObserver, opened ...*Home) (authErr error) {
	defer observeAuthorizationResult(observe, &authErr)()
	if ownerBypass(flags) {
		return nil
	}
	var file HomeFile
	var err error
	if len(opened) > 0 && opened[0] != nil {
		file = opened[0].File
	} else {
		file, err = ReadHome(home)
	}
	if err != nil {
		return err
	}
	as := FlagString(flags, "as")
	for _, item := range file.Catalogs {
		if catalogReadAllowed(home, as, item.ID, opened...) {
			return nil
		}
	}
	return kernel.Fail(kernel.ErrForbidden, "%s is not allowed to catalog.read", as)
}

func authorize(home, command string, flags map[string]FlagValue, observe authorizationObserver, opened ...*Home) (authErr error) {
	defer observeAuthorizationResult(observe, &authErr)()
	action := normalizeAction(command)
	if strings.HasPrefix(action, "knowledge.") && FlagString(flags, "repo") == "" {
		if err := hoistTaskPinDefinition(flags); err != nil {
			return err
		}
		definition := suppliedKnowledgeSet(flags)
		if definition != nil && setIDOf(flags) != "" {
			if action == "knowledge.search" || action == "knowledge.rerank" {
				return kernel.Fail(kernel.ErrForbidden, "a caller label cannot select a published Workspace while supplying a temporary definition")
			}
			return kernel.Fail(kernel.ErrUsageInvalid, "choose a named --dataset or a temporary definition")
		}
		if err := prepareKnowledgePinContext(flags); err != nil {
			return err
		}
	}
	switch action {
	case "help", "identity.read", "identity.admission.request":
		return nil
	case "writer.commit":
		// commit --dataset routes by path after the body starts,
		if FlagString(flags, "dataset") != "" {
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
	catalogID := FlagString(flags, "catalog")
	if catalogID == "" {
		catalogID = FlagString(flags, "_default-catalog")
	}
	catalogID = defaultAllowCatalog(home, catalogID)
	q := AllowQuery{
		Principal: FlagString(flags, "as"),
		Action:    action,
		Repo:      FlagString(flags, "repo"),
		Catalog:   catalogID,
		Ref:       FlagString(flags, "ref"),
		Object:    FlagString(flags, "object"),
		Aspect:    FlagString(flags, "aspect"),
		Dataset:   setIDOf(flags),
	}
	definition := suppliedKnowledgeSet(flags)
	var ws *Home
	if len(opened) > 0 {
		ws = opened[0]
	}
	if isCatalogDiscovery(flags) && (action == "dataset.resolve" || action == "knowledge.search") {
		if !catalogReadAllowed(home, q.Principal, q.Catalog, ws) {
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
	if err := authorizeWorkspaceKnowledgeDefinition(file.Rules, q, definition); err != errNotWorkspaceKnowledge {
		return err
	}
	if _, ok := MatchAllow(file.Rules, q); !ok {
		if action == "catalog.read" && catalogReadAllowed(home, q.Principal, q.Catalog, ws) {
			return nil
		}
		if repositoryAuthenticatedAllowed(home, q.Principal, action, q.Repo, ws) {
			return nil
		}
		return kernel.Fail(kernel.ErrForbidden, "%s is not allowed to %s", q.Principal, action)
	}
	return nil
}

var errNotWorkspaceKnowledge = fmt.Errorf("not workspace knowledge")

// authorizeWorkspaceKnowledge is the named-dataset gate: file.read on the
// Dataset admits knowledge interpretation of listed files. It does not admit
// --repo knowledge.* or writer.*, and it does not imply dataset.resolve.
func authorizeWorkspaceKnowledge(rules []AllowRule, q AllowQuery, temporary ...bool) error {
	var definition *catalog.KnowledgeSet
	if len(temporary) > 0 && temporary[0] {
		definition = &catalog.KnowledgeSet{}
	}
	return authorizeWorkspaceKnowledgeDefinition(rules, q, definition)
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

func authorizeWorkspaceKnowledgeDefinition(rules []AllowRule, q AllowQuery, definition *catalog.KnowledgeSet) error {
	hasDefinition := definition != nil
	if (q.Dataset == "" && !hasDefinition) || q.Repo != "" || !strings.HasPrefix(q.Action, "knowledge.") {
		return errNotWorkspaceKnowledge
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

// knowledgeActionVerbAllowed admits a named-workspace knowledge verb. A
// repository-scoped grant is enough to invoke the verb; it does not drop other
// pin members from discovery.
func knowledgeActionVerbAllowed(rules []AllowRule, q AllowQuery, action string) bool {
	verbQ := q
	verbQ.Action = action
	if _, ok := MatchAllow(rules, verbQ); ok {
		return true
	}
	for _, rule := range rules {
		if rule.Principal != q.Principal {
			continue
		}
		hit := false
		for _, granted := range rule.Actions {
			if actionMatches(granted, action) {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		if rule.Catalog != "" && rule.Catalog != q.Catalog {
			continue
		}
		if rule.Dataset != "" && rule.Dataset != q.Dataset {
			continue
		}
		return true
	}
	return false
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
	case "writer-receipt":
		// A command's authority is fixed by its durable ledger entry. The
		// caller cannot widen or substitute that scope with request flags.
		delete(derived, "repo")
		delete(derived, "ref")
		if cx.WS.Commands != nil {
			if entry, ok := cx.WS.Commands.Lookup(cx.flag("command-id")); ok {
				derived["repo"] = entry.Request.RepositoryID
				derived["ref"] = entry.Request.TargetRef
			}
		}
		return derived
	case "governance-proposal-merge":
		proposal, ok := cx.WS.Control.Proposals[cx.flag("proposal")]
		if !ok {
			return cx.Flags
		}
		derived["repo"] = string(proposal.TargetRepository)
		derived["ref"] = proposal.TargetRef
		return derived
	case "governance-preview-validate", "governance-validation-record":
		preview, ok := cx.WS.Control.Previews[cx.flag("preview")]
		if !ok {
			return cx.Flags
		}
		derived["dataset"] = preview.SetID
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
