package cli

import (
	inv "kc/invocation"
)

// Retrieval derivation verbs (layer ③). An index only locates; the caller reads
// the canonical unit back after a hit. One index belongs to one
// (repository, basisCommit) plus that Repository's schema, never to a Workspace.

func indexVerbs() map[string]command {
	return map[string]command{
		"knowledge-search":                {Stage: stageGoverned, Run: inv.VerbSearch},
		"operations-projection-describe":  {Stage: stageGoverned, Run: inv.VerbDescribeIndex},
		"operations-projection-sync":      {Stage: stageGoverned, Run: inv.VerbIndexSync},
		"operations-projection-notice":    {Stage: stageGoverned, Run: inv.VerbIndexNotify},
		"operations-access-spec-describe": {Stage: stageGoverned, Run: inv.VerbDescribeAccess},
	}
}
