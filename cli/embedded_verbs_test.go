package cli

import (
	inv "kc/invocation"
	"kc/invoke"
)

import (
	apphome "kc/home"

	"kc/snapshot/dataset"
)

import (
	"strings"
)

func allowVerbs() map[string]inv.Command {
	return map[string]inv.Command{
		"whoami": {Stage: invoke.StageHome, Run: inv.VerbWhoami},
		"admission-show": {Stage: invoke.StageHome, Run: func(cx *inv.Invocation) (any, error) {
			return admissionOverview(cx)
		}},
		"grant-add":    {Stage: invoke.StageHome, Run: inv.VerbAllow},
		"grant-remove": {Stage: invoke.StageHome, Run: inv.VerbRevoke},
		"grant-list":   {Stage: invoke.StageHome, Run: inv.VerbAllowed},
		"grant-update": {Stage: invoke.StageHome, Run: snapshotServerRequired},
	}
}

func datasetVerbs() map[string]inv.Command {
	return map[string]inv.Command{
		"show":           {Stage: invoke.StageGoverned, Run: inv.ReadInventoryState},
		"dataset-define": {Stage: invoke.StageGoverned, Run: inv.VerbDefineDataset},
		"dataset-show":   {Stage: invoke.StageHome, Run: snapshotServerRequired},
		"files-list":     {Stage: invoke.StageHome, Run: snapshotServerRequired},
		"files-read":     {Stage: invoke.StageHome, Run: snapshotServerRequired},
		"attach":         {Stage: invoke.StageGoverned, Run: inv.VerbRegister},
		"create":         {Stage: invoke.StageGoverned, Run: inv.VerbCreateManagedRepository},
		"dataset-retire": {Stage: invoke.StageGoverned, Run: inv.VerbRetireDataset},
		"detach":         {Stage: invoke.StageGoverned, Run: inv.VerbDetach},
	}
}

func homeVerbs() map[string]inv.Command {
	return map[string]inv.Command{
		"help":                      {Stage: invoke.StageHome, Run: verbHelp},
		"registry-audit":            {Stage: invoke.StageHome, Run: inv.VerbAudit},
		"deployment-init":           {Stage: invoke.StageHome, Run: verbClientOperation},
		"deployment-status":         {Stage: invoke.StageHome, Run: verbClientOperation},
		"deployment-system-publish": {Stage: invoke.StageHome, Run: verbClientOperation},
		"dataset-overlay":           {Stage: invoke.StageHome, Run: verbClientOperation},
	}
}

func verbHelp(cx *inv.Invocation) (any, error) {
	return helpFor(cx.Flag("topic"))
}

func indexVerbs() map[string]inv.Command {
	return map[string]inv.Command{
		"object-search":                   {Stage: invoke.StageGoverned, Run: inv.VerbSearch},
		"operations-projection-describe":  {Stage: invoke.StageGoverned, Run: inv.VerbDescribeIndex},
		"operations-projection-sync":      {Stage: invoke.StageGoverned, Run: inv.VerbIndexSync},
		"operations-projection-notice":    {Stage: invoke.StageGoverned, Run: inv.VerbIndexNotify},
		"operations-access-spec-describe": {Stage: invoke.StageGoverned, Run: inv.VerbDescribeAccess},
	}
}

func maintenanceVerbs() map[string]inv.Command {
	return map[string]inv.Command{
		"pin-check": {Stage: invoke.StageGoverned, Run: inv.VerbCheckDataset},
	}
}

func observabilityVerbs() map[string]inv.Command {
	return map[string]inv.Command{
		"operations-audit-access":    {Stage: invoke.StageHome, Run: inv.VerbAccessLog},
		"operations-audit-trace":     {Stage: invoke.StageHome, Run: inv.VerbTrace},
		"operations-audit-hitmap":    {Stage: invoke.StageHome, Run: inv.VerbHitmap},
		"operations-feedback-record": {Stage: invoke.StageHome, Run: inv.VerbRecordFeedback},
	}
}

