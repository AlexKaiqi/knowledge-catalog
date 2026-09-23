package index

import (
	"context"
	"kc/kernel"
	"kc/knowledge"
)

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

// authorityEngineID scopes one projection engine to the authority instance
// that backs the repository. Two deployments may serve the same logical
// repository id against one provider node; their projections (control state
// and generation indices) must never be shared, or one deployment reads the
// other's stale documents as its own fixed-basis truth.
func authorityEngineID(repo knowledge.Repository) kernel.RepositoryID {
	if identified, ok := repo.(knowledge.StoreIdentified); ok {
		if digest := identified.StoreDigest(); digest != "" {
			return kernel.RepositoryID(string(repo.ID()) + "@" + string(digest))
		}
	}
	return repo.ID()
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
	return idx.liveEngineForCommitContext(context.Background(), id, commit)
}

func (idx *Index) liveEngineForCommitContext(ctx context.Context, id kernel.RepositoryID, commit kernel.CommitID) (Engine, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	live, err := idx.engine(id)
	if err != nil {
		return nil, false, err
	}
	meta, err := loadMetaContext(ctx, live)
	if err != nil {
		return nil, false, err
	}
	return live, meta.Basis == commit, nil
}

// acquireEngineForCommit avoids retaining an engine for every ad-hoc historic
// pin. Explicit EnsureAt projections remain cached; read-only misses are opened
// for one request and released when that request finishes.
func (idx *Index) acquireEngineForCommit(id kernel.RepositoryID, commit kernel.CommitID) (Engine, func(), error) {
	return idx.acquireEngineForCommitContext(context.Background(), id, commit)
}

func (idx *Index) acquireEngineForCommitContext(ctx context.Context, id kernel.RepositoryID, commit kernel.CommitID) (Engine, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	key := engineKey{repo: id, commit: commit}
	idx.mu.Lock()
	if eng, ok := idx.engs[key]; ok {
		idx.mu.Unlock()
		return eng, func() {}, nil
	}
	idx.mu.Unlock()
	eng, fixedErr := idx.open(idx.dir, pinOpenID(id, commit))
	var meta Meta
	if fixedErr == nil {
		meta, fixedErr = loadMetaContext(ctx, eng)
	}
	// A retained serving basis takes precedence over the mutable HEAD lane,
	// including after reopening the process. Never race a published Dataset
	// against an unrelated HEAD update when its own fixed projection exists.
	if fixedErr == nil && meta.Basis == commit && meta.State == ProjectionStateReady {
		return eng, func() { _ = eng.Close() }, nil
	}
	live, matches, liveErr := idx.liveEngineForCommitContext(ctx, id, commit)
	if liveErr != nil || fixedErr != nil || matches {
		if eng != nil {
			_ = eng.Close()
		}
	}
	if liveErr != nil {
		return nil, nil, liveErr
	}
	if matches {
		return live, func() {}, nil
	}
	if fixedErr != nil {
		return nil, nil, fixedErr
	}
	return eng, func() { _ = eng.Close() }, nil
}
