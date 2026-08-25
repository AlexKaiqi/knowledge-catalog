package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kc/catalog"
	"kc/internal/gitdir"
	"kc/kernel"
	"kc/reader"
	"kc/repository"
	"kc/writer"
)

func servingWorkspace(flags map[string]FlagValue) bool {
	return workspaceIDOf(flags) != "" && !readingCatalogCommand(flags)
}

func workspaceIDOf(flags map[string]FlagValue) string {
	return FlagString(flags, "workspace")
}

func workspaceIDFlag(flags map[string]FlagValue) (string, error) {
	workspace := FlagString(flags, "workspace")
	if workspace == "" {
		return "", fmt.Errorf("missing --workspace")
	}
	return workspace, nil
}

func readingCatalogCommand(flags map[string]FlagValue) bool {
	if FlagString(flags, "repo") != "" || FlagString(flags, "commit") != "" || FlagString(flags, "ref") != "" {
		return false
	}
	if FlagString(flags, "object") != "" || FlagString(flags, "aspect") != "" {
		return false
	}
	if workspaceIDOf(flags) != "" {
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
	if servingWorkspace(flags) {
		switch command {
		case "read", "list", "search", "provenance", "describe-schema", "resolve", "resolve-binding", "log", "checkout", "inspect", "describe-access",
			"sync", "vfs-read", "vfs-list":
			return "read-workspace"
		}
	}
	return command
}

func rejectRemovedFlags(flags map[string]FlagValue) error {
	for _, name := range []string{"view", "release", "generation", "base-generation", "input-vrv"} {
		if _, exists := flags[name]; exists {
			return fmt.Errorf("unknown flag --%s", name)
		}
	}
	return nil
}

func openServing(ws *Home, flags map[string]FlagValue) (*reader.Serving, *catalog.Catalog, error) {
	if FlagString(flags, "repo") != "" || FlagString(flags, "commit") != "" || FlagString(flags, "ref") != "" {
		return nil, nil, fmt.Errorf("--workspace cannot be combined with --repo, --commit, or --ref")
	}
	cat, err := pickCatalog(ws, flags)
	if err != nil {
		return nil, nil, err
	}
	workspaceID, err := workspaceIDFlag(flags)
	if err != nil {
		return nil, nil, err
	}
	resolved, err := resolveOrReplay(ws, ws.Dir, cat, workspaceID, flags)
	if err != nil {
		return nil, nil, err
	}
	serving := reader.Open(cat.RequireKnowledge, workspacePin(resolved))
	return serving, cat, nil
}

func resolveOrReplay(ws *Home, home string, cat *catalog.Catalog, workspaceID string, flags map[string]FlagValue) (catalog.ResolvedWorkspace, error) {
	def, err := effectiveWorkspace(ws, home, cat, workspaceID, flags)
	if err != nil {
		return catalog.ResolvedWorkspace{}, err
	}
	if pinPath := FlagString(flags, "pin"); pinPath != "" {
		resolved, replayErr := replayPin(cat, def, pinPath)
		if replayErr == nil {
			flags[resolvedPinFlag] = resolved.PinID
		}
		return resolved, replayErr
	}
	resolved, resolveErr := cat.ResolveDefinition(def)
	if resolveErr == nil {
		flags[resolvedPinFlag] = resolved.PinID
	}
	return resolved, resolveErr
}

func replayPin(cat *catalog.Catalog, def catalog.WorkspaceDefinition, pinPath string) (catalog.ResolvedWorkspace, error) {
	raw := []byte(pinPath)
	if !strings.HasPrefix(strings.TrimSpace(pinPath), "{") {
		var err error
		raw, err = os.ReadFile(pinPath)
		if err != nil {
			return catalog.ResolvedWorkspace{}, err
		}
	}
	var pin catalog.ResolvedWorkspace
	if err := catalog.DecodeJSON(raw, &pin); err != nil {
		return catalog.ResolvedWorkspace{}, kernel.Fail(kernel.ErrUsageInvalid, "--pin is not a ResolvedWorkspace JSON file")
	}
	workspaceID := def.WorkspaceID
	if pin.WorkspaceID != "" && pin.WorkspaceID != workspaceID {
		return catalog.ResolvedWorkspace{}, kernel.Fail(kernel.ErrUsageInvalid, "--pin is for workspace %s, not %s", pin.WorkspaceID, workspaceID)
	}
	pin.WorkspaceID = workspaceID
	want := map[kernel.RepositoryID]struct{}{}
	for _, src := range def.Sources {
		want[src.Repository] = struct{}{}
	}
	if len(want) != len(pin.Repositories) {
		return catalog.ResolvedWorkspace{}, kernel.Fail(kernel.ErrUsageInvalid, "--pin membership does not match workspace %s", workspaceID)
	}
	for id := range pin.Repositories {
		if _, ok := want[id]; !ok {
			return catalog.ResolvedWorkspace{}, kernel.Fail(kernel.ErrUsageInvalid, "--pin names repository %s which is not in workspace %s", id, workspaceID)
		}
	}
	if check := cat.CheckResolved(pin); check.Outcome != "PASSED" {
		return catalog.ResolvedWorkspace{}, kernel.Fail(kernel.ErrWorkspaceInvalid, "replayed pin failed CheckResolved")
	}
	return pin, nil
}

func workspacePin(resolved catalog.ResolvedWorkspace) reader.WorkspacePin {
	return reader.WorkspacePin{
		WorkspaceID:  resolved.WorkspaceID,
		Revision:     resolved.Revision,
		Repositories: resolved.Repositories,
	}
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

func filterWorkspaceReads(home string, flags map[string]FlagValue, _ *catalog.Catalog, values []reader.FederatedValue) []reader.FederatedValue {
	out := []reader.FederatedValue{}
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

func filterWorkspaceLogs(home string, flags map[string]FlagValue, logs []reader.ObjectLog) []reader.ObjectLog {
	out := []reader.ObjectLog{}
	for _, item := range logs {
		if allowedRepoRead(home, flags, string(item.Repository), string(item.ObjectID)) {
			out = append(out, item)
		}
	}
	return out
}

func searchWorkspace(ws *Home, home string, flags map[string]FlagValue) (any, error) {
	serving, cat, err := openServing(ws, flags)
	if err != nil {
		return nil, err
	}
	plan, err := reader.PlanAccess(cat.RequireKnowledge, serving.Pin())
	if err != nil {
		return nil, err
	}
	req, err := searchRequestFromFlags(flags)
	if err != nil {
		return nil, err
	}
	out := reader.SearchResult{
		View:         reader.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{}},
		Completeness: reader.CompletenessComplete,
		Hits:         []reader.KnowledgeHit{},
	}
	for _, spec := range plan.Specs {
		out.View.Snapshots[spec.Repository] = spec.Commit
	}
	queryDigest := reader.SearchQueryDigest(req)
	viewDigest := reader.SearchViewDigest(out.View)
	startMember := 0
	memberContinuation := ""
	if req.Continuation != "" {
		state, err := reader.DecodeContinuation(req.Continuation)
		if err != nil || state.Scope != "workspace" || state.Query != queryDigest || state.View != viewDigest || state.Member < 0 || state.Member >= len(plan.Specs) {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "continuation does not match this search view")
		}
		startMember = state.Member
		memberContinuation = state.Position
	}
	req.Continuation = ""
	tried, unsat := 0, 0
	for memberIndex := startMember; memberIndex < len(plan.Specs); memberIndex++ {
		spec := plan.Specs[memberIndex]
		repo, err := ws.Store.Knowledge(spec.Repository, kernel.ErrUsageInvalid)
		if err != nil {
			return nil, err
		}
		for {
			memberReq := req
			memberReq.Continuation = memberContinuation
			if req.Limit > 0 {
				memberReq.Limit = req.Limit - len(out.Hits)
				if memberReq.Limit <= 0 {
					break
				}
			}
			tried++
			member, err := ws.Index.SearchAt(repo, spec.Commit, memberReq)
			if err != nil {
				if kernel.CodeOf(err) == kernel.ErrCapabilityUnsatisfied {
					unsat++
					out.Completeness = reader.CompletenessPartial
					out.Claims = append(out.Claims, "member does not satisfy search: "+string(spec.Repository))
					memberContinuation = ""
					break
				}
				return nil, err
			}
			if member.Completeness == reader.CompletenessPartial {
				out.Completeness = reader.CompletenessPartial
			}
			out.Claims = append(out.Claims, member.Claims...)
			for _, hit := range member.Hits {
				if allowedRepoRead(home, flags, string(hit.Knowledge.Repository), string(hit.Knowledge.Address.ObjectID)) {
					out.Hits = append(out.Hits, hit)
				}
			}
			memberContinuation = member.Continuation
			if req.Limit > 0 && len(out.Hits) >= req.Limit {
				nextMember := memberIndex
				if memberContinuation == "" {
					nextMember++
				}
				if nextMember < len(plan.Specs) {
					out.Continuation = reader.EncodeContinuation(reader.ContinuationState{
						Scope: "workspace", Query: queryDigest, View: viewDigest,
						Member: nextMember, Position: memberContinuation,
					})
				}
				return out, nil
			}
			if memberContinuation == "" {
				break
			}
		}
		memberContinuation = ""
	}
	if tried > 0 && unsat == tried {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "no member index satisfies this search")
	}
	return out, nil
}

