package dolt

import (
	"strings"

	"kc/internal/gitdir"
	"kc/kernel"
)

// NativeCommit is an adapter-specific incremental SQL commit. It deliberately
// contains no knowledge types; layer ② providers own table shape and encode
// their mutations into SQL before crossing this boundary.
type NativeCommit struct {
	TargetRef            string
	BaseCommit           kernel.CommitID
	ExpectedTargetCommit kernel.CommitID
	Statements           []string
	Tables               []string
	Message              string
	Author               string
	RequestID            string
	RuleID               string
	CommandID            string
}

// NativeQuery executes an immutable or metadata SQL query under the database
// lock. Callers must bind historical reads with AS OF themselves.
func (r *DoltRepository) NativeQuery(query string) ([]map[string]any, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.query(query)
}

// EnsureNativeSchema installs provider-owned versioned tables once. The
// resulting schema change is a normal Dolt commit on main.
func (r *DoltRepository) EnsureNativeSchema(required []string, createStatements []string) (kernel.CommitID, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	missing := false
	for _, table := range required {
		rows, err := r.query("SHOW TABLES LIKE " + sqlString(table))
		if err != nil {
			return "", err
		}
		if len(rows) == 0 {
			missing = true
			break
		}
	}
	if !missing {
		return r.queryHash("main")
	}
	if r.archivedLocked() {
		return "", kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", r.repositoryID)
	}
	current, err := r.queryHash("main")
	if err != nil {
		return "", err
	}
	if _, err := r.run("checkout", "main"); err != nil {
		return "", err
	}
	if _, err := r.run("sql", "-q", strings.Join(createStatements, "; ")); err != nil {
		_, _ = r.run("reset", "--hard", string(current))
		return "", err
	}
	if _, err := r.run("add", "."); err != nil {
		_, _ = r.run("reset", "--hard", string(current))
		return "", err
	}
	if _, err := r.run("commit", "--allow-empty", "-m", "install native knowledge schema"); err != nil {
		_, _ = r.run("reset", "--hard", string(current))
		return "", err
	}
	return r.queryHash("main")
}

// ApplyNativeCommit applies bounded row mutations and advances exactly one
// branch after a CAS check. The database-wide lock is the reference single
// active writer lease.
func (r *DoltRepository) ApplyNativeCommit(change NativeCommit) (kernel.CommitID, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	if r.archivedLocked() {
		return "", kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", r.repositoryID)
	}
	if change.BaseCommit != change.ExpectedTargetCommit {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "baseCommit must equal expectedTargetCommit")
	}
	if len(change.Statements) == 0 {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "native commit has no statements")
	}
	branch, ok := doltBranch(change.TargetRef)
	if !ok {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "unsupported Dolt ref %s", change.TargetRef)
	}
	current, err := r.queryHash(branch)
	if err != nil || current != change.ExpectedTargetCommit {
		return "", kernel.Fail(kernel.ErrNonFastForward, "ref %s moved: expected commit %s, actual %s", change.TargetRef, change.ExpectedTargetCommit, current)
	}
	if _, err := r.run("checkout", branch); err != nil {
		return "", err
	}
	statements := append([]string{"START TRANSACTION"}, change.Statements...)
	statements = append(statements, "COMMIT")
	if _, err := r.runSQLScript(strings.Join(statements, ";\n") + ";\n"); err != nil {
		_, _ = r.run("reset", "--hard", string(current))
		return "", err
	}
	if len(change.Tables) > 0 {
		if _, err := r.run(append([]string{"add"}, change.Tables...)...); err != nil {
			_, _ = r.run("reset", "--hard", string(current))
			return "", err
		}
	}
	message := change.Message
	if strings.TrimSpace(message) == "" {
		message = "knowledge commit"
	}
	if change.CommandID != "" {
		message += "\n\nKC-Command-Id: " + change.CommandID
	}
	name, email, formatted := (gitdir.Signature{
		Author: change.Author, Message: message, RequestID: change.RequestID, RuleID: change.RuleID,
	}).Format()
	if _, err := r.run("commit", "--allow-empty", "--author", name+" <"+email+">", "-m", formatted); err != nil {
		_, _ = r.run("reset", "--hard", string(current))
		return "", err
	}
	return r.queryHash(branch)
}