func policyVerbs() map[string]inv.Command {
	return map[string]inv.Command{
		"operations-hook-add":    {Stage: invoke.StageHome, Run: inv.VerbHookAdd},
		"operations-hook-list":   {Stage: invoke.StageHome, Run: inv.VerbHookLs},
		"operations-hook-remove": {Stage: invoke.StageHome, Run: inv.VerbHookRm},
	}
}

func readVerbs() map[string]inv.Command {
	return map[string]inv.Command{
		"object-plugin-help":     {Stage: invoke.StageHome, Run: inv.VerbPluginHelp},
		"pin":                    {Stage: invoke.StageGoverned, Run: inv.VerbResolve},
		"object-resolve":         {Stage: invoke.StageGoverned, Run: inv.VerbResolveObject},
		"object-binding-show":    {Stage: invoke.StageGoverned, Run: inv.VerbResolveBinding},
		"object-read":            {Stage: invoke.StageGoverned, Run: inv.VerbRead},
		"object-provenance":      {Stage: invoke.StageGoverned, Run: inv.VerbProvenance},
		"object-relations":       {Stage: invoke.StageGoverned, Run: inv.VerbRelations},
		"object-traverse":        {Stage: invoke.StageGoverned, Run: inv.VerbTraverse},
		"object-schema-describe": {Stage: invoke.StageGoverned, Run: inv.VerbDescribeSchema},
		"object-schema-list":     {Stage: invoke.StageGoverned, Run: inv.VerbBrowseSchemas},
		"object-log":             {Stage: invoke.StageGoverned, Run: inv.VerbLog},
	}
}

func resourceVerbs() map[string]inv.Command {
	return map[string]inv.Command{
		"object-access": {Stage: invoke.StageGoverned, Run: inv.VerbResourceAccess},
		"object-invoke": {Stage: invoke.StageGoverned, Run: inv.VerbResourceInvoke},
	}
}

// effectiveWorkspace is the recipe this command actually uses: registration state (or a
// hitchhiking yaml) plus the local overlay for this --as, if any.
func effectiveWorkspace(ws *apphome.Home, home string, cat dataset.Service, setID string, flags map[string]FlagValue) (dataset.Dataset, error) {
	def, err := inv.EnsureWorkspace(ws, home, cat, setID)
	if err != nil {
		return dataset.Dataset{}, err
	}
	if ws.Deployment != nil {
		return def, nil
	}
	return inv.ApplyOverlay(home, invoke.FlagString(flags, "as"), def)
}

func writeVerbs() map[string]inv.Command {
	return map[string]inv.Command{
		"diff":           {Stage: invoke.StageGoverned, Run: inv.VerbDiff},
		"writer-put":     {Stage: invoke.StageGoverned, Run: inv.VerbPut},
		"writer-remove":  {Stage: invoke.StageGoverned, Run: inv.VerbRemove},
		"writer-commit":  {Stage: invoke.StageGoverned, Run: inv.VerbCommit},
		"writer-receipt": {Stage: invoke.StageGoverned, Run: inv.VerbReceipt},
		"writer-head":    {Stage: invoke.StageGoverned, Run: inv.VerbWriterHead},
		"writer-branch":  {Stage: invoke.StageGoverned, Run: inv.VerbWriterBranch},
		"writer-merge":   {Stage: invoke.StageGoverned, Run: inv.VerbWriterMerge},
	}
}

// loginVerbs returns the login/logout command group. These are stage-local
// commands that manage client-side authentication state before connecting to
// a KC server.
func loginVerbs() map[string]inv.Command {
	return map[string]inv.Command{
		"login":  {Stage: invoke.StageHome, Run: verbLogin},
		"logout": {Stage: invoke.StageHome, Run: verbLogout},
	}
}

func verbClientOperation(cx *inv.Invocation) (any, error) {
	return runClientOperation(strings.ReplaceAll(cx.Command, "-", " "), cx.Flags)
}