func checkoutWorkspace(ws *Home, home string, flags map[string]FlagValue) (any, error) {
	cat, err := pickCatalog(ws, flags)
	if err != nil {
		return nil, err
	}
	workspaceID, err := workspaceIDFlag(flags)
	if err != nil {
		return nil, err
	}
	def, err := effectiveWorkspace(ws, home, cat, workspaceID, flags)
	if err != nil {
		return nil, err
	}
	dest, err := checkoutDest(ws, home, workspaceID, flags)
	if err != nil {
		return nil, err
	}
	if recipeHasMounts(def) {
		return checkoutMountsWorkspace(ws, home, flags, cat, def, dest)
	}
	return checkoutKnowledgeWorkspace(ws, home, flags, dest)
}

func recipeHasMounts(def catalog.WorkspaceDefinition) bool {
	return catalog.HasMountPaths(def.Sources)
}

func checkoutDest(ws *Home, home, workspaceID string, flags map[string]FlagValue) (string, error) {
	if to := FlagString(flags, "to"); to != "" {
		if filepath.IsAbs(to) {
			return to, nil
		}
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(cwd, to), nil
	}
	root, err := resolveStoreDir(home, ws.Stores.Layout.Checkouts, defaultCheckoutsDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, reader.EncodeCheckoutDir(workspaceID)), nil
}

