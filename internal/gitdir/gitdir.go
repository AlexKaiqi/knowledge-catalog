// Package gitdir is git plumbing for a directory that has a working tree:
// init, config stamp, refs, tree reads, worktree commit, log.
//
// It knows commits, refs and file bytes. It does not know object_id, Aspect or
// Workspace, so both the layer ⓪ knowledge Snapshot adapter (local.FileGitRepository)
// and the layer ① Catalog registry can sit on it without depending on each other.
package gitdir

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultBranch is the branch git init leaves behind, and the one a worktree
// commit checks out. Ref form is repository.DefaultRef.
const DefaultBranch = "main"

// Dir is one git working directory. Methods shell out to git; there is no
// long-lived handle to keep in sync.
type Dir struct {
	root string
}

// At wraps an existing directory without touching it.
func At(root string) *Dir { return &Dir{root: root} }

// Open creates the directory and an empty git repository with a root commit if
// it is not one yet. Calling it on an existing repository only ensures excludes.
func Open(root string, excludes ...string) (*Dir, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	d := At(root)
	if _, err := os.Stat(filepath.Join(root, ".git")); os.IsNotExist(err) {
		if err := exec.Command("git", "-C", root, "init", "-q").Run(); err != nil {
			return nil, err
		}
		_, _ = d.Git("branch", "-M", DefaultBranch)
		if _, err := d.Commit(Signature{Message: "root"}, true); err != nil {
			return nil, err
		}
	}
	if err := d.Exclude(excludes...); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Dir) Root() string { return d.root }

// Git runs one git command in the directory and returns trimmed stdout.
// On failure the error carries git's own stderr line (e.g. "fatal: 'x'
// already exists"): cmd.Output alone leaves that in the *exec.ExitError's
// Stderr field, and nothing else in this package ever unwraps it, so every
// caller of Git/OK used to see only "exit status N".
func (d *Dir) Git(args ...string) (string, error) {
	out, err := d.gitRaw(args...)
	return strings.TrimSpace(string(out)), err
}

// gitRaw is Git without the TrimSpace, for byte-exact consumers (ShowRaw).
func (d *Dir) gitRaw(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = d.root
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if msg := strings.TrimSpace(string(exitErr.Stderr)); msg != "" {
				err = fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
			}
		}
	}
	return out, err
}

// OK reports whether a git command exits zero. Use it for existence probes.
func (d *Dir) OK(args ...string) bool {
	_, err := d.Git(args...)
	return err == nil
}

// Rev resolves a ref (or "HEAD" when empty) to a commit id.
func (d *Dir) Rev(ref string) (string, bool) {
	if ref == "" {
		ref = "HEAD"
	}
	if !d.OK("rev-parse", "--verify", ref) {
		return "", false
	}
	out, err := d.Git("rev-parse", ref)
	if err != nil {
		return "", false
	}
	return out, true
}

// HasCommit reports whether a commit object exists.
func (d *Dir) HasCommit(commit string) bool {
	return commit != "" && d.OK("cat-file", "-e", commit+"^{commit}")
}

func (d *Dir) Config(key string) (string, error) {
	return d.Git("config", "--local", "--get", key)
}

func (d *Dir) SetConfig(key, value string) error {
	_, err := d.Git("config", "--local", key, value)
	return err
}

