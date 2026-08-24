package scenario

import "kc/cli"

func companyAllow() []cli.AllowRule {
	return []cli.AllowRule{
		{Principal: "collector", Cmds: []string{"commit", "put"}, Repo: string(Metadata)},
		{Principal: "steward", Cmds: []string{"propose", "merge"}, Repo: string(Semantics)},
		{Principal: "steward", Cmds: []string{"define-workspace"}, Catalog: CatalogID},
		{Principal: "kai", Cmds: []string{"commit", "put", "append"}, Repo: string(Personal)},
		{Principal: "kai", Cmds: []string{"define-workspace", "read-workspace"}, Catalog: CatalogID, Workspace: ViewDesk},
		{Principal: "analyst-agent", Cmds: []string{"read-workspace", "read"}, Workspace: ViewBoard},
	}
}

func allowOK(principal, cmd, repo, catalogID, workspace string) bool {
	_, ok := cli.MatchAllow(companyAllow(), cli.AllowQuery{
		Principal: principal,
		Cmd:       cmd,
		Repo:      repo,
		Catalog:   catalogID,
		Workspace: workspace,
	})
	return ok
}

func mustAllow(t testingT, principal, cmd, repo, catalogID, workspace string) {
	t.Helper()
	if !allowOK(principal, cmd, repo, catalogID, workspace) {
		t.Fatalf("%s %s repo=%s catalog=%s workspace=%s denied", principal, cmd, repo, catalogID, workspace)
	}
}

func mustDeny(t testingT, principal, cmd, repo, catalogID, workspace string) {
	t.Helper()
	if allowOK(principal, cmd, repo, catalogID, workspace) {
		t.Fatalf("%s %s repo=%s catalog=%s workspace=%s allowed", principal, cmd, repo, catalogID, workspace)
	}
}

type testingT interface {
	Helper()
	Fatalf(string, ...any)
}
