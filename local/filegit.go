// Package local is the portable store set: FileGit Snapshot + JSONLStream + SQLite index.
// Not native Dolt SQL, not StarRocks, not Redis.
package local

// FileGit is the local Snapshot authority (layer ⓪): Snapshot = git.
// Knowledge interpretation (object_id / Aspect, layer ②) currently lives in
// this adapter. APPEND is a separate Stream (JSONLStream beside .git).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/repository"
)

const (
	cfgRepositoryID = "kc.repositoryId"
	cfgDriver       = "kc.driver"
)

var (
	_ repository.Repository    = (*FileGitRepository)(nil)
	_ repository.SnapshotStore = (*FileGitRepository)(nil)
	_ repository.Knowledge     = (*FileGitRepository)(nil)
)

func git(cwd string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	cmd.Stdin = nil
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func gitOK(cwd string, args ...string) bool {
	_, err := git(cwd, args...)
	return err == nil
}

func checkoutName(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}

func safeRelativePath(value string) (string, error) {
	if value == "" || filepath.IsAbs(value) {
		return "", kernel.Fail(kernel.ErrPreconditionFailed, "path must be relative: %s", value)
	}
	normalized := filepath.Clean(value)
	if normalized == ".." || strings.HasPrefix(normalized, ".."+string(os.PathSeparator)) {
		return "", kernel.Fail(kernel.ErrPreconditionFailed, "path escapes repository root: %s", value)
	}
	return normalized, nil
}

type FileGitRepository struct {
	repositoryID kernel.RepositoryID
	rootDir      string
}

func NewFileGit(rootDir string, repositoryID kernel.RepositoryID) (*FileGitRepository, error) {
	return OpenGitSnapshot(rootDir, repositoryID, "filegit")
}

// OpenGitSnapshot opens a git-shaped knowledge Snapshot (tree/commit/ref/CAS).
// driver is stamped in git config (filegit or dolt). APPEND is not opened here.
func OpenGitSnapshot(rootDir string, repositoryID kernel.RepositoryID, driver string) (*FileGitRepository, error) {
	if driver == "" {
		driver = "filegit"
	}
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(rootDir, ".git")); os.IsNotExist(err) {
		cmd := exec.Command("git", "init", "-q")
		cmd.Dir = rootDir
		if err := cmd.Run(); err != nil {
			return nil, err
		}
		_, _ = git(rootDir, "branch", "-M", "main")
		if _, err := git(rootDir, "-c", "user.name=knowledge-catalog", "-c", "user.email=dev@knowledge-catalog.local", "commit", "--allow-empty", "-q", "-m", "root"); err != nil {
			return nil, err
		}
	}
	excludePath := filepath.Join(rootDir, ".git", "info", "exclude")
	exclude, _ := os.ReadFile(excludePath)
	text := string(exclude)
	if !strings.Contains("\n"+text+"\n", "\nstreams/\n") {
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += "streams/\n"
		_ = os.MkdirAll(filepath.Dir(excludePath), 0o755)
		_ = os.WriteFile(excludePath, []byte(text), 0o644)
	}
	if err := stampFileGit(rootDir, repositoryID, driver); err != nil {
		return nil, err
	}
	return &FileGitRepository{repositoryID: repositoryID, rootDir: rootDir}, nil
}

func stampFileGit(rootDir string, repositoryID kernel.RepositoryID, driver string) error {
	if driver == "" {
		driver = "filegit"
	}
	existing, err := git(rootDir, "config", "--local", "--get", cfgRepositoryID)
	if err == nil && existing != "" && existing != string(repositoryID) {
		return fmt.Errorf("directory %s is stamped as %s, not %s", rootDir, existing, repositoryID)
	}
	if _, err := git(rootDir, "config", "--local", cfgRepositoryID, string(repositoryID)); err != nil {
		return err
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
	head, err := r.Head("refs/heads/main")
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
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "ref %s is unresolved", ref)
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

func (r *FileGitRepository) everExisted(objectID kernel.ObjectID) bool {
	prefix := "objects/" + string(objectID)
	raw, err := git(r.rootDir, "log", "--all", "--pretty=format:%H", "--", prefix, prefix+".json")
	return err == nil && raw != ""
}

func (r *FileGitRepository) HasCommit(commitID kernel.CommitID) bool {
	return commitID != "" && gitOK(r.rootDir, "cat-file", "-e", string(commitID)+"^{commit}")
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
		return kernel.Fail(kernel.ErrPreconditionFailed, "unknown commit %s", commitID)
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
		return "", kernel.Fail(kernel.ErrNonFastForward, "%s is not a descendant of %s", candidate, expected)
	}
	if !gitOK(r.rootDir, "update-ref", targetRef, string(candidate), string(expected)) {
		cur, _ := r.GetRef(targetRef)
		return "", kernel.Fail(kernel.ErrNonFastForward, "expected %s but ref is %s", expected, cur)
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
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s is unresolved", commitID)
	}
	idx := repofile.NewTree()
	raw, err := git(r.rootDir, "ls-tree", "-r", "--name-only", string(commitID))
	if err != nil {
		return nil, err
	}
	for _, rel := range splitNonEmpty(raw) {
		if !repofile.KnowledgePath(rel) {
			continue
		}
		content, err := git(r.rootDir, "show", string(commitID)+":"+rel)
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
