package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kc/catalog"
	"kc/catalog/worktree"
	"kc/internal/gitdir"
	"kc/kernel"
	"kc/knowledge/reader"
	"kc/snapshot"
	"kc/snapshot/treewriter"
)

type workspaceCommitPlan struct {
	workspaceID  string
	commandID    string
	dest         string
	pin          *worktree.MountCheckoutPin
	writes       map[kernel.RepositoryID][]worktree.MountWrite
	selectors    map[kernel.RepositoryID]string
	paths        map[kernel.RepositoryID][]string
	repositories []kernel.RepositoryID
}

type workspaceCommitResult struct {
	Repository kernel.RepositoryID `json:"repository"`
	Receipt    treewriter.Receipt  `json:"receipt"`
	Error      string              `json:"error,omitempty"`
}

func commitWorkspace(cx *invocation) (any, error) {
	plan, err := prepareWorkspaceCommit(cx)
	if err != nil {
		return nil, err
	}
	if err := authorizeWorkspaceCommit(cx, plan); err != nil {
		return nil, err
	}
	rows, nextMounts, failed, err := applyWorkspaceCommit(cx, plan)
	if err != nil {
		return nil, err
	}
	if err := worktree.WriteMountCheckoutPin(plan.dest, worktree.MountCheckoutPin{
		WorkspaceID: plan.pin.WorkspaceID, Revision: plan.pin.Revision, Mounts: nextMounts,
	}); err != nil {
		return nil, err
	}
	outcome := "complete"
	if failed > 0 {
		outcome = "partial"
	}
	return map[string]any{"workspaceId": plan.workspaceID, "outcome": outcome, "commits": rows}, nil
}

func prepareWorkspaceCommit(cx *invocation) (workspaceCommitPlan, error) {
	workspaceID, err := workspaceIDFlag(cx.Flags)
	if err != nil {
		return workspaceCommitPlan{}, err
	}
	commandID, err := cx.require("command-id")
	if err != nil {
		return workspaceCommitPlan{}, err
	}
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return workspaceCommitPlan{}, err
	}
	def, err := effectiveWorkspace(cx.WS, cx.Home, cat, workspaceID, cx.Flags)
	if err != nil {
		return workspaceCommitPlan{}, err
	}
	if !recipeHasMounts(def) {
		return workspaceCommitPlan{}, kernel.Fail(kernel.ErrUsageInvalid, "commit --workspace requires a workspace recipe with mount paths")
	}
	dest, err := checkoutDest(cx.WS, cx.Home, workspaceID, cx.Flags)
	if err != nil {
		return workspaceCommitPlan{}, err
	}
	pin, err := worktree.ReadMountCheckoutPin(dest)
	if err != nil {
		return workspaceCommitPlan{}, err
	}
	if pin == nil {
		return workspaceCommitPlan{}, kernel.Fail(kernel.ErrUsageInvalid, "%s has no host worktree pin; writer commit --workspace requires an existing mount checkout", dest)
	}
	writes, err := worktree.CollectMountChanges(pin.Mounts)
	if err != nil {
		return workspaceCommitPlan{}, err
	}
	if len(writes) == 0 {
		return workspaceCommitPlan{}, kernel.Fail(kernel.ErrUsageInvalid, "no local changes to commit")
	}
	byRepo := map[kernel.RepositoryID][]worktree.MountWrite{}
	for _, w := range writes {
		byRepo[w.Repository] = append(byRepo[w.Repository], w)
	}
	selectorOf := map[kernel.RepositoryID]string{}
	pathsOf := map[kernel.RepositoryID][]string{}
	for _, src := range def.Sources {
		selectorOf[src.Repository] = src.Selector
		if src.Path != nil {
			pathsOf[src.Repository] = append(pathsOf[src.Repository], strings.Trim(*src.Path, "/"))
		}
	}
	repositories := make([]kernel.RepositoryID, 0, len(byRepo))
	for repo := range byRepo {
		repositories = append(repositories, repo)
	}
	sort.Slice(repositories, func(i, j int) bool { return repositories[i] < repositories[j] })
	return workspaceCommitPlan{
		workspaceID: workspaceID, commandID: commandID, dest: dest, pin: pin,
		writes: byRepo, selectors: selectorOf, paths: pathsOf, repositories: repositories,
	}, nil
}

func authorizeWorkspaceCommit(cx *invocation, plan workspaceCommitPlan) error {
	var forbidden []string
	for repo, batch := range plan.writes {
		if err := authorizeRoutedWrite(cx, repo); err != nil {
			label := string(repo)
			if paths := plan.paths[repo]; len(paths) > 0 {
				label = strings.Join(paths, ", ") + " (" + string(repo) + ")"
			}
			forbidden = append(forbidden, fmt.Sprintf("%d files under %s", len(batch), label))
		}
	}
	if len(forbidden) > 0 {
		return kernel.Fail(kernel.ErrForbidden,
			"%s belong to a mount with no write grant; use propose --path or revert those files",
			strings.Join(forbidden, "; "))
	}
	return nil
}

func applyWorkspaceCommit(cx *invocation, plan workspaceCommitPlan) ([]workspaceCommitResult, []worktree.MountCheckout, int, error) {
	out := make([]workspaceCommitResult, 0, len(plan.writes))
	failed := 0
	nextMounts := append([]worktree.MountCheckout{}, plan.pin.Mounts...)
	for _, repo := range plan.repositories {
		batch := plan.writes[repo]
		changes := make([]snapshot.TreeChange, 0, len(batch))
		for _, w := range batch {
			changes = append(changes, snapshot.TreeChange{Path: w.Path, Content: w.Content, Remove: w.Remove})
		}
		var base kernel.CommitID
		var workDirs []string
		for _, m := range plan.pin.Mounts {
			if m.Repository == repo {
				if base == "" {
					base = m.Commit
				}
				if m.Dir != "" {
					workDirs = append(workDirs, m.Dir)
				}
			}
		}
		ref := plan.selectors[repo]
		if ref == "" {
			ref = snapshot.DefaultRef
		}
		receipt, err := cx.WS.TreeWriter.Commit(plan.commandID+":"+string(repo), snapshot.TreeChangeSet{
			TargetRepository: repo, TargetRef: ref, BaseCommit: base, ExpectedTargetCommit: base,
			Changes: changes, Message: cx.flag("message"),
		})
		row := workspaceCommitResult{Repository: repo}
		if err != nil {
			row.Error = err.Error()
			out = append(out, row)
			failed++
			continue
		}
		row.Receipt = receipt
		out = append(out, row)
		for _, workDir := range workDirs {
			if resetErr := gitdir.At(workDir).ResetHard(string(receipt.Result.NewCommit)); resetErr != nil {
				return nil, nil, failed, resetErr
			}
		}
		for i, m := range nextMounts {
			if m.Repository == repo {
				nextMounts[i].Commit = receipt.Result.NewCommit
			}
		}
	}
	return out, nextMounts, failed, nil
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
