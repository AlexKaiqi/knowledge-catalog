package cli

import (
	inv "kc/invocation"
)

// Consumer read verbs. Every one of them answers on two targets:
//
//	--dataset  federated Serving over this command's ResolveKnowledgeSet pin
//	--repo  one maintainer-pinned {repository, commit}
//
// The branch is written once in onTarget so a new read verb cannot accidentally
// support only one of them, and so neither path can start following a live ref
// halfway through a command.

func readVerbs() map[string]command {
	return map[string]command{
		"pin":                       {Stage: stageGoverned, Run: inv.VerbResolve},
		"knowledge-resolve":         {Stage: stageGoverned, Run: inv.VerbResolveObject},
		"knowledge-binding-show":    {Stage: stageGoverned, Run: inv.VerbResolveBinding},
		"knowledge-read":            {Stage: stageGoverned, Run: inv.VerbRead},
		"knowledge-provenance":      {Stage: stageGoverned, Run: inv.VerbProvenance},
		"knowledge-relations":       {Stage: stageGoverned, Run: inv.VerbRelations},
		"knowledge-traverse":        {Stage: stageGoverned, Run: inv.VerbTraverse},
		"knowledge-schema-describe": {Stage: stageGoverned, Run: inv.VerbDescribeSchema},
		"knowledge-schema-list":     {Stage: stageGoverned, Run: inv.VerbBrowseSchemas},
		"knowledge-log":             {Stage: stageGoverned, Run: inv.VerbLog},
	}
}
