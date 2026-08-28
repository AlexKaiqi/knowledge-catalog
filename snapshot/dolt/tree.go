package dolt

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
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
	if _, err := r.queryHash(string(commit)); err != nil {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	pathExpr := "CONVERT(FROM_BASE64(" + sqlString(base64.StdEncoding.EncodeToString([]byte(clean))) + ") USING utf8mb4)"
	rows, err := r.query("SELECT TO_BASE64(content) AS content64 FROM kc_files AS OF " + sqlString(string(commit)) + " WHERE path=" + pathExpr + " LIMIT 1")
	if err != nil {
		if strings.Contains(err.Error(), "table not found: kc_files") {
			return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "path %s is missing at commit %s", path, commit)
		}
		return nil, err
	}
	if len(rows) == 0 {
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "path %s is missing at commit %s", path, commit)
	}
	content64, _ := rows[0]["content64"].(string)
	content, err := base64.StdEncoding.DecodeString(content64)
	if err != nil {
		return nil, err
	}
	return content, nil
}

func (r *DoltRepository) ReadDirectory(request snapshot.DirectoryRequest) (snapshot.DirectoryPage, error) {
	directory := strings.Trim(strings.TrimSpace(request.Directory), "/")
	if directory != "" {
		clean, err := treepath.Clean(directory)
		if err != nil {
			return snapshot.DirectoryPage{}, err
		}
		directory = clean
	}
	limit := request.Limit
	if limit == 0 {
		limit = 256
	}
	if limit < 1 || limit > 1000 {
		return snapshot.DirectoryPage{}, kernel.Fail(kernel.ErrUsageInvalid, "directory limit must be between 1 and 1000")
	}
	cursor, err := snapshot.DecodeDirectoryCursor(request.Continuation, request.Commit, directory)
	if err != nil {
		return snapshot.DirectoryPage{}, err
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	if _, err := r.queryHash(string(request.Commit)); err != nil {
		return snapshot.DirectoryPage{}, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", request.Commit)
	}
	prefix := directory
	if prefix != "" {
		prefix += "/"
	}
	start := len([]byte(prefix)) + 1
	suffix := fmt.Sprintf("SUBSTRING(CONVERT(path USING utf8mb4),%d)", start)
	name := "SUBSTRING_INDEX(" + suffix + ", '/', 1)"
	where := "CONVERT(path USING utf8mb4) LIKE " + sqlString(escapeDoltLike(prefix)+"%") + " ESCAPE '\\\\'"
	having := name + " <> ''"
	if cursor.Position != "" {
		having += " AND " + name + " > " + sqlString(cursor.Position)
	}
	query := "SELECT TO_BASE64(" + name + ") AS name64, MAX(IF(INSTR(" + suffix + ", '/')>0,1,0)) AS is_dir FROM kc_files AS OF " + sqlString(string(request.Commit)) + " WHERE " + where + " GROUP BY " + name + " HAVING " + having + " ORDER BY " + name + " LIMIT " + strconv.Itoa(limit+1)
	rows, err := r.query(query)
	if err != nil {
		if strings.Contains(err.Error(), "table not found: kc_files") {
			return snapshot.DirectoryPage{Entries: []snapshot.DirectoryEntry{}, Exhausted: true, Generation: string(request.Commit)}, nil
		}
		return snapshot.DirectoryPage{}, err
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	entries := make([]snapshot.DirectoryEntry, 0, len(rows))
	last := ""
	for _, row := range rows {
		name64, _ := row["name64"].(string)
		raw, decodeErr := base64.StdEncoding.DecodeString(name64)
		if decodeErr != nil {
			return snapshot.DirectoryPage{}, decodeErr
		}
		last = string(raw)
		kind := "file"
		if doltNumber(row["is_dir"]) > 0 {
			kind = "directory"
		}
		entries = append(entries, snapshot.DirectoryEntry{Name: last, Kind: kind})
	}
	next := ""
	if more {
		next = snapshot.EncodeDirectoryCursor(snapshot.DirectoryCursor{Commit: request.Commit, Directory: directory, Position: last})
	}
	return snapshot.DirectoryPage{Entries: entries, Continuation: next, Exhausted: !more, Generation: string(request.Commit)}, nil
}

func escapeDoltLike(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "%", "\\%")
	return strings.ReplaceAll(value, "_", "\\_")
}
func doltNumber(value any) int64 {
	switch n := value.(type) {
	case float64:
		return int64(n)
	case int:
		return int64(n)
	case int64:
		return n
	case string:
		v, _ := strconv.ParseInt(n, 10, 64)
		return v
	}
	return 0
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
	if _, err := r.runSQLScript(strings.Join(statements, ";\n") + ";\n"); err != nil {
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