func deniedMounts(home string, flags map[string]FlagValue, def catalog.WorkspaceDefinition) map[kernel.RepositoryID]string {
	out := map[kernel.RepositoryID]string{}
	as := FlagString(flags, "as")
	for _, src := range def.Sources {
		if allowedRepoRead(home, flags, string(src.Repository), "") {
			continue
		}
		reason := "not allowed to read " + string(src.Repository)
		if as != "" {
			reason = as + " is not allowed to read " + string(src.Repository)
		}
		out[src.Repository] = reason
	}
	return out
}

func checkoutMountsWorkspace(ws *Home, home string, flags map[string]FlagValue, cat *catalog.Catalog, def catalog.WorkspaceDefinition, dest string) (any, error) {
	denied := deniedMounts(home, flags, def)
	mounts, err := cat.CheckoutMountsAllowingDef(def, dest, denied)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"workspaceId": def.WorkspaceID,
		"dir":         homeRel(home, dest),
		"mounts":      mounts,
	}, nil
}

func checkoutKnowledgeWorkspace(ws *Home, home string, flags map[string]FlagValue, dest string) (any, error) {
	serving, cat, err := openServing(ws, flags)
	if err != nil {
		return nil, err
	}
	values, err := serving.List()
	if err != nil {
		return nil, err
	}
	values = filterWorkspaceReads(home, flags, cat, values)
	report, err := reader.WriteCheckout(dest, serving.Pin(), values)
	if err != nil {
		return nil, err
	}
	report.Dir = homeRel(home, dest)
	return withKnowledgeEvidence(report, values), nil
}

