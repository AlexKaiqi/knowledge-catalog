// Package filegit implements the local Git Snapshot authority.
package filegit

// FileGit is the local Snapshot authority (layer ⓪): Snapshot = git. The
// adapter also implements the optional layer ② Knowledge capability; physical
// retrieval providers live under retrieval/, never in this package.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kc/internal/gitdir"
	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

const (
	cfgRepositoryID = "kc.repositoryId"
	cfgDriver       = "kc.driver"
)

var (
	_ snapshot.Store       = (*FileGitRepository)(nil)
	_ snapshot.TreeStore   = (*FileGitRepository)(nil)
	_ knowledge.Repository = (*FileGitRepository)(nil)
)

// git plumbing lives in internal/gitdir so the Catalog registry can reuse it
// without importing this knowledge adapter.
func git(cwd string, args ...string) (string, error) {
	return gitdir.At(cwd).Git(args...)
}

func gitOK(cwd string, args ...string) bool {
	return gitdir.At(cwd).OK(args...)
}

// The path-escape guard for write targets is repofile.SafeRelativePath, applied
// in repofile.Apply. A second copy used to sit here, unreachable.

type FileGitRepository struct {
	repositoryID kernel.RepositoryID
	rootDir      string
	dir          *gitdir.Dir
}

func NewFileGit(rootDir string, repositoryID kernel.RepositoryID) (*FileGitRepository, error) {
	return OpenGitSnapshot(rootDir, repositoryID, "filegit")
}

// AttachGit opens an existing git directory as a Snapshot without initializing,
// stamping kc.repositoryId, or writing managed excludes. That is the
// "point at a Git Repository this tool does not own" case (docs/COMPOSITION.md): the
// directory stays a plain git clone for anyone who never installed kc.
func AttachGit(rootDir string, repositoryID kernel.RepositoryID) (*FileGitRepository, error) {
	if _, err := os.Stat(filepath.Join(rootDir, ".git")); err != nil {
		return nil, fmt.Errorf("%s is not a git directory", rootDir)
	}
	return &FileGitRepository{repositoryID: repositoryID, rootDir: rootDir, dir: gitdir.At(rootDir)}, nil
}

// OpenGitSnapshot opens a git-shaped knowledge Snapshot (tree/commit/ref/CAS).
// driver is stamped in git config (filegit or dolt).
func OpenGitSnapshot(rootDir string, repositoryID kernel.RepositoryID, driver string) (*FileGitRepository, error) {
	if driver == "" {
		driver = "filegit"
	}
	dir, err := gitdir.Open(rootDir, "")
	if err != nil {
		return nil, err
	}
	if err := stampFileGit(rootDir, repositoryID, driver); err != nil {
		return nil, err
	}
	return &FileGitRepository{repositoryID: repositoryID, rootDir: rootDir, dir: dir}, nil
}

func stampFileGit(rootDir string, repositoryID kernel.RepositoryID, driver string) error {
	if driver == "" {
		driver = "filegit"
	}
	existing, err := git(rootDir, "config", "--local", "--get", cfgRepositoryID)
	if err == nil && existing != "" && existing != string(repositoryID) {
		return fmt.Errorf("directory %s is stamped as %s, not %s", rootDir, existing, repositoryID)
	}
	if existing != string(repositoryID) {
		if _, err := git(rootDir, "config", "--local", cfgRepositoryID, string(repositoryID)); err != nil {
			return err
		}
	}
	existingDriver, _ := git(rootDir, "config", "--local", "--get", cfgDriver)
	if existingDriver == driver {
		return nil
	}
	_, err = git(rootDir, "config", "--local", cfgDriver, driver)
	return err
}

// ReadFileGitStamp returns the repository id and driver stored in git config.
func ReadFileGitStamp(rootDir string) (id, driver string, err error) {
	id, err = git(rootDir, "config", "--local", "--get", cfgRepositoryID)
	if err != nil || id == "" {
		return "", "", fmt.Errorf("no kc.repositoryId in %s", rootDir)
	}
	driver, _ = git(rootDir, "config", "--local", "--get", cfgDriver)
	if driver == "" {
		driver = "filegit"
	}
	return id, driver, nil
}

func (r *FileGitRepository) ID() kernel.RepositoryID { return r.repositoryID }
func (r *FileGitRepository) RootDir() string         { return r.rootDir }

const archivedRef = "refs/kc/archived"

