package dolt

import (
	"encoding/base64"
	"sort"
	"strings"

	"kc/internal/gitdir"
	"kc/internal/treepath"
	"kc/kernel"
	"kc/snapshot"
)

func (r *DoltRepository) snapshotFiles(commit kernel.CommitID) (map[string][]byte, error) {
	if _, err := r.queryHash(string(commit)); err != nil {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	rows, err := r.query("SELECT TO_BASE64(path) AS path64, TO_BASE64(content) AS content64 FROM kc_files AS OF " + sqlString(string(commit)) + " ORDER BY path")
	if err != nil {
		// The init ancestor predates kc_files and is a valid empty Snapshot.
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
	clean, err := treepath.Clean(path)
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

func (r *DoltRepository) ApplyTreeCommit(cs snapshot.TreeChangeSet) (kernel.CommitID, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.applyTreeLocked(cs)
}

func (r *DoltRepository) applyTreeLocked(cs snapshot.TreeChangeSet) (kernel.CommitID, error) {
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
		clean, err := treepath.Clean(change.Path)
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