func verbSync(cx *invocation) (any, error) {
	workspaceID, err := workspaceIDFlag(cx.Flags)
	if err != nil {
		return nil, err
	}
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	def, err := effectiveWorkspace(cx.WS, cx.Home, cat, workspaceID, cx.Flags)
	if err != nil {
		return nil, err
	}
	dest, err := checkoutDest(cx.WS, cx.Home, workspaceID, cx.Flags)
	if err != nil {
		return nil, err
	}
	syncs, err := cat.SyncMountsDef(def, dest)
	if err != nil {
		return nil, err
	}
	return map[string]any{"workspaceId": workspaceID, "dir": homeRel(cx.Home, dest), "mounts": syncs}, nil
}

func statusMounts(cx *invocation) (any, error) {
	workspaceID, err := workspaceIDFlag(cx.Flags)
	if err != nil {
		if to := cx.flag("to"); to != "" {
			pin, pinErr := catalog.ReadCheckoutPin(to)
			if pinErr != nil {
				return nil, pinErr
			}
			if pin == nil {
				return nil, kernel.Fail(kernel.ErrUsageInvalid, "%s has never been checked out; pass --workspace or run checkout first", to)
			}
			workspaceID = pin.WorkspaceID
			cx.Flags["workspace"] = workspaceID
		} else {
			return nil, err
		}
	}
	dest, err := checkoutDest(cx.WS, cx.Home, workspaceID, cx.Flags)
	if err != nil {
		return nil, err
	}
	pin, err := catalog.ReadCheckoutPin(dest)
	if err != nil {
		return nil, err
	}
	if pin == nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "%s has never been checked out; use checkout first", dest)
	}
	reports, err := catalog.MountStatus(pin.Mounts)
	if err != nil {
		return nil, err
	}
	return map[string]any{"workspaceId": pin.WorkspaceID, "dir": homeRel(cx.Home, dest), "mounts": reports}, nil
}

func commitWorkspace(cx *invocation) (any, error) {
	workspaceID, err := workspaceIDFlag(cx.Flags)
	if err != nil {
		return nil, err
	}
	commandID, err := cx.require("command-id")
	if err != nil {
		return nil, err
	}
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	def, err := effectiveWorkspace(cx.WS, cx.Home, cat, workspaceID, cx.Flags)
	if err != nil {
		return nil, err
	}
	if !recipeHasMounts(def) {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "commit --workspace requires a workspace recipe with mount paths")
	}
	dest, err := checkoutDest(cx.WS, cx.Home, workspaceID, cx.Flags)
	if err != nil {
		return nil, err
	}
	pin, err := catalog.ReadCheckoutPin(dest)
	if err != nil {
		return nil, err
	}
	if pin == nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "%s has never been checked out; use checkout first", dest)
	}
	writes, err := catalog.CollectMountChanges(pin.Mounts)
	if err != nil {
		return nil, err
	}
	if len(writes) == 0 {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "no local changes to commit")
	}
	byRepo := map[kernel.RepositoryID][]catalog.MountWrite{}
	for _, w := range writes {
		byRepo[w.Repository] = append(byRepo[w.Repository], w)
	}
	selectorOf := map[kernel.RepositoryID]string{}
	pathOf := map[kernel.RepositoryID]string{}
	for _, src := range def.Sources {
		selectorOf[src.Repository] = src.Selector
		if src.Path != nil {
			pathOf[src.Repository] = strings.Trim(*src.Path, "/")
		}
	}
	var forbidden []string
	for repo, batch := range byRepo {
		if err := authorizeRoutedWrite(cx, repo); err != nil {
			label := string(repo)
			if p := pathOf[repo]; p != "" {
				label = p + " (" + string(repo) + ")"
			}
			forbidden = append(forbidden, fmt.Sprintf("%d files under %s", len(batch), label))
		}
	}
	if len(forbidden) > 0 {
		return nil, kernel.Fail(kernel.ErrForbidden,
			"%s belong to a mount with no write grant; use propose --path or revert those files",
			strings.Join(forbidden, "; "))
	}

	type one struct {
		Receipt writer.CommitReceipt `json:"receipt"`
		Error   string               `json:"error,omitempty"`
	}
	out := make([]one, 0, len(byRepo))
	nextMounts := append([]catalog.MountCheckout{}, pin.Mounts...)
	repos := make([]kernel.RepositoryID, 0, len(byRepo))
	for repo := range byRepo {
		repos = append(repos, repo)
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i] < repos[j] })
	for _, repo := range repos {
		batch := byRepo[repo]
		changes := make([]repository.RawFileChange, 0, len(batch))
		for _, w := range batch {
			changes = append(changes, repository.RawFileChange{Path: w.Path, Content: w.Content, Remove: w.Remove})
		}
		var base kernel.CommitID
		var workDir string
		for _, m := range pin.Mounts {
			if m.Repository == repo {
				base = m.Commit
				workDir = m.Dir
				break
			}
		}
		ref := selectorOf[repo]
		if ref == "" {
			ref = repository.DefaultRef
		}
		receipt, err := cx.WS.Writer.RawWrite(commandID+":"+string(repo), repository.RawFileChangeSet{
			TargetRepository:     repo,
			TargetRef:            ref,
			BaseCommit:           base,
			ExpectedTargetCommit: base,
			Changes:              changes,
			Message:              cx.flag("message"),
		})
		row := one{}
		if err != nil {
			row.Error = err.Error()
			out = append(out, row)
			continue
		}
		row.Receipt = receipt
		out = append(out, row)
		if workDir != "" {
			if resetErr := gitdir.At(workDir).ResetHard(string(receipt.Result.NewCommit)); resetErr != nil {
				return nil, resetErr
			}
		}
		for i, m := range nextMounts {
			if m.Repository == repo {
				nextMounts[i].Commit = receipt.Result.NewCommit
			}
		}
	}
	if err := catalog.WriteCheckoutPin(dest, catalog.CheckoutPin{WorkspaceID: pin.WorkspaceID, Revision: pin.Revision, Mounts: nextMounts}); err != nil {
		return nil, err
	}
	return map[string]any{"workspaceId": workspaceID, "commits": out}, nil
}

