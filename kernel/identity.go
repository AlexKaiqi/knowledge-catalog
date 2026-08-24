package kernel

// RepositoryID is kr://<org>/<scope>/<name>. It names a Snapshot (layer ⓪).
// Catalog.Repositories are these ids. ObjectID is layer ② (in file content).
type RepositoryID string

// ObjectID is stable within one repository and independent of path.
type ObjectID string

// CommitID is an immutable snapshot version (git hash in FileGit).
type CommitID string

// Digest is a sha256 of a canonical value.
type Digest string

// KnowledgeRef is kc://<repo>/<object-id> — long-term, path-independent.
type KnowledgeRef struct {
	Repository RepositoryID `json:"repository"`
	Object     ObjectID     `json:"object"`
}

// PinnedKnowledgeRef adds a CommitID for reproducible reads.
type PinnedKnowledgeRef struct {
	KnowledgeRef
	Commit CommitID `json:"commit"`
}

// FileRef locates a raw file only.
type FileRef struct {
	Repository RepositoryID `json:"repository"`
	Commit     CommitID     `json:"commit"`
	Path       string       `json:"path"`
	Digest     Digest       `json:"digest,omitempty"`
}

func shortRepo(id RepositoryID) string {
	s := string(id)
	if len(s) >= 5 && s[:5] == "kr://" {
		return s[5:]
	}
	return s
}

func FormatKnowledgeRef(repository RepositoryID, object ObjectID) string {
	return "kc://" + shortRepo(repository) + "/" + string(object)
}

func FormatPinnedRef(repository RepositoryID, commit CommitID, object ObjectID) string {
	return "kc://" + shortRepo(repository) + "@" + string(commit) + "/" + string(object)
}