// Exclude appends patterns to .git/info/exclude so packing directories beside
// the tree (streams/) never enter a commit.
func (d *Dir) Exclude(patterns ...string) error {
	if len(patterns) == 0 {
		return nil
	}
	path := filepath.Join(d.root, ".git", "info", "exclude")
	current, _ := os.ReadFile(path)
	text := string(current)
	changed := false
	for _, pattern := range patterns {
		if strings.Contains("\n"+text+"\n", "\n"+pattern+"\n") {
			continue
		}
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += pattern + "\n"
		changed = true
	}
	if !changed {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

// Paths lists every blob path in a commit tree.
func (d *Dir) Paths(rev string) ([]string, error) {
	raw, err := d.Git("ls-tree", "-r", "--name-only", rev)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out, nil
}

// Show reads one blob at a commit, trimmed. Frontmatter/text callers want the
// trim; a byte-exact consumer (a virtual filesystem serving raw content, e.g.
// docs/COMPOSITION.md's RawFileStore) wants ShowRaw instead.
func (d *Dir) Show(rev, path string) (string, error) {
	return d.Git("show", rev+":"+path)
}

// ShowRaw reads one blob at a commit exactly as git returns it: no
// TrimSpace, so trailing newlines and whitespace survive a round trip.
// git show <rev>:<path> succeeds for a tree path too (it prints a listing),
// so a caller that must reject directories needs ObjectType first — ShowRaw
// itself stays a raw plumbing primitive, not a "this must be a file" guard.
func (d *Dir) ShowRaw(rev, path string) ([]byte, error) {
	return d.gitRaw("show", rev+":"+path)
}

// ObjectType reports the git object type ("blob", "tree", "commit") at path
// in rev, or ok=false when nothing exists there. This is the guard a
// file-only reader (ShowRaw's callers) needs: git show <rev>:<path> does not
// fail for a directory, it prints that tree's listing as if it were content.
func (d *Dir) ObjectType(rev, path string) (kind string, ok bool) {
	out, err := d.Git("cat-file", "-t", rev+":"+path)
	if err != nil {
		return "", false
	}
	return out, true
}

// CheckedOutRef is the ref HEAD currently points at, empty when detached.
func (d *Dir) CheckedOutRef() string {
	ref, _ := d.Git("symbolic-ref", "-q", "HEAD")
	return ref
}

// Checkout switches to a branch name (not a ref path).
func (d *Dir) Checkout(name string) error {
	_, err := d.Git("checkout", "-q", name)
	return err
}

// AddWorktree creates a linked working tree at dest, detached at commit. dest
// must not already exist (git worktree add creates the leaf directory itself).
// Detached, not on a branch: two worktrees off the same Dir cannot share a
// branch checkout, and a mount is pinned to a commit, not a moving branch.
func (d *Dir) AddWorktree(dest, commit string) error {
	_, err := d.Git("worktree", "add", "--detach", "-q", dest, commit)
	return err
}

// RemoveWorktree deletes a linked working tree created by AddWorktree,
// discarding any uncommitted changes in it.
func (d *Dir) RemoveWorktree(dest string) error {
	_, err := d.Git("worktree", "remove", "--force", dest)
	return err
}

// CheckoutDetached moves d, a linked worktree already created by AddWorktree
// (also detached), to a different commit — advancing a mount in place without
// removing and recreating it. Only safe on a clean tree; callers check Dirty
// first so nothing is silently lost.
func (d *Dir) CheckoutDetached(commit string) error {
	_, err := d.Git("checkout", "--detach", "-q", commit)
	return err
}

// Dirty reports whether the working tree or index differs from HEAD.
func (d *Dir) Dirty() bool {
	out, _ := d.Git("status", "--porcelain")
	return out != ""
}

// SparseCheckout limits this working tree to subPath. Empty subPath is a
// no-op. Written through core.sparseCheckout + info/sparse-checkout rather
// than `sparse-checkout set --cone`, because cone mode keeps files in every
// parent of the cone (including the repo root) and that is exactly what a
// mount SubPath must hide.
func (d *Dir) SparseCheckout(subPath string) error {
	subPath = strings.Trim(subPath, "/")
	if subPath == "" || subPath == "." {
		return nil
	}
	if _, err := d.Git("config", "core.sparseCheckout", "true"); err != nil {
		return err
	}
	path, err := d.Git("rev-parse", "--git-path", "info/sparse-checkout")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(d.root, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := "/" + subPath + "/\n/" + subPath + "/**\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}
	_, err = d.Git("read-tree", "-mu", "HEAD")
	return err
}

// ResetHard moves HEAD and the working tree to commit, discarding local
// changes. Used after a Writer.RawWrite so a linked worktree matches the
// commit that just landed in the object store — the worktree was dirty with
// the same bytes, and leaving it dirty would make the next status lie.
func (d *Dir) ResetHard(commit string) error {
	_, err := d.Git("reset", "--hard", "-q", commit)
	return err
}

// WorktreeChange is one path git status --porcelain reported in this tree.
type WorktreeChange struct {
	Path    string
	Orig    string
	Removed bool
}

// PorcelainChanges parses `git status --porcelain` into per-path writes and
// deletes (renames become a delete of Orig plus a write of Path). Paths are
// repository-relative. Untracked files are writes.
func (d *Dir) PorcelainChanges() ([]WorktreeChange, error) {
	raw, err := d.Git("status", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parsePorcelain(raw), nil
}

func parsePorcelain(raw string) []WorktreeChange {
	var out []WorktreeChange
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) < 4 {
			continue
		}
		xy := line[:2]
		rest := strings.TrimPrefix(line[2:], " ")
		path, orig := rest, ""
		if xy[0] == 'R' || xy[0] == 'C' || xy[1] == 'R' || xy[1] == 'C' {
			if from, to, ok := strings.Cut(rest, " -> "); ok {
				orig, path = from, to
			}
		}
		path = unquotePorcelain(path)
		orig = unquotePorcelain(orig)
		removed := xy[0] == 'D' || xy[1] == 'D'
		if orig != "" {
			out = append(out, WorktreeChange{Path: orig, Removed: true})
			out = append(out, WorktreeChange{Path: path})
			continue
		}
		out = append(out, WorktreeChange{Path: path, Removed: removed})
	}
	return out
}

func unquotePorcelain(p string) string {
	p = strings.TrimSpace(p)
	if len(p) >= 2 && p[0] == '"' && p[len(p)-1] == '"' {
		if u, err := strconv.Unquote(p); err == nil {
			return u
		}
	}
	return p
}

// StageAll stages every change including deletions.
func (d *Dir) StageAll() error {
	_, err := d.Git("add", "-A")
	return err
}

// Commit records the staged tree. It does not stage; call StageAll first.
func (d *Dir) Commit(sig Signature, allowEmpty bool) (string, error) {
	name, email, message := sig.Format()
	args := []string{"-c", "user.name=" + name, "-c", "user.email=" + email, "commit", "-q", "-m", message}
	if allowEmpty {
		args = append(args, "--allow-empty")
	}
	if _, err := d.Git(args...); err != nil {
		return "", err
	}
	head, ok := d.Rev("")
	if !ok {
		return "", fmt.Errorf("commit in %s left no HEAD", d.root)
	}
	return head, nil
}

// CommitWorktree stages whatever is in the working tree and commits it onto the
// default branch, refusing when the branch moved away from expected. It returns
// the unchanged head when the tree is already clean.
//
// This is the write path for flat config files (Catalog registry YAML). Knowledge
// objects go through a Writer COMMIT, which builds its own change set.
func (d *Dir) CommitWorktree(expected string, sig Signature) (string, error) {
	current, ok := d.Rev(BranchRef(DefaultBranch))
	if !ok {
		return "", fmt.Errorf("branch %s does not exist in %s", DefaultBranch, d.root)
	}
	if expected != "" && current != expected {
		return "", ErrMoved{Ref: BranchRef(DefaultBranch), Expected: expected, Actual: current}
	}
	if err := d.Checkout(DefaultBranch); err != nil {
		return "", err
	}
	if err := d.StageAll(); err != nil {
		return "", err
	}
	if !d.Dirty() {
		return current, nil
	}
	return d.Commit(sig, false)
}

// ErrMoved is a compare-and-set failure on a ref. Callers map it to their own
// protocol code (kernel.ErrNonFastForward) so this package stays error-code free.
type ErrMoved struct {
	Ref      string
	Expected string
	Actual   string
}

func (e ErrMoved) Error() string {
	return fmt.Sprintf("ref %s moved: expected commit %s, actual %s", e.Ref, e.Expected, e.Actual)
}

// BranchRef is the full ref path of a branch name.
func BranchRef(name string) string { return "refs/heads/" + name }

// BranchName is the branch name of a ref path.
func BranchName(ref string) string { return strings.TrimPrefix(ref, "refs/heads/") }

const (
	recordSep = "\x1e"
	fieldSep  = "\x1f"
)

// LogEntry is one commit with the kc trailers already split out.
type LogEntry struct {
	Commit    string `json:"commit"`
	Author    string `json:"author,omitempty"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"`
	RuleID    string `json:"ruleId,omitempty"`
}

// Log reads commit history, newest first, optionally limited to one path.
// An unborn branch or missing path yields an empty slice, not an error.
func (d *Dir) Log(limit int, path string) ([]LogEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	args := []string{"log", "-" + strconv.Itoa(limit), "--format=%H" + fieldSep + "%an" + fieldSep + "%s" + fieldSep + "%b" + recordSep}
	if path != "" {
		args = append(args, "--", path)
	}
	raw, err := d.Git(args...)
	if err != nil {
		return []LogEntry{}, nil
	}
	entries := []LogEntry{}
	for _, record := range strings.Split(strings.Trim(raw, recordSep+"\n "), recordSep) {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}
		parts := strings.SplitN(record, fieldSep, 4)
		if len(parts) < 3 {
			continue
		}
		entry := LogEntry{Commit: parts[0], Author: parts[1], Message: parts[2]}
		if len(parts) > 3 {
			entry.RequestID, entry.RuleID = ParseTrailers(parts[3])
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
