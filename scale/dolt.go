package scale

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"kc/internal/gitdir"
	"kc/internal/repofile"
	"kc/kernel"
	"kc/repository"
)

// DoltRepository is a native Dolt Snapshot adapter. Literal repository paths
// are rows in the versioned kc_files table; Snapshot commit IDs and branches
// are Dolt commits and branches, and historical reads use AS OF. It never
// creates a .git directory or delegates authority to FileGit.
type DoltRepository struct {
	repositoryID kernel.RepositoryID
	rootDir      string
	lock         *sync.Mutex
	archived     bool
}

var (
	_             repository.Repository    = (*DoltRepository)(nil)
	_             repository.SnapshotStore = (*DoltRepository)(nil)
	_             repository.Knowledge     = (*DoltRepository)(nil)
	_             repository.RawFileStore  = (*DoltRepository)(nil)
	doltRootLocks sync.Map
)

const (
	doltStamp       = ".kc-dolt-repository"
	doltDockerImage = "dolthub/dolt:latest"
)

// OpenDolt opens or initializes a native Dolt database. KC_DOLT_BIN may name
// a dolt executable. When none is installed, Docker is used so the reference
// implementation can still exercise a real Dolt engine in a clean room.
func OpenDolt(rootDir string, id kernel.RepositoryID) (repository.Repository, error) {
	abs, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	value, _ := doltRootLocks.LoadOrStore(abs, &sync.Mutex{})
	r := &DoltRepository{repositoryID: id, rootDir: abs, lock: value.(*sync.Mutex)}
	r.lock.Lock()
	defer r.lock.Unlock()
	if err := r.ensure(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *DoltRepository) ID() kernel.RepositoryID { return r.repositoryID }

// ReadDoltStamp identifies a native Dolt repository during home discovery.
func ReadDoltStamp(rootDir string) (kernel.RepositoryID, error) {
	if _, err := os.Stat(filepath.Join(rootDir, ".dolt")); err != nil {
		return "", err
	}
	raw, err := os.ReadFile(filepath.Join(rootDir, doltStamp))
	if err != nil {
		return "", err
	}
	id := kernel.RepositoryID(strings.TrimSpace(string(raw)))
	if id == "" {
		return "", fmt.Errorf("empty Dolt repository stamp in %s", rootDir)
	}
	return id, nil
}

func (r *DoltRepository) ensure() error {
	stampPath := filepath.Join(r.rootDir, doltStamp)
	if raw, err := os.ReadFile(stampPath); err == nil {
		if strings.TrimSpace(string(raw)) != string(r.repositoryID) {
			return fmt.Errorf("Dolt database %s is stamped as %s, not %s", r.rootDir, strings.TrimSpace(string(raw)), r.repositoryID)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(filepath.Join(r.rootDir, ".dolt")); os.IsNotExist(err) {
		if _, err := r.run("init", "--name", "knowledge-catalog", "--email", "kc@local"); err != nil {
			return err
		}
		if _, err := r.run("sql", "-q", "CREATE TABLE kc_files (path VARCHAR(1024) PRIMARY KEY, content LONGBLOB NOT NULL)"); err != nil {
			return err
		}
		if _, err := r.run("add", "."); err != nil {
			return err
		}
		if _, err := r.run("commit", "-m", "root"); err != nil {
			return err
		}
	}
	if err := os.WriteFile(stampPath, []byte(string(r.repositoryID)+"\n"), 0o600); err != nil {
		return err
	}
	if _, err := r.queryHash("main"); err != nil {
		return err
	}
	rows, err := r.query("SELECT hash FROM dolt_branches WHERE name=" + sqlString("kc-archived"))
	if err != nil {
		return err
	}
	r.archived = len(rows) == 1
	return nil
}

func (r *DoltRepository) run(args ...string) (string, error) {
	bin := strings.TrimSpace(os.Getenv("KC_DOLT_BIN"))
	forceDocker := strings.TrimSpace(os.Getenv("KC_DOLT_FORCE_DOCKER")) == "1"
	var cmd *exec.Cmd
	if bin != "" {
		cmd = exec.Command(bin, args...)
		cmd.Dir = r.rootDir
	} else if found, err := exec.LookPath("dolt"); err == nil && !forceDocker {
		cmd = exec.Command(found, args...)
		cmd.Dir = r.rootDir
	} else {
		image := strings.TrimSpace(os.Getenv("KC_DOLT_DOCKER_IMAGE"))
		if image == "" {
			image = doltDockerImage
		}
		dockerArgs := []string{"run", "--rm", "-v", r.rootDir + ":/repo", "-w", "/repo", image}
		cmd = exec.Command("docker", append(dockerArgs, args...)...)
	}
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return "", fmt.Errorf("dolt %s: %s", strings.Join(args, " "), text)
	}
	return stripANSI(text), nil
}

func stripANSI(value string) string {
	for {
		start := strings.IndexByte(value, 0x1b)
		if start < 0 {
			return strings.TrimSpace(value)
		}
		end := strings.IndexByte(value[start:], 'm')
		if end < 0 {
			return strings.TrimSpace(value[:start])
		}
		value = value[:start] + value[start+end+1:]
	}
}

type doltRows struct {
	Rows []map[string]any `json:"rows"`
}

func (r *DoltRepository) query(query string) ([]map[string]any, error) {
	out, err := r.run("sql", "-r", "json", "-q", query)
	if err != nil {
		return nil, err
	}
	var result doltRows
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return nil, fmt.Errorf("decode Dolt JSON: %w (%s)", err, out)
	}
	return result.Rows, nil
}

func sqlString(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

func (r *DoltRepository) queryHash(ref string) (kernel.CommitID, error) {
	rows, err := r.query("SELECT DOLT_HASHOF(" + sqlString(ref) + ") AS hash")
	if err != nil || len(rows) != 1 {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "Dolt ref %s does not exist", ref)
	}
	hash, _ := rows[0]["hash"].(string)
	if hash == "" {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "Dolt ref %s does not exist", ref)
	}
	return kernel.CommitID(hash), nil
}

