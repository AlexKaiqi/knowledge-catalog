package index

import "kc/kernel"

// engineKey identifies either a live engine (empty commit) or a frozen pin engine.
type engineKey struct {
	repo   kernel.RepositoryID
	commit kernel.CommitID
	lane   string
}

func (idx *Index) engine(id kernel.RepositoryID) (Engine, error) {
	return idx.engineAt(id, "")
}

func pinOpenID(id kernel.RepositoryID, commit kernel.CommitID) kernel.RepositoryID {
	if commit == "" {
		return id
	}
	return kernel.RepositoryID(string(id) + "@" + string(commit))
}

func (idx *Index) engineAt(id kernel.RepositoryID, commit kernel.CommitID) (Engine, error) {
	return idx.engineLaneAt(id, commit, "")
}

func (idx *Index) engineLaneAt(id kernel.RepositoryID, commit kernel.CommitID, lane string) (Engine, error) {
	key := engineKey{repo: id, commit: commit, lane: lane}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if eng, ok := idx.engs[key]; ok {
		return eng, nil
	}
	openID := pinOpenID(id, commit)
	if lane != "" {
		openID = kernel.RepositoryID(string(openID) + "#" + lane)
	}
	eng, err := idx.open(idx.dir, openID)
	if err != nil {
		return nil, err
	}
	idx.engs[key] = eng
	return eng, nil
}

func (idx *Index) stateEngineAt(id kernel.RepositoryID, commit kernel.CommitID) (Engine, error) {
	return idx.engineLaneAt(id, commit, "state")
}

func (idx *Index) engineForCommit(id kernel.RepositoryID, commit kernel.CommitID) (Engine, error) {
	live, matches, err := idx.liveEngineForCommit(id, commit)
	if err != nil {
		return nil, err
	}
	if matches {
		return live, nil
	}
	return idx.engineAt(id, commit)
}

func (idx *Index) liveEngineForCommit(id kernel.RepositoryID, commit kernel.CommitID) (Engine, bool, error) {
	live, err := idx.engine(id)
	if err != nil {
		return nil, false, err
	}
	meta, err := live.LoadMeta()
	if err != nil {
		return nil, false, err
	}
	return live, meta.Basis == commit, nil
}

// acquireEngineForCommit avoids retaining an engine for every ad-hoc historic
// pin. Explicit EnsureAt projections remain cached; read-only misses are opened
// for one request and released when that request finishes.
func (idx *Index) acquireEngineForCommit(id kernel.RepositoryID, commit kernel.CommitID) (Engine, func(), error) {
	live, matches, err := idx.liveEngineForCommit(id, commit)
	if err != nil {
		return nil, nil, err
	}
	if matches {
		return live, func() {}, nil
	}
	key := engineKey{repo: id, commit: commit}
	idx.mu.Lock()
	if eng, ok := idx.engs[key]; ok {
		idx.mu.Unlock()
		return eng, func() {}, nil
	}
	idx.mu.Unlock()
	eng, err := idx.open(idx.dir, pinOpenID(id, commit))
	if err != nil {
		return nil, nil, err
	}
	return eng, func() { _ = eng.Close() }, nil
}
