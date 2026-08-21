package scenario

import "kc/cli"

func companyAllow() []cli.AllowRule {
	return []cli.AllowRule{
		{Principal: "collector", Cmds: []string{"commit", "put"}, Repo: string(Metadata)},
		{Principal: "steward", Cmds: []string{"propose", "merge"}, Repo: string(Semantics)},
		{Principal: "steward", Cmds: []string{"define-view"}, Catalog: CatalogID},
		{Principal: "kai", Cmds: []string{"commit", "put", "append"}, Repo: string(Personal)},
		{Principal: "kai", Cmds: []string{"define-view", "read-view"}, Catalog: CatalogID, View: ViewDesk},
		{Principal: "analyst-agent", Cmds: []string{"read-view", "read"}, View: ViewBoard},
	}
}

func allowOK(principal, cmd, repo, catalogID, view string) bool {
	_, ok := cli.MatchAllow(companyAllow(), cli.AllowQuery{
		Principal: principal,
		Cmd:       cmd,
		Repo:      repo,
		Catalog:   catalogID,
		View:      view,
	})
	return ok
}

func mustAllow(t testingT, principal, cmd, repo, catalogID, view string) {
	t.Helper()
	if !allowOK(principal, cmd, repo, catalogID, view) {
		t.Fatalf("%s %s repo=%s catalog=%s view=%s denied", principal, cmd, repo, catalogID, view)
	}
}

func mustDeny(t testingT, principal, cmd, repo, catalogID, view string) {
	t.Helper()
	if allowOK(principal, cmd, repo, catalogID, view) {
		t.Fatalf("%s %s repo=%s catalog=%s view=%s allowed", principal, cmd, repo, catalogID, view)
	}
}

type testingT interface {
	Helper()
	Fatalf(string, ...any)
}