func (r *FileGitRepository) Archived() bool {
	_, ok := r.GetRef(archivedRef)
	return ok
}

func (r *FileGitRepository) Archive() error {
	if r.Archived() {
		return nil
	}
	head, err := r.Head(gitdir.BranchRef(gitdir.DefaultBranch))
	if err != nil {
		return err
	}
	return r.CreateRef(archivedRef, head)
}

func (r *FileGitRepository) denyIfArchived() error {
	if r.Archived() {
		return kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", r.repositoryID)
	}
	return nil
}

func (r *FileGitRepository) Head(ref string) (kernel.CommitID, error) {
	if ref == "" {
		ref = "HEAD"
	}
	commit, ok := r.GetRef(ref)
	if !ok {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "ref %s does not exist", ref)
	}
	return commit, nil
}

func (r *FileGitRepository) GetRef(ref string) (kernel.CommitID, bool) {
	if !gitOK(r.rootDir, "rev-parse", "--verify", ref) {
		return "", false
	}
	out, err := git(r.rootDir, "rev-parse", ref)
	if err != nil {
		return "", false
	}
	return kernel.CommitID(out), true
}

func (r *FileGitRepository) everExisted(objectID knowledge.ObjectID) bool {
	prefix := "objects/" + string(objectID)
	raw, err := git(r.rootDir, "log", "--all", "--pretty=format:%H", "--", prefix, prefix+".json")
	return err == nil && raw != ""
}

func (r *FileGitRepository) HasCommit(commitID kernel.CommitID) bool {
	return r.dir.HasCommit(string(commitID))
}

func (r *FileGitRepository) CreateRef(ref string, commitID kernel.CommitID) error {
	if ref != archivedRef {
		if err := r.denyIfArchived(); err != nil {
			return err
		}
	}
	if _, ok := r.GetRef(ref); ok {
		return kernel.Fail(kernel.ErrPreconditionFailed, "ref %s already exists", ref)
	}
	if !gitOK(r.rootDir, "cat-file", "-e", string(commitID)+"^{commit}") {
		return kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commitID)
	}
	_, err := git(r.rootDir, "update-ref", ref, string(commitID))
	return err
}

func (r *FileGitRepository) Merge(targetRef string, candidate, expected kernel.CommitID) (kernel.CommitID, error) {
	if err := r.denyIfArchived(); err != nil {
		return "", err
	}
	checkedOut, _ := git(r.rootDir, "symbolic-ref", "-q", "HEAD")
	if !gitOK(r.rootDir, "merge-base", "--is-ancestor", string(expected), string(candidate)) {
		return "", kernel.Fail(kernel.ErrNonFastForward, "commit %s is not a descendant of %s", candidate, expected)
	}
	if !gitOK(r.rootDir, "update-ref", targetRef, string(candidate), string(expected)) {
		cur, _ := r.GetRef(targetRef)
		return "", kernel.Fail(kernel.ErrNonFastForward, "ref %s moved: expected commit %s, actual %s", targetRef, expected, cur)
	}
	if checkedOut == targetRef {
		_, _ = git(r.rootDir, "reset", "--hard", "-q", string(candidate))
	}
	return candidate, nil
}
func (r *FileGitRepository) scan() (*repofile.Tree, error) {
	idx := repofile.NewTree()
	err := filepath.WalkDir(r.rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() && (name == ".git" || name == "streams") {
			return filepath.SkipDir
		}
		if d.IsDir() || !repofile.KnowledgePath(name) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		parsed := repofile.Parse(string(b))
		if parsed == nil {
			return nil
		}
		rel, _ := filepath.Rel(r.rootDir, path)
		return repofile.Ingest(idx, parsed, rel)
	})
	return idx, err
}

func (r *FileGitRepository) scanAt(commitID kernel.CommitID) (*repofile.Tree, error) {
	if !r.HasCommit(commitID) {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commitID)
	}
	idx := repofile.NewTree()
	paths, err := r.dir.Paths(string(commitID))
	if err != nil {
		return nil, err
	}
	for _, rel := range paths {
		if !repofile.KnowledgePath(rel) {
			continue
		}
		content, err := r.dir.Show(string(commitID), rel)
		if err != nil {
			return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "failed to read %s at %s", rel, commitID)
		}
		parsed := repofile.Parse(content)
		if parsed == nil {
			continue
		}
		if err := repofile.Ingest(idx, parsed, rel); err != nil {
			return nil, err
		}
	}
	return idx, nil
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
