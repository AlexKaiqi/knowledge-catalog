package cli

import (
	inv "kc/invocation"
)

func observabilityVerbs() map[string]command {
	return map[string]command{
		"operations-audit-access":    {Stage: stageHome, Run: inv.VerbAccessLog},
		"operations-audit-trace":     {Stage: stageHome, Run: inv.VerbTrace},
		"operations-audit-hitmap":    {Stage: stageHome, Run: inv.VerbHitmap},
		"operations-feedback-record": {Stage: stageHome, Run: inv.VerbRecordFeedback},
	}
}
