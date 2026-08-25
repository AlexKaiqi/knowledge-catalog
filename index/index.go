package index

import (
	"regexp"
	"sync"

	"kc/kernel"
	"kc/knowledge"
)

// Index sits above one Repository (K-19): derived, discardable, never Canonical.
// Independent of Writer / Reader / Catalog. Subscribe via catalog.Hook (Sink).
// Live key is repository id (commit empty). Pin key is (repository, basisCommit).
type Index struct {
	dir  string
	open EngineOpener
	mu   sync.Mutex
	engs map[engineKey]Engine
}

func NewIndex(dir string) *Index {
	return NewIndexEngine(dir, nil)
}

// NewIndexEngine builds one working projection using the supplied physical engine.
func NewIndexEngine(dir string, opener EngineOpener) *Index {
	if opener == nil {
		opener = func(string, kernel.RepositoryID) (Engine, error) {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "index engine opener required (retrieval/sqlite or retrieval/elasticsearch)")
		}
	}
	return &Index{dir: dir, open: opener, engs: map[engineKey]Engine{}}
}

// AfterSnapshot applies a member snapshot to the working projection.
// Catalog never imports this package; CLI / tests wrap Catalog.Hook.
func (idx *Index) AfterSnapshot(repo knowledge.Repository, from, to kernel.CommitID, objectIDs []knowledge.ObjectID) error {
	if idx == nil || repo == nil {
		return nil
	}
	if objectIDs == nil {
		_, err := idx.Ensure(repo, to)
		return err
	}
	_, err := idx.Apply(repo, from, to, objectIDs)
	return err
}

func (idx *Index) Close() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	var first error
	for key, eng := range idx.engs {
		if err := eng.Close(); err != nil && first == nil {
			first = err
		}
		delete(idx.engs, key)
	}
	return first
}

var unsafeIndexChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func SanitizeID(id string) string {
	s := unsafeIndexChars.ReplaceAllString(id, "_")
	if s == "" {
		return "repo"
	}
	return s
}
