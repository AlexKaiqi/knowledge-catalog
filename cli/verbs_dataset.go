package cli

import (
	inv "kc/invocation"
)

// Dataset application operations admit repositories and publish Workspace
// recipes. Managed creation delegates allocation and an explicit initial
// grant policy to Home; ordinary attach and Workspace composition do not grant.
// show lists repository ids; README is a knowledge object, not inventory
// title/summary. The Dataset package does not read knowledge.

func datasetVerbs() map[string]command {
	return map[string]command{
		"show":           {Stage: stageGoverned, Run: inv.ReadInventoryState},
		"dataset-define": {Stage: stageGoverned, Run: inv.VerbDefineKnowledgeSet},
		"dataset-clone":  {Stage: stageGoverned, Run: inv.VerbDatasetCloneRequiresServer},
		"attach":         {Stage: stageGoverned, Run: inv.VerbRegister},
		"create":         {Stage: stageGoverned, Run: inv.VerbCreateManagedRepository},
		"dataset-retire": {Stage: stageGoverned, Run: inv.VerbRetireKnowledgeSet},
		"detach":         {Stage: stageGoverned, Run: inv.VerbDetach},
	}
}