func doltBranch(ref string) (string, bool) {
	switch {
	case ref == "", ref == "HEAD", ref == repository.DefaultRef:
		return "main", true
	case ref == "refs/kc/archived":
		return "kc-archived", true
	case strings.HasPrefix(ref, "refs/heads/"):
		name := strings.TrimPrefix(ref, "refs/heads/")
		return name, name != "" && !strings.Contains(name, "..")
	default:
		return "", false
	}
}

func (r *DoltRepository) Head(ref string) (kernel.CommitID, error) {
	commit, ok := r.GetRef(ref)
	if !ok {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "ref %s does not exist", ref)
	}
	return commit, nil
}

func (r *DoltRepository) GetRef(ref string) (kernel.CommitID, bool) {
	branch, ok := doltBranch(ref)
	if !ok {
		return "", false
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	commit, err := r.queryHash(branch)
	return commit, err == nil
}

func (r *DoltRepository) HasCommit(commit kernel.CommitID) bool {
	if commit == "" {
		return false
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	_, err := r.queryHash(string(commit))
	return err == nil
}

func (r *DoltRepository) CreateRef(ref string, commit kernel.CommitID) error {
	branch, ok := doltBranch(ref)
	if !ok || branch == "main" {
		return kernel.Fail(kernel.ErrUsageInvalid, "unsupported Dolt ref %s", ref)
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	if ref != "refs/kc/archived" && r.archivedLocked() {
		return kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", r.repositoryID)
	}
	if _, err := r.queryHash(branch); err == nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "ref %s already exists", ref)
	}
	if _, err := r.queryHash(string(commit)); err != nil {
		return kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	_, err := r.run("branch", branch, string(commit))
	return err
}

func (r *DoltRepository) Merge(targetRef string, candidate, expected kernel.CommitID) (kernel.CommitID, error) {
	if r.Archived() {
		return "", kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", r.repositoryID)
	}
	branch, ok := doltBranch(targetRef)
	if !ok {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "unsupported Dolt ref %s", targetRef)
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	current, err := r.queryHash(branch)
	if err != nil || current != expected {
		return "", kernel.Fail(kernel.ErrNonFastForward, "ref %s moved: expected commit %s, actual %s", targetRef, expected, current)
	}
	base, err := r.run("merge-base", string(expected), string(candidate))
	if err != nil || strings.TrimSpace(base) != string(expected) {
		return "", kernel.Fail(kernel.ErrNonFastForward, "commit %s is not a descendant of %s", candidate, expected)
	}
	if _, err := r.run("checkout", branch); err != nil {
		return "", err
	}
	if _, err := r.run("reset", "--hard", string(candidate)); err != nil {
		return "", err
	}
	return candidate, nil
}

func (r *DoltRepository) Archived() bool {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.archivedLocked()
}

func (r *DoltRepository) Archive() error {
	if r.Archived() {
		return nil
	}
	head, err := r.Head(repository.DefaultRef)
	if err != nil {
		return err
	}
	if err := r.CreateRef("refs/kc/archived", head); err != nil {
		return err
	}
	r.lock.Lock()
	r.archived = true
	r.lock.Unlock()
	return nil
}

func (r *DoltRepository) snapshotFiles(commit kernel.CommitID) (map[string][]byte, error) {
	if _, err := r.queryHash(string(commit)); err != nil {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	rows, err := r.query("SELECT TO_BASE64(path) AS path64, TO_BASE64(content) AS content64 FROM kc_files AS OF " + sqlString(string(commit)) + " ORDER BY path")
	if err != nil {
		// dolt init creates an initial commit before kc_files is introduced.
		// That ancestor is a valid empty Repository snapshot, not an invalid
		// version; object LOG walks through it to find the introduction edge.
		if strings.Contains(err.Error(), "table not found: kc_files") {
			return map[string][]byte{}, nil
		}
		return nil, err
	}
	out := map[string][]byte{}
	for _, row := range rows {
		path64, _ := row["path64"].(string)
		content64, _ := row["content64"].(string)
		pathBytes, err := base64.StdEncoding.DecodeString(path64)
		if err != nil {
			return nil, err
		}
		content, err := base64.StdEncoding.DecodeString(content64)
		if err != nil {
			return nil, err
		}
		out[string(pathBytes)] = content
	}
	return out, nil
}

func (r *DoltRepository) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	clean, err := repofile.SafeRelativePath(path)
	if err != nil {
		return nil, err
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	files, err := r.snapshotFiles(commit)
	if err != nil {
		return nil, err
	}
	content, ok := files[clean]
	if !ok {
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "path %s is missing at commit %s", path, commit)
	}
	return append([]byte(nil), content...), nil
}

func (r *DoltRepository) ListFiles(commit kernel.CommitID) ([]string, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	files, err := r.snapshotFiles(commit)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func (r *DoltRepository) ApplyRawCommit(cs repository.RawFileChangeSet) (kernel.CommitID, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.applyRawLocked(cs)
}

func (r *DoltRepository) applyRawLocked(cs repository.RawFileChangeSet) (kernel.CommitID, error) {
	if r.archivedLocked() {
		return "", kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", r.repositoryID)
	}
	if cs.TargetRepository != r.repositoryID {
		return "", kernel.Fail(kernel.ErrTargetRepositoryDenied, "target %s does not match %s", cs.TargetRepository, r.repositoryID)
	}
	if cs.BaseCommit != cs.ExpectedTargetCommit {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "baseCommit must equal expectedTargetCommit")
	}
	if len(cs.Changes) == 0 {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "raw changeset has no changes")
	}
	branch, ok := doltBranch(cs.TargetRef)
	if !ok {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "unsupported Dolt ref %s", cs.TargetRef)
	}
	current, err := r.queryHash(branch)
	if err != nil || current != cs.ExpectedTargetCommit {
		return "", kernel.Fail(kernel.ErrNonFastForward, "ref %s moved: expected commit %s, actual %s", cs.TargetRef, cs.ExpectedTargetCommit, current)
	}
	if _, err := r.run("checkout", branch); err != nil {
		return "", err
	}
	statements := []string{"START TRANSACTION"}
	for _, change := range cs.Changes {
		clean, err := repofile.SafeRelativePath(change.Path)
		if err != nil {
			return "", err
		}
		pathExpr := "CONVERT(FROM_BASE64(" + sqlString(base64.StdEncoding.EncodeToString([]byte(clean))) + ") USING utf8mb4)"
		if change.Remove {
			statements = append(statements, "DELETE FROM kc_files WHERE path="+pathExpr)
		} else {
			content := sqlString(base64.StdEncoding.EncodeToString(change.Content))
			statements = append(statements, "REPLACE INTO kc_files(path,content) VALUES ("+pathExpr+",FROM_BASE64("+content+"))")
		}
	}
	statements = append(statements, "COMMIT")
	if _, err := r.run("sql", "-q", strings.Join(statements, "; ")); err != nil {
		_, _ = r.run("reset", "--hard", string(current))
		return "", err
	}
	if _, err := r.run("add", "kc_files"); err != nil {
		_, _ = r.run("reset", "--hard", string(current))
		return "", err
	}
	name, email, message := (gitdir.Signature{
		Author: cs.Author, Message: cs.Message, RequestID: cs.RequestID, RuleID: cs.RuleID,
	}).Format()
	if _, err := r.run("commit", "--allow-empty", "--author", name+" <"+email+">", "-m", message); err != nil {
		_, _ = r.run("reset", "--hard", string(current))
		return "", err
	}
	return r.queryHash(branch)
}

func (r *DoltRepository) archivedLocked() bool {
	if r.archived {
		return true
	}
	rows, err := r.query("SELECT hash FROM dolt_branches WHERE name=" + sqlString("kc-archived"))
	if err == nil && len(rows) == 1 {
		r.archived = true
	}
	return r.archived
}
