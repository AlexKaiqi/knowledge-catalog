package worktree

import (
	"fmt"

	"kc/catalog"
	"kc/internal/gitdir"
	"kc/kernel"
)

// SyncOutcome is what happened to one mount during Sync: a read of what
// changed, not a policy layered on top of it.
type SyncOutcome string

const (
	SyncCheckedOut SyncOutcome = "checked-out"
	SyncUnchanged  SyncOutcome = "unchanged"
	SyncAdvanced   SyncOutcome = "advanced"
	SyncBlocked    SyncOutcome = "blocked"
	SyncSkipped    SyncOutcome = "skipped"
)

// MountSync is one mount's outcome from SyncMounts.
type MountSync struct {
	Repository kernel.RepositoryID `json:"repository"`
	Path       string              `json:"path"`
	Dir        string              `json:"dir,omitempty"`
	From       kernel.CommitID     `json:"from,omitempty"`
	To         kernel.CommitID     `json:"to"`
	Outcome    SyncOutcome         `json:"outcome"`
	Reason     string              `json:"reason,omitempty"`
}

// SyncMounts advances an existing checkout independently per mount. Dirty
// mounts are left untouched; clean mounts move to the newly resolved commit.
func SyncMounts(c *catalog.Catalog, workspaceID, root string) ([]MountSync, error) {
	def, err := c.Workspace(workspaceID)
	if err != nil {
		return nil, err
	}
	return SyncMountsDef(c, def, root)
}

func SyncMountsDef(c *catalog.Catalog, def catalog.WorkspaceDefinition, root string) ([]MountSync, error) {
	abs, resolved, err := prepareCheckout(c, def, root)
	if err != nil {
		return nil, err
	}
	prior, err := ReadMountCheckoutPin(abs)
	if err != nil {
		return nil, err
	}
	if prior == nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid,
			"%s has never been checked out for workspace %s; use CheckoutMounts first", abs, def.WorkspaceID)
	}
	type mountKey struct {
		repository kernel.RepositoryID
		path       string
	}
	priorByMount := make(map[mountKey]MountCheckout, len(prior.Mounts))
	for _, m := range prior.Mounts {
		priorByMount[mountKey{repository: m.Repository, path: m.Path}] = m
	}

	sources := catalog.RootFirst(def.Sources)
	current := map[mountKey]bool{}
	for _, src := range sources {
		current[mountKey{repository: src.Repository, path: catalog.NormalizeMountPath(*src.Path)}] = true
	}
	for key := range priorByMount {
		if !current[key] {
			return nil, kernel.Fail(kernel.ErrUsageInvalid,
				"repository %s was removed or moved from mount path %s since %s was checked out; re-checkout from scratch",
				key.repository, mountLabel(key.path), abs)
		}
	}
	out := make([]MountSync, 0, len(sources))
	next := make([]MountCheckout, 0, len(sources))
	for _, src := range sources {
		commit, ok := resolved.Repositories[src.Repository]
		if !ok {
			return nil, kernel.Fail(kernel.ErrWorkspaceInvalid, "resolved pin has no commit for repository %s", src.Repository)
		}
		norm := catalog.NormalizeMountPath(*src.Path)
		key := mountKey{repository: src.Repository, path: norm}
		was, seen := priorByMount[key]
		if !seen {
			mount, err := materializeOneMount(c, src, commit, abs, "")
			if err != nil {
				return nil, err
			}
			next = append(next, mount)
			sync := MountSync{Repository: mount.Repository, Path: mount.Path, Dir: mount.Dir, To: commit, Outcome: SyncCheckedOut}
			if mount.Skipped {
				sync.Outcome, sync.Reason = SyncSkipped, mount.Reason
			}
			out = append(out, sync)
			continue
		}
		if was.Skipped {
			next = append(next, MountCheckout{Repository: src.Repository, Path: norm, Commit: commit, Skipped: true, Reason: was.Reason})
			out = append(out, MountSync{Repository: src.Repository, Path: norm, To: commit, Outcome: SyncSkipped, Reason: was.Reason})
			continue
		}
		mount, sync, err := syncOneMount(src.Repository, norm, was, commit)
		if err != nil {
			return nil, err
		}
		next = append(next, mount)
		out = append(out, sync)
	}
	if err := WriteMountCheckoutPin(abs, MountCheckoutPin{WorkspaceID: def.WorkspaceID, Revision: def.Revision, Mounts: next}); err != nil {
		return nil, err
	}
	if err := refreshRootExclude(c, sources, next); err != nil {
		return nil, err
	}
	return out, nil
}

func syncOneMount(repo kernel.RepositoryID, path string, was MountCheckout, commit kernel.CommitID) (MountCheckout, MountSync, error) {
	dir := gitdir.At(was.Dir)
	switch {
	case dir.Dirty():
		return was, MountSync{
			Repository: repo, Path: path, Dir: was.Dir, From: was.Commit, To: commit,
			Outcome: SyncBlocked, Reason: "local changes; commit or discard before syncing",
		}, nil
	case was.Commit == commit:
		return was, MountSync{Repository: repo, Path: path, Dir: was.Dir, From: was.Commit, To: commit, Outcome: SyncUnchanged}, nil
	default:
		if err := dir.CheckoutDetached(string(commit)); err != nil {
			return MountCheckout{}, MountSync{}, fmt.Errorf("sync mount %s at %s: %w", repo, mountLabel(path), err)
		}
		advanced := MountCheckout{Repository: repo, Path: path, Dir: was.Dir, Commit: commit}
		return advanced, MountSync{Repository: repo, Path: path, Dir: was.Dir, From: was.Commit, To: commit, Outcome: SyncAdvanced}, nil
	}
}
