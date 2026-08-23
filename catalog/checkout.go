package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kc/internal/gitdir"
	"kc/kernel"
)

// localTree is the duck-typed capability a member must have for its pinned
// commit to become a real, writable git working tree on this machine: an
// on-disk git directory (local.FileGitRepository satisfies it). Native Dolt
// and remote Gitea deliberately do not: their VFS remains writable through
// RawFileStore, but neither pretends to be a local Git worktree.
// and catalog must not import those adapters to find out (docs/LAYERS.md) —
// so this asks the capability, not the type.
type localTree interface {
	RootDir() string
}

// MountCheckout is one mount's checkout outcome: which repository and commit
// it is pinned to, and where it landed. Path is normalized (root is "").
// Skipped is true when the member has no local git directory to check out a
// writable worktree from (e.g. a remote-only gitea Snapshot): Dir is then
// empty and Reason says why, rather than the whole checkout failing. A caller
// that can still read that member (has Knowledge — cli can, catalog cannot,
// see docs/LAYERS.md) may materialize a read-only export at Path itself;
// CheckoutMounts only decides layout. docs/COMPOSITION.md §3.5 states this
// same "report honestly, do not pretend it does not exist" rule for a mount
// unreachable for lack of permission; a mount unreachable for lack of local
// git capability is reported the same way, not treated as a harder failure.
type MountCheckout struct {
	Repository kernel.RepositoryID `json:"repository"`
	Path       string              `json:"path"`
	Dir        string              `json:"dir,omitempty"`
	Commit     kernel.CommitID     `json:"commit"`
	Skipped    bool                `json:"skipped,omitempty"`
	Reason     string              `json:"reason,omitempty"`
}

// CheckoutPinFile is Loom's own record of a checkout, written at root. It is
// not reader.CheckoutPinFile (².kc-pin.json for an assembled read-only knowledge
// export): this one records per-mount git coordinates and doubles as the
// marker that lets CheckoutMounts recognize an existing checkout and refuse
// cleanly, and that SyncMounts diffs against to tell an advance from a no-op.
const CheckoutPinFile = ".kc-pin.json"

// CheckoutPin is the CheckoutPinFile's shape.
type CheckoutPin struct {
	WorkspaceID string          `json:"workspaceId"`
	Revision    int             `json:"revision"`
	Mounts      []MountCheckout `json:"mounts"`
}

