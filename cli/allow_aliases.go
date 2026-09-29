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

func writeAllow(home string, file AllowFile) error { return inv.WriteAllow(home, file) }

func MatchAllow(rules []AllowRule, q AllowQuery) (AllowRule, bool) { return inv.MatchAllow(rules, q) }

func prepareKnowledgePinContext(flags map[string]FlagValue) error {
	return inv.PrepareKnowledgePinContext(flags)
}

func suppliedKnowledgeSet(flags map[string]FlagValue) *catalog.KnowledgeSet {
	return inv.SuppliedKnowledgeSet(flags)
}

func allowPath(home string) string { return inv.AllowPath(home) }

func splitCmds(raw string) []string { return inv.SplitCmds(raw) }

func actionMatches(granted, requested string) bool { return inv.ActionMatches(granted, requested) }
