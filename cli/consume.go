package cli

import (
	"fmt"

	"kc/catalog"
	"kc/kernel"
	"kc/repository"
)

func servingView(flags map[string]FlagValue) bool {
	return FlagString(flags, "view") != "" && !readingCatalogCommand(flags)
}

func readingCatalogCommand(flags map[string]FlagValue) bool {
	if FlagString(flags, "repo") != "" || FlagString(flags, "commit") != "" || FlagString(flags, "ref") != "" {
		return false
	}
	if FlagString(flags, "object") != "" || FlagString(flags, "aspect") != "" {
		return false
	}
	if FlagString(flags, "view") != "" {
		return false
	}
	_, ok := flags["catalog"]
	return ok
}

func readingCatalog(command string, flags map[string]FlagValue) bool {
	if command != "read" {
		return false
	}
	return readingCatalogCommand(flags)
}

func consumerAllowCmd(command string, flags map[string]FlagValue) string {
	if readingCatalog(command, flags) {
		return "read-catalog"
	}
	if servingView(flags) {
		switch command {
		case "read", "list", "search", "provenance", "describe-schema", "resolve", "log":
			return "read-view"
		}
	}
	return command
}

func rejectRetiredCommand(command string) error {
	switch command {
	case "read-catalog":
		return fmt.Errorf("unknown command read-catalog; use: kc read --catalog")
	case "read-release":
		return fmt.Errorf("unknown command read-release; use: kc read --view")
	case "pin-view":
		return fmt.Errorf("unknown command pin-view; consumers follow the view's published branches")
	case "promote":
		return fmt.Errorf("unknown command promote; publishers move the repo branch; consumers read --view")
	case "rollback":
		return fmt.Errorf("unknown command rollback; revert the knowledge repo instead")
	case "retire-release":
		return fmt.Errorf("unknown command retire-release; use: kc retire-view")
	}
	return nil
}

func rejectRetiredFlags(flags map[string]FlagValue) error {
	if FlagString(flags, "release") != "" {
		return fmt.Errorf("unknown flag --release; use: kc read --view")
	}
	if FlagString(flags, "generation") != "" || FlagString(flags, "base-generation") != "" {
		return fmt.Errorf("unknown flag --generation; preview binds a view, not a stored generation")
	}
	return nil
}

func openServing(ws *OpenWorkspace, flags map[string]FlagValue) (*catalog.Serving, *catalog.Catalog, error) {
	if FlagString(flags, "repo") != "" || FlagString(flags, "commit") != "" || FlagString(flags, "ref") != "" {
		return nil, nil, fmt.Errorf("--view cannot be combined with --repo, --commit, or --ref")
	}
	if FlagString(flags, "release") != "" {
		return nil, nil, fmt.Errorf("unknown flag --release; use: kc read --view")
	}
	cat, err := pickCatalog(ws, flags)
	if err != nil {
		return nil, nil, err
	}
	viewID, err := RequireFlag(flags, "view")
	if err != nil {
		return nil, nil, err
	}
	serving, err := cat.OpenView(viewID)
	if err != nil {
		return nil, nil, err
	}
	return serving, cat, nil
}

func aspectSelectorFrom(flags map[string]FlagValue) *repository.AspectSelector {
	include := FlagStrings(flags, "include")
	exclude := FlagStrings(flags, "exclude")
	if len(include) == 0 && len(exclude) == 0 {
		return nil
	}
	return &repository.AspectSelector{Include: include, Exclude: exclude}
}

func allowedRepoRead(home string, flags map[string]FlagValue, repo, object string) bool {
	if ownerBypass(flags) {
		return true
	}
	file, err := ReadAllow(home)
	if err != nil {
		return true
	}
	_, ok := MatchAllow(file.Rules, AllowQuery{
		Principal: FlagString(flags, "as"),
		Cmd:       "read",
		Repo:      repo,
		Object:    object,
	})
	return ok
}

func filterViewReads(home string, flags map[string]FlagValue, _ *catalog.Catalog, values []catalog.FederatedValue) []catalog.FederatedValue {
	out := []catalog.FederatedValue{}
	for _, item := range values {
		if allowedRepoRead(home, flags, string(item.Repository), string(item.ObjectID)) {
			out = append(out, item)
		}
	}
	return out
}

func filterKnowledgeReads(home string, flags map[string]FlagValue, values []repository.KnowledgeValue) []repository.KnowledgeValue {
	out := []repository.KnowledgeValue{}
	for _, item := range values {
		if allowedRepoRead(home, flags, string(item.Repository), string(item.Address.ObjectID)) {
			out = append(out, item)
		}
	}
	return out
}

func filterViewLogs(home string, flags map[string]FlagValue, logs []catalog.ObjectLog) []catalog.ObjectLog {
	out := []catalog.ObjectLog{}
	for _, item := range logs {
		if allowedRepoRead(home, flags, string(item.Repository), string(item.ObjectID)) {
			out = append(out, item)
		}
	}
	return out
}

func searchView(ws *OpenWorkspace, home string, flags map[string]FlagValue) (any, error) {
	serving, cat, err := openServing(ws, flags)
	if err != nil {
		return nil, err
	}
	plan, err := cat.PlanIndexResolved(serving.Resolved())
	if err != nil {
		return nil, err
	}
	req, err := searchRequestFromFlags(flags)
	if err != nil {
		return nil, err
	}
	out := []repository.KnowledgeValue{}
	tried, unsat := 0, 0
	for _, proj := range plan.Projections {
		repo, err := ws.Store.Require(proj.Repository, kernel.ErrTemporaryUnavailable)
		if err != nil {
			return nil, err
		}
		tried++
		hits, err := ws.Index.SearchAt(repo, proj.Commit, req)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrCapabilityUnsatisfied {
				unsat++
				continue
			}
			return nil, err
		}
		out = append(out, hits...)
	}
	if tried > 0 && unsat == tried {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "no member index satisfies this search")
	}
	return filterKnowledgeReads(home, flags, out), nil
}
