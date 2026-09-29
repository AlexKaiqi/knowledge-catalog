package cli

import (
	inv "kc/invocation"
)

func homeVerbs() map[string]command {
	return map[string]command{
		"help":                        {Stage: stageHome, Run: verbHelp},
		"catalog-audit":               {Stage: stageHome, Run: inv.VerbAudit},
		"deployment-init":             {Stage: stageHome, Run: verbClientOperation},
		"deployment-status":           {Stage: stageHome, Run: verbClientOperation},
		"deployment-system-publish":   {Stage: stageHome, Run: verbClientOperation},
		"deployment-identity-migrate": {Stage: stageHome, Run: verbClientOperation},
		"dataset-overlay":             {Stage: stageHome, Run: verbClientOperation},
	}
}

func verbHelp(cx *invocation) (any, error) {
	return helpFor(cx.Flag("topic"))
}