// inspectWorkspace composes Catalog state, the Snapshot pin, logical AccessSpecs
// and physical projection state.
// and index descriptors at this pin (not the live working projection).
func inspectWorkspace(ws *Home, flags map[string]FlagValue) (any, error) {
	cat, err := pickCatalog(ws, flags)
	if err != nil {
		return nil, err
	}
	workspaceID, err := workspaceIDFlag(flags)
	if err != nil {
		return nil, err
	}
	resolved, err := resolveOrReplay(ws, ws.Dir, cat, workspaceID, flags)
	if err != nil {
		return nil, err
	}
	pin := workspacePin(resolved)
	plan, err := reader.PlanAccess(cat.RequireKnowledge, pin)
	if err != nil {
		return nil, err
	}
	indexes := []any{}
	for _, spec := range plan.Specs {
		repo, err := ws.Store.Knowledge(spec.Repository, kernel.ErrUsageInvalid)
		if err != nil {
			return nil, err
		}
		desc, err := ws.Index.DescribeAt(repo, spec.Commit)
		if err != nil {
			return nil, err
		}
		indexes = append(indexes, desc)
	}
	return map[string]any{
		"catalog":    filterCatalogState(ws.Dir, flags, cat.DumpState()),
		"pin":        resolved,
		"accessPlan": plan,
		"indexes":    indexes,
	}, nil
}

func filterCatalogState(home string, flags map[string]FlagValue, state catalog.CatalogState) catalog.CatalogState {
	if ownerBypass(flags) {
		return catalog.NormalizeCatalogState(state)
	}
	visible := map[string]bool{}
	var repos []string
	for _, id := range state.Repositories {
		if allowedRepoRead(home, flags, id, "") {
			visible[id] = true
			repos = append(repos, id)
		}
	}
	var workspaces []catalog.WorkspaceDefinition
	for _, workspace := range state.Workspaces {
		var sources []catalog.WorkspaceSource
		for _, src := range workspace.Sources {
			if visible[string(src.Repository)] {
				sources = append(sources, src)
			}
		}
		if len(sources) == 0 {
			continue
		}
		copy := workspace
		copy.Sources = sources
		workspaces = append(workspaces, copy)
	}
	state.Repositories = repos
	state.Workspaces = workspaces
	return catalog.NormalizeCatalogState(state)
}
