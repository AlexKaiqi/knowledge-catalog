package cli

import (
	inv "kc/invocation"
)

// Maintenance verbs: PROPOSAL → Preview → validate → Merge. Merge consults the
// gate list; a gate is a check over the pinned Preview, not a hook.
//
// The plane itself is stateless, so proposal / preview / validation ids are kept
// in the local home between commands. requireProposal and requirePreview are the
// only readers of that store.

func controlVerbs() map[string]command {
	return map[string]command{
		"governance-proposal-create":   {Stage: stageGoverned, Run: inv.VerbPropose},
		"governance-preview-create":    {Stage: stageGoverned, Run: inv.VerbPreview},
		"governance-preview-validate":  {Stage: stageGoverned, Run: inv.VerbValidate},
		"governance-validation-record": {Stage: stageGoverned, Run: inv.VerbRecordValidation},
		"governance-proposal-merge":    {Stage: stageGoverned, Run: inv.VerbMerge},
	}
}
