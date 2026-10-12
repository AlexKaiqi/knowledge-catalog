package cli

// The authorization model and its evaluation gate moved to kc/invocation in
// CAP-ORG-01 B5b: the shared dispatcher authorizes inside the execution seam,
// and the CLI allow-management verbs plus the typed service handlers consume
// the same model through these aliases. The dispatcher's flag-derivation seam
// (authorizationFlags) moves with the dispatcher in B5c; the wrappers keep
// every call site unchanged meanwhile.

import (
	inv "kc/invocation"

	"kc/snapshot/dataset"
)

type (
	taskKnowledgeSetPin = inv.TaskKnowledgeSetPin
)

const (
	workspaceDefinitionFlag        = inv.WorkspaceDefinitionFlag
	inventoryDiscoveryFlag         = inv.InventoryDiscoveryFlag
	verifiedInventoryDiscoveryFlag = inv.VerifiedInventoryDiscoveryFlag
)

func writeAllow(home string, file inv.AllowFile) error { return inv.WriteAllow(home, file) }

func prepareKnowledgePinContext(flags map[string]FlagValue) error {
	return inv.PrepareKnowledgePinContext(flags)
}

func suppliedKnowledgeSet(flags map[string]FlagValue) *dataset.KnowledgeSet {
	return inv.SuppliedKnowledgeSet(flags)
}

func allowPath(home string) string { return inv.AllowPath(home) }

func splitCmds(raw string) []string { return inv.SplitCmds(raw) }

func actionMatches(granted, requested string) bool { return inv.ActionMatches(granted, requested) }
