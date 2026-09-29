package cli

import (
	inv "kc/invocation"
)

// Outbound hooks and merge gates. Two different mechanisms that are easy to
// confuse, so they are registered side by side here:
//
//	hook-*  call a user system before/after a kc verb (docs/reviewed/hooks-and-gates.md)
//	gate-*  evidence a merge must present about a pinned Preview (docs/reviewed/hooks-and-gates.md)
//
// A gate is not a kind of hook: it is a pure check, and it cannot run a suite.

func policyVerbs() map[string]command {
	return map[string]command{
		"operations-hook-add":    {Stage: stageHome, Run: inv.VerbHookAdd},
		"operations-hook-list":   {Stage: stageHome, Run: inv.VerbHookLs},
		"operations-hook-remove": {Stage: stageHome, Run: inv.VerbHookRm},
		"operations-gate-add":    {Stage: stageHome, Run: inv.VerbGateAdd},
		"operations-gate-list":   {Stage: stageHome, Run: inv.VerbGateLs},
		"operations-gate-remove": {Stage: stageHome, Run: inv.VerbGateRm},
	}
}