func WriteCheckoutPin(root string, pin CheckoutPin) error {
	raw, err := json.MarshalIndent(pin, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(filepath.Join(root, CheckoutPinFile), raw, 0o644)
}

// ReadCheckoutPin returns (nil, nil) when root has never been checked out —
// that is not an error, it is the fact CheckoutMounts and SyncMounts each
// branch on.
func ReadCheckoutPin(root string) (*CheckoutPin, error) {
	raw, err := os.ReadFile(filepath.Join(root, CheckoutPinFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var pin CheckoutPin
	if err := DecodeJSON(raw, &pin); err != nil {
		return nil, fmt.Errorf("%s is corrupt: %w", CheckoutPinFile, err)
	}
	return &pin, nil
}

// CheckoutMounts materializes every declared mount of a resolved Workspace as
// its own real git working tree under root, joined at each mount's Path. It
// does not invent a tree format: each mount is a linked git worktree off its
// member's own git directory, detached at the pinned commit, so status, diff
// and conflicts are git's from the start (docs/COMPOSITION.md §2.3).
//
// def must declare Path on every source (see ValidateMountPaths, enforced by
// DefineWorkspace); resolved must be the ResolveWorkspace pin naming def's repositories.
//
// When one mount declares the root path (Path: ""), that member's worktree
// lands exactly at root and every other mount nests inside it; root then has
// that member's .git, and its info/exclude is given entries for the sibling
// mount directories (and CheckoutPinFile) so its own `git status` does not
// report them. When no mount claims the root, root is a plain directory with
// no .git of its own.
//
// Calling this again on an already-checked-out root fails with USAGE_INVALID
// naming SyncMounts: re-running the same worktree-add would either collide
// with git's own bookkeeping or silently duplicate work, neither of which is
// "checkout" — advancing an existing checkout is a different operation with
// different rules (docs/COMPOSITION.md §3.1).
func (c *Catalog) CheckoutMounts(workspaceID, root string) ([]MountCheckout, error) {
	return c.CheckoutMountsAllowing(workspaceID, root, nil)
}

// CheckoutMountsAllowing is CheckoutMounts with a per-repository deny map:
// denied mounts are reported Skipped with that reason and never touch the
// disk (docs/COMPOSITION.md §3.4 — the agent boundary is at checkout time).
// nil/empty denied is CheckoutMounts.
func (c *Catalog) CheckoutMountsAllowing(workspaceID, root string, denied map[kernel.RepositoryID]string) ([]MountCheckout, error) {
	def, err := c.Workspace(workspaceID)
	if err != nil {
		return nil, err
	}
	return c.CheckoutMountsAllowingDef(def, root, denied)
}

func (c *Catalog) CheckoutMountsAllowingDef(def WorkspaceDefinition, root string, denied map[kernel.RepositoryID]string) ([]MountCheckout, error) {
	abs, resolved, err := c.prepareCheckout(def, root)
	if err != nil {
		return nil, err
	}
	prior, err := ReadCheckoutPin(abs)
	if err != nil {
		return nil, err
	}
	if prior != nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid,
			"%s is already checked out for workspace %s; use SyncMounts to advance it, not CheckoutMounts again", abs, prior.WorkspaceID)
	}
	out, err := c.materializeMounts(def, resolved, abs, denied)
	if err != nil {
		return nil, err
	}
	if err := WriteCheckoutPin(abs, CheckoutPin{WorkspaceID: def.WorkspaceID, Revision: def.Revision, Mounts: out}); err != nil {
		return nil, err
	}
	return out, nil
}

// prepareCheckout is the prelude shared by CheckoutMounts and SyncMounts:
// validate root, resolve the recipe, and hand back the absolute path every
// worktree operation must use (git worktree add resolves a relative dest
// against the *source* repo's directory, not the caller's cwd — a relative
// root would silently land each mount under the wrong tree).
func (c *Catalog) prepareCheckout(def WorkspaceDefinition, root string) (string, ResolvedWorkspace, error) {
	if strings.TrimSpace(root) == "" {
		return "", ResolvedWorkspace{}, kernel.Fail(kernel.ErrUsageInvalid, "checkout root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", ResolvedWorkspace{}, err
	}
	if err := requireAllMountsDeclared(def.Sources); err != nil {
		return "", ResolvedWorkspace{}, err
	}
	resolved, err := c.ResolveDefinition(def)
	if err != nil {
		return "", ResolvedWorkspace{}, err
	}
	return abs, resolved, nil
}

func requireAllMountsDeclared(sources []WorkspaceSource) error {
	for _, src := range sources {
		if src.Path == nil {
			return kernel.Fail(kernel.ErrUsageInvalid,
				"repository %s has no declared mount path; checkout needs a workspace recipe, not a federated-read recipe", src.Repository)
		}
	}
	return nil
}

// materializeMounts is CheckoutMounts' core: every source gets a fresh
// worktree (or, lacking the capability, a reserved directory and a Skipped
// report). root must be absolute; the caller has already checked it is not
// an existing checkout.
func (c *Catalog) materializeMounts(def WorkspaceDefinition, resolved ResolvedWorkspace, root string, denied map[kernel.RepositoryID]string) ([]MountCheckout, error) {
	sources := rootFirst(def.Sources)
	if err := ensureRootDir(sources, root); err != nil {
		return nil, err
	}
	out := make([]MountCheckout, 0, len(sources))
	for _, src := range sources {
		commit, ok := resolved.Repositories[src.Repository]
		if !ok {
			return nil, kernel.Fail(kernel.ErrWorkspaceInvalid, "resolved pin has no commit for repository %s", src.Repository)
		}
		mount, err := c.materializeOneMount(src, commit, root, denied[src.Repository])
		if err != nil {
			return nil, err
		}
		out = append(out, mount)
	}
	if err := c.refreshRootExclude(sources, out); err != nil {
		return nil, err
	}
	return out, nil
}

// materializeOneMount checks out a single mount fresh. root must already
// exist (materializeMounts' ensureRootDir, or an established checkout when
// called from SyncMounts for a mount newly added to the recipe).
func (c *Catalog) materializeOneMount(src WorkspaceSource, commit kernel.CommitID, root, denyReason string) (MountCheckout, error) {
	norm := normalizeMountPath(*src.Path)
	if denyReason != "" {
		return MountCheckout{
			Repository: src.Repository, Path: norm, Commit: commit,
			Skipped: true, Reason: denyReason,
		}, nil
	}
	snapshot, err := c.store.Require(src.Repository, kernel.ErrUsageInvalid)
	if err != nil {
		return MountCheckout{}, err
	}
	dest := filepath.Join(root, filepath.FromSlash(norm))
	tree, ok := snapshot.(localTree)
	if !ok {
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return MountCheckout{}, err
		}
		return MountCheckout{
			Repository: src.Repository, Path: norm, Commit: commit,
			Skipped: true,
			Reason:  fmt.Sprintf("repository %s has no local git directory; a writable worktree cannot be checked out here", src.Repository),
		}, nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return MountCheckout{}, err
	}
	if err := gitdir.At(tree.RootDir()).AddWorktree(dest, string(commit)); err != nil {
		return MountCheckout{}, fmt.Errorf("checkout mount %s at %s: %w", src.Repository, mountLabel(norm), err)
	}
	if err := gitdir.At(dest).SparseCheckout(src.SubPath); err != nil {
		return MountCheckout{}, fmt.Errorf("checkout mount %s subPath %s: %w", src.Repository, src.SubPath, err)
	}
	return MountCheckout{Repository: src.Repository, Path: norm, Dir: dest, Commit: commit}, nil
}

func ensureRootDir(sources []WorkspaceSource, root string) error {
	for _, src := range sources {
		if normalizeMountPath(*src.Path) == "" {
			return os.MkdirAll(filepath.Dir(root), 0o755)
		}
	}
	return os.MkdirAll(root, 0o755)
}

// rootFirst orders the root mount (if any) ahead of the rest: its worktree
// must exist before a nested mount's parent directory can be created under it.
func rootFirst(sources []WorkspaceSource) []WorkspaceSource {
	out := make([]WorkspaceSource, len(sources))
	copy(out, sources)
	for i, src := range out {
		if src.Path != nil && normalizeMountPath(*src.Path) == "" && i != 0 {
			out[0], out[i] = out[i], out[0]
			break
		}
	}
	return out
}

// refreshRootExclude is a no-op unless def has a root mount with a local
// git directory; it is safe to call after every CheckoutMounts and
// SyncMounts, including when nothing about the root mount changed —
// Exclude() already dedupes patterns it has already written.
func (c *Catalog) refreshRootExclude(sources []WorkspaceSource, mounts []MountCheckout) error {
	for _, src := range sources {
		if src.Path == nil || normalizeMountPath(*src.Path) != "" {
			continue
		}
		snapshot, err := c.store.Require(src.Repository, kernel.ErrUsageInvalid)
		if err != nil {
			return err
		}
		tree, ok := snapshot.(localTree)
		if !ok {
			return nil
		}
		return excludeSiblingMounts(tree.RootDir(), mounts)
	}
	return nil
}

// excludeSiblingMounts keeps the root mount's own git status quiet about the
// other mounts nested inside its working tree, and about CheckoutPinFile:
// info/exclude is per repository (shared by every worktree off it), so it is
// written once on the member's own directory, not on the checked-out
// worktree path.
func excludeSiblingMounts(rootMemberDir string, mounts []MountCheckout) error {
	patterns := []string{"/" + CheckoutPinFile}
	for _, m := range mounts {
		if m.Path == "" {
			continue
		}
		first, _, _ := strings.Cut(m.Path, "/")
		patterns = append(patterns, "/"+first)
	}
	return gitdir.At(rootMemberDir).Exclude(patterns...)
}

// SyncOutcome is what happened to one mount during Sync: a read of what
// changed, not a policy layered on top of it.
type SyncOutcome string

const (
	// SyncCheckedOut: a mount added to the recipe since the last checkout,
	// materialized now for the first time.
	SyncCheckedOut SyncOutcome = "checked-out"
	// SyncUnchanged: already at the newly resolved commit.
	SyncUnchanged SyncOutcome = "unchanged"
	// SyncAdvanced: moved forward in place; there were no local changes to lose.
	SyncAdvanced SyncOutcome = "advanced"
	// SyncBlocked: local changes exist; left alone, commit or discard first.
	SyncBlocked SyncOutcome = "blocked"
	// SyncSkipped: no local git directory, same as CheckoutMounts' Skipped.
	SyncSkipped SyncOutcome = "skipped"
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

// SyncMounts advances an existing checkout independently per mount
// (docs/COMPOSITION.md §3.1): a mount with no local changes moves straight to
// the newly resolved commit (CheckoutDetached in place — no remove/re-add);
// one with local changes is left exactly as it is and reported Blocked, so a
// sync never discards uncommitted work. There is no single version for the
// composed tree, only per-mount outcomes.
//
// root must already be a CheckoutMounts result: SyncMounts on a directory
// that was never checked out fails with USAGE_INVALID naming CheckoutMounts
// instead. A mount whose Path changed since the last checkout, or that would
// now claim the root when the checkout was not rooted (or vice versa), also
// fails with USAGE_INVALID: that is a recipe shape change, not a version
// advance, and Sync does not restructure an existing tree — re-checkout from
// scratch.
func (c *Catalog) SyncMounts(workspaceID, root string) ([]MountSync, error) {
	def, err := c.Workspace(workspaceID)
	if err != nil {
		return nil, err
	}
	return c.SyncMountsDef(def, root)
}

func (c *Catalog) SyncMountsDef(def WorkspaceDefinition, root string) ([]MountSync, error) {
	abs, resolved, err := c.prepareCheckout(def, root)
	if err != nil {
		return nil, err
	}
	prior, err := ReadCheckoutPin(abs)
	if err != nil {
		return nil, err
	}
	if prior == nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid,
			"%s has never been checked out for workspace %s; use CheckoutMounts first", abs, def.WorkspaceID)
	}
	priorByRepo := make(map[kernel.RepositoryID]MountCheckout, len(prior.Mounts))
	for _, m := range prior.Mounts {
		priorByRepo[m.Repository] = m
	}

	sources := rootFirst(def.Sources)
	out := make([]MountSync, 0, len(sources))
	next := make([]MountCheckout, 0, len(sources))
	for _, src := range sources {
		commit, ok := resolved.Repositories[src.Repository]
		if !ok {
			return nil, kernel.Fail(kernel.ErrWorkspaceInvalid, "resolved pin has no commit for repository %s", src.Repository)
		}
		norm := normalizeMountPath(*src.Path)
		was, seen := priorByRepo[src.Repository]
		if !seen {
			mount, err := c.materializeOneMount(src, commit, abs, "")
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
		if was.Path != norm {
			return nil, kernel.Fail(kernel.ErrUsageInvalid,
				"repository %s moved from mount path %s to %s since %s was checked out; re-checkout from scratch",
				src.Repository, mountLabel(was.Path), mountLabel(norm), abs)
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
	if err := WriteCheckoutPin(abs, CheckoutPin{WorkspaceID: def.WorkspaceID, Revision: def.Revision, Mounts: next}); err != nil {
		return nil, err
	}
	if err := c.refreshRootExclude(sources, next); err != nil {
		return nil, err
	}
	return out, nil
}

// syncOneMount decides and applies the outcome for one already-checked-out,
// non-skipped mount: dirty wins over everything (nothing is discarded), then
// unchanged-vs-advance is just whether the resolved commit moved.
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

// MountStatusReport is one mount's local git status: a read primitive, not a
// sync or write-back decision. `kc status` (or an equivalent) lines these up
// per mount; docs/COMPOSITION.md §2.3 — there is no single status for the
// composed tree because there is no single git repository underneath it. A
// Skipped mount (see MountCheckout) carries no git status; Dirty stays false.
type MountStatusReport struct {
	Repository kernel.RepositoryID `json:"repository"`
	Path       string              `json:"path"`
	Commit     kernel.CommitID     `json:"commit"`
	Dirty      bool                `json:"dirty"`
	Changed    []string            `json:"changed,omitempty"`
	Skipped    bool                `json:"skipped,omitempty"`
}

// MountStatus runs git status in each checked-out mount's own working tree.
// It does not touch the Catalog or any Snapshot; callers already hold the
// MountCheckout list from CheckoutMounts.
func MountStatus(mounts []MountCheckout) ([]MountStatusReport, error) {
	out := make([]MountStatusReport, 0, len(mounts))
	for _, m := range mounts {
		if m.Skipped {
			out = append(out, MountStatusReport{Repository: m.Repository, Path: m.Path, Commit: m.Commit, Skipped: true})
			continue
		}
		raw, err := gitdir.At(m.Dir).Git("status", "--porcelain")
		if err != nil {
			return nil, fmt.Errorf("status mount %s at %s: %w", m.Repository, mountLabel(m.Path), err)
		}
		var changed []string
		for _, line := range strings.Split(raw, "\n") {
			if strings.TrimSpace(line) != "" {
				changed = append(changed, line)
			}
		}
		out = append(out, MountStatusReport{
			Repository: m.Repository,
			Path:       m.Path,
			Commit:     m.Commit,
			Dirty:      len(changed) > 0,
			Changed:    changed,
		})
	}
	return out, nil
}

// MountWrite is one dirty path in a checked-out mount, ready to feed
// Writer.RawWrite: repository-internal path, literal bytes (or Remove).
// Catalog does not interpret frontmatter; grouping and CAS live with the caller.
type MountWrite struct {
	Repository kernel.RepositoryID `json:"repository"`
	Path       string              `json:"path"`
	Content    []byte              `json:"content,omitempty"`
	Remove     bool                `json:"remove,omitempty"`
}

// CollectMountChanges reads each non-skipped mount's git status and the
// bytes (or deletion) of every changed path. It does not write and it does
// not decide permission — that is the CLI's job, after this returns.
func CollectMountChanges(mounts []MountCheckout) ([]MountWrite, error) {
	var out []MountWrite
	for _, m := range mounts {
		if m.Skipped || m.Dir == "" {
			continue
		}
		changes, err := gitdir.At(m.Dir).PorcelainChanges()
		if err != nil {
			return nil, fmt.Errorf("collect mount %s at %s: %w", m.Repository, mountLabel(m.Path), err)
		}
		for _, ch := range changes {
			writes, err := collectOneChange(m, ch)
			if err != nil {
				return nil, err
			}
			out = append(out, writes...)
		}
	}
	return out, nil
}

func collectOneChange(m MountCheckout, ch gitdir.WorktreeChange) ([]MountWrite, error) {
	rel := strings.Trim(ch.Path, "/")
	if ch.Removed {
		return []MountWrite{{Repository: m.Repository, Path: rel, Remove: true}}, nil
	}
	full := filepath.Join(m.Dir, filepath.FromSlash(rel))
	info, err := os.Stat(full)
	if err != nil {
		return nil, fmt.Errorf("stat %s in mount %s: %w", rel, m.Repository, err)
	}
	if !info.IsDir() {
		raw, err := os.ReadFile(full)
		if err != nil {
			return nil, fmt.Errorf("read %s in mount %s: %w", rel, m.Repository, err)
		}
		return []MountWrite{{Repository: m.Repository, Path: rel, Content: raw}}, nil
	}
	var out []MountWrite
	err = filepath.WalkDir(full, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		relFile, err := filepath.Rel(m.Dir, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out = append(out, MountWrite{Repository: m.Repository, Path: filepath.ToSlash(relFile), Content: raw})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s in mount %s: %w", rel, m.Repository, err)
	}
	return out, nil
}
