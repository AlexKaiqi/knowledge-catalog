package cli

import (
	inv "kc/invocation"
)

// Write surface verbs. COMMIT and PROPOSAL compile Knowledge changes onto one
// Snapshot TreeStore; the algebra is only PUT and REMOVE. Dynamic State/Stream
// values are observations, not Writer surfaces.

func writeVerbs() map[string]command {
	return map[string]command{
		"diff":           {Stage: stageGoverned, Run: inv.VerbDiff},
		"writer-put":     {Stage: stageGoverned, Run: inv.VerbPut},
		"writer-remove":  {Stage: stageGoverned, Run: inv.VerbRemove},
		"writer-commit":  {Stage: stageGoverned, Run: inv.VerbCommit},
		"writer-receipt": {Stage: stageGoverned, Run: inv.VerbReceipt},
		"writer-head":    {Stage: stageGoverned, Run: inv.VerbWriterHead},
	}
}
