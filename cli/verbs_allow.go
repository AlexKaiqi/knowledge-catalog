package cli

import (
	inv "kc/invocation"
)

// Grant operations. A grant is over stable semantic actions on a Repository or the service registration space,
// evaluated against .kc/allow.json. Table-level GRANT is knowledge, enforced by
// the source system, and never lands here. See docs/reviewed/permissions.md.
//
// These stay at stageHome: answering "who may do what" must not require the
// workspace to mount.

func allowVerbs() map[string]command {
	return map[string]command{
		"whoami": {Stage: stageHome, Run: inv.VerbWhoami},
		"admission-show": {Stage: stageHome, Run: func(cx *invocation) (any, error) {
			return admissionOverview(cx)
		}},
		"grant-add":    {Stage: stageHome, Run: inv.VerbAllow},
		"grant-remove": {Stage: stageHome, Run: inv.VerbRevoke},
		"grant-list":   {Stage: stageHome, Run: inv.VerbAllowed},
	}
}
