package cli

import (
	"fmt"
	"sort"
	"strings"

	"kc/catalog"
	"kc/internal/gitdir"
	"kc/kernel"
	"kc/snapshot"
	"kc/writer"
)

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
	pin, err := catalog.ReadMountCheckoutPin(dest)
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
		changes := make([]snapshot.TreeChange, 0, len(batch))
		for _, w := range batch {
			changes = append(changes, snapshot.TreeChange{Path: w.Path, Content: w.Content, Remove: w.Remove})
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
			ref = snapshot.DefaultRef
		}
		receipt, err := cx.WS.Writer.RawWrite(commandID+":"+string(repo), snapshot.TreeChangeSet{
			TargetRepository: repo, TargetRef: ref, BaseCommit: base, ExpectedTargetCommit: base,
			Changes: changes, Message: cx.flag("message"),
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
	if err := catalog.WriteMountCheckoutPin(dest, catalog.MountCheckoutPin{WorkspaceID: pin.WorkspaceID, Revision: pin.Revision, Mounts: nextMounts}); err != nil {
		return nil, err
	}
	return map[string]any{"workspaceId": workspaceID, "commits": out}, nil
}
