package cli

import (
	inv "kc/invocation"
)

func maintenanceVerbs() map[string]command {
	return map[string]command{
		"pin-check": {Stage: stageGoverned, Run: inv.VerbCheckKnowledgeSet},
	}
}
