package cli

// The authorization model and its evaluation gate moved to kc/invocation in
// CAP-ORG-01 B5b: the shared dispatcher authorizes inside the execution seam,
// and the CLI allow-management verbs plus the typed service handlers consume
// the same model through these aliases. The dispatcher's flag-derivation seam
// (authorizationFlags) moves with the dispatcher in B5c; the wrappers keep
// every call site unchanged meanwhile.

import (
	inv "kc/invocation"

	"kc/catalog"
)

type (
	AllowRule                = inv.AllowRule
	AllowFile                = inv.AllowFile
	AllowQuery               = inv.AllowQuery
	AdmissionReceipt         = inv.AdmissionReceipt
	IdentityMigrationReceipt = inv.IdentityMigrationReceipt
	taskKnowledgeSetPin      = inv.TaskKnowledgeSetPin
)

const (
	workspaceDefinitionFlag      = inv.WorkspaceDefinitionFlag
	catalogDiscoveryFlag         = inv.CatalogDiscoveryFlag
	verifiedCatalogDiscoveryFlag = inv.VerifiedCatalogDiscoveryFlag
)

func ReadAllow(home string) (AllowFile, error) { return inv.ReadAllow(home) }

func WriteAllow(home string, file AllowFile) error { return inv.WriteAllow(home, file) }

func MatchAllow(rules []AllowRule, q AllowQuery) (AllowRule, bool) { return inv.MatchAllow(rules, q) }

func PrincipalAllowed(home, as, cmd, repo, catalogID string, opened ...*Home) bool {
	return inv.PrincipalAllowed(home, as, cmd, repo, catalogID, opened...)
}

func authorize(home, command string, flags map[string]FlagValue, observe authorizationObserver, opened ...*Home) (authErr error) {
	return inv.Authorize(home, command, flags, observe, opened...)
}
func authorizationFlags(cx *invocation) map[string]FlagValue { return inv.AuthorizationFlags(cx) }

func prepareKnowledgePinContext(flags map[string]FlagValue) error {
	return inv.PrepareKnowledgePinContext(flags)
}

func suppliedKnowledgeSet(flags map[string]FlagValue) *catalog.KnowledgeSet {
	return inv.SuppliedKnowledgeSet(flags)
}

func prepareCatalogDiscovery(ws *Home, action string, flags map[string]FlagValue) error {
	return inv.PrepareCatalogDiscovery(ws, action, flags)
}

func configuredDiscoveryWorkspace(ws *Home, catalogID string) string {
	return inv.ConfiguredDiscoveryWorkspace(ws, catalogID)
}

func ownerBypass(flags map[string]FlagValue) bool { return inv.OwnerBypass(flags) }

func defaultAllowCatalog(home, catalogID string) string {
	return inv.DefaultAllowCatalog(home, catalogID)
}

func repositoryAuthenticatedAllowed(home, as, action, repo string, opened ...*Home) bool {
	return inv.RepositoryAuthenticatedAllowed(home, as, action, repo, opened...)
}

func allowPath(home string) string { return inv.AllowPath(home) }

func splitCmds(raw string) []string { return inv.SplitCmds(raw) }

func validateActions(actions []string) error { return inv.ValidateActions(actions) }

func nextRuleID(rules []AllowRule) string { return inv.NextRuleID(rules) }

func actionMatches(granted, requested string) bool { return inv.ActionMatches(granted, requested) }

func authorizeWorkspaceKnowledge(rules []AllowRule, q AllowQuery, temporary ...bool) error {
	return inv.AuthorizeWorkspaceKnowledge(rules, q, temporary...)
}

var errNotWorkspaceKnowledge = inv.ErrNotWorkspaceKnowledge

var allowVersion = inv.AllowVersion

func hoistTaskPinDefinition(flags map[string]FlagValue) error {
	return inv.HoistTaskPinDefinition(flags)
}

func catalogReadAllowed(home, as, catalogID string, opened ...*Home) bool {
	return inv.CatalogReadAllowed(home, as, catalogID, opened...)
}

func isCatalogDiscovery(flags map[string]FlagValue) bool { return inv.IsCatalogDiscovery(flags) }
