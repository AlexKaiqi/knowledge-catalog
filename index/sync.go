package index

import (
	"kc/kernel"
	"kc/knowledge"
	knowledgemaintenance "kc/knowledge/maintenance"
	"kc/retrieval"
)

const (
	IndexModeIncremental = "incremental"
	IndexModeRebuild     = "rebuild"
	IndexModeReady       = "ready"

	IndexCauseContent  = "content"
	IndexCauseSchema   = "schema"
	IndexCauseReady    = "ready"
	IndexCauseCold     = "cold"
	IndexCauseDiverged = "diverged"
)

// IndexSync reports how a projection caught up to a commit.
type IndexSync struct {
	Mode           string              `json:"mode"`
	Cause          string              `json:"cause"`
	Repository     kernel.RepositoryID `json:"repository"`
	BasisCommit    kernel.CommitID     `json:"basisCommit"`
	AccessDigest   kernel.Digest       `json:"accessDigest"`
	PhysicalDigest kernel.Digest       `json:"physicalDigest"`
	ObjectCount    int                 `json:"objectCount"`
	Updated        int                 `json:"updated"`
	Removed        int                 `json:"removed"`
}

func (idx *Index) Ensure(repo knowledge.Repository, commit kernel.CommitID) (IndexSync, error) {
	spec, err := specAtCommit(repo, commit)
	if err != nil {
		return IndexSync{}, err
	}
	eng, err := idx.engine(authorityEngineID(repo))
	if err != nil {
		return IndexSync{}, err
	}
	meta, err := eng.LoadMeta()
	if err != nil {
		return IndexSync{}, err
	}
	if projectionMatches(eng, meta, commit, spec.AccessDigest) {
		return readySync(repo.ID(), commit, spec.AccessDigest, eng)
	}
	if meta.Basis == "" {
		return idx.rebuild(eng, repo, commit, spec, IndexCauseCold)
	}
	if meta.AccessDigest != spec.AccessDigest || !physicalMatches(eng, meta) {
		return idx.rebuild(eng, repo, commit, spec, IndexCauseSchema)
	}
	if !repo.HasCommit(meta.Basis) {
		// The projection belongs to a prior authority generation that reused
		// the logical repository identity. This is a proven divergence, not a
		// failed change-capability call, so an explicit rebuild is safe.
		return idx.rebuild(eng, repo, commit, spec, IndexCauseDiverged)
	}
	if changed, ok := repo.(knowledge.FastObjectChanges); ok {
		objects, err := changed.FastChangedObjects(meta.Basis, commit)
		if err != nil {
			return IndexSync{}, err
		}
		return idx.applyChangedObjects(eng, repo, meta.Basis, commit, spec, objects, IndexCauseContent)
	}
	ids, err := knowledgemaintenance.ChangedObjectIDs(repo, meta.Basis, commit)
	if err != nil {
		return IndexSync{}, err
	}
	sync, err := idx.apply(eng, repo, meta.Basis, commit, spec, ids, IndexCauseContent)
	if err != nil {
		return IndexSync{}, err
	}
	return sync, nil
}

// EnsureAt builds a projection at commit without moving the live engine.
func (idx *Index) EnsureAt(repo knowledge.Repository, commit kernel.CommitID) (IndexSync, error) {
	idx.fixedBuildMu.Lock()
	defer idx.fixedBuildMu.Unlock()
	if commit == "" {
		return IndexSync{}, kernel.Fail(kernel.ErrUsageInvalid, "EnsureAt requires a commit")
	}
	spec, err := specAtCommit(repo, commit)
	if err != nil {
		return IndexSync{}, err
	}
	eng, err := idx.engineAt(authorityEngineID(repo), commit)
	if err != nil {
		return IndexSync{}, err
	}
	meta, err := eng.LoadMeta()
	if err != nil {
		return IndexSync{}, err
	}
	if projectionMatches(eng, meta, commit, spec.AccessDigest) {
		return readySync(repo.ID(), commit, spec.AccessDigest, eng)
	}
	cause := IndexCauseCold
	if meta.Basis != "" {
		cause = IndexCauseContent
	}
	return idx.rebuild(eng, repo, commit, spec, cause)
}

func (idx *Index) Apply(repo knowledge.Repository, from, to kernel.CommitID, objectIDs []knowledge.ObjectID) (IndexSync, error) {
	spec, err := specAtCommit(repo, to)
	if err != nil {
		return IndexSync{}, err
	}
	eng, err := idx.engine(authorityEngineID(repo))
	if err != nil {
		return IndexSync{}, err
	}
	meta, err := eng.LoadMeta()
	if err != nil {
		return IndexSync{}, err
	}
	cause, needRebuild := classify(eng, meta, from, spec.AccessDigest)
	if needRebuild {
		return idx.rebuild(eng, repo, to, spec, cause)
	}
	if meta.Basis == to {
		return readySync(repo.ID(), to, spec.AccessDigest, eng)
	}
	if from == "" {
		from = meta.Basis
	}
	return idx.apply(eng, repo, from, to, spec, objectIDs, IndexCauseContent)
}

func (idx *Index) applyChangedObjects(eng Engine, repo knowledge.Repository, from, to kernel.CommitID, spec retrieval.AccessSpec, changes []knowledge.ChangedObject, cause string) (IndexSync, error) {
	ids := make([]knowledge.ObjectID, 0, len(changes))
	fromPaths := map[knowledge.ObjectID][]string{}
	toPaths := map[knowledge.ObjectID][]string{}
	for _, change := range changes {
		ids = append(ids, change.ObjectID)
		fromPaths[change.ObjectID] = change.FromPaths
		toPaths[change.ObjectID] = change.ToPaths
	}
	return idx.applyHydrated(eng, repo, from, to, spec, ids, fromPaths, toPaths, cause)
}

func classify(eng Engine, meta Meta, from kernel.CommitID, digest kernel.Digest) (cause string, rebuild bool) {
	if meta.Basis == "" {
		return IndexCauseCold, true
	}
	if meta.AccessDigest != digest || !physicalMatches(eng, meta) {
		return IndexCauseSchema, true
	}
	if from != "" && meta.Basis != from {
		return IndexCauseDiverged, true
	}
	return IndexCauseContent, false
}

func (idx *Index) Rebuild(repo knowledge.Repository, commit kernel.CommitID) (IndexSync, error) {
	spec, err := specAtCommit(repo, commit)
	if err != nil {
		return IndexSync{}, err
	}
	eng, err := idx.engine(authorityEngineID(repo))
	if err != nil {
		return IndexSync{}, err
	}
	cause := IndexCauseCold
	if meta, err := eng.LoadMeta(); err == nil && meta.Basis != "" {
		if meta.AccessDigest != spec.AccessDigest || !physicalMatches(eng, meta) {
			cause = IndexCauseSchema
		} else {
			cause = IndexCauseContent
		}
	}
	return idx.rebuild(eng, repo, commit, spec, cause)
}

func readySync(id kernel.RepositoryID, commit kernel.CommitID, digest kernel.Digest, eng Engine) (IndexSync, error) {
	count, err := eng.Count()
	if err != nil {
		return IndexSync{}, err
	}
	return IndexSync{
		Mode: IndexModeReady, Cause: IndexCauseReady, Repository: id,
		BasisCommit: commit, AccessDigest: digest, PhysicalDigest: projectionPhysicalDigest(eng), ObjectCount: count,
	}, nil
}

func projectionMeta(eng Engine, basis kernel.CommitID, access kernel.Digest, mode, cause string) Meta {
	meta := Meta{Basis: basis, AccessDigest: access, State: ProjectionStateReady, Coverage: 1, Mode: mode, Cause: cause}
	if provider, ok := eng.(ProviderIdentity); ok {
		meta.ProviderRevision = provider.ProviderRevision()
		meta.PhysicalDigest = provider.PhysicalDigest()
	}
	return meta
}

func projectionPhysicalDigest(eng Engine) kernel.Digest {
	if provider, ok := eng.(ProviderIdentity); ok {
		return provider.PhysicalDigest()
	}
	return ""
}

func physicalMatches(eng Engine, meta Meta) bool {
	if meta.State != "" && meta.State != ProjectionStateReady {
		return false
	}
	provider, ok := eng.(ProviderIdentity)
	if !ok {
		return true
	}
	return meta.PhysicalDigest == provider.PhysicalDigest() && meta.ProviderRevision == provider.ProviderRevision()
}

func projectionMatches(eng Engine, meta Meta, basis kernel.CommitID, access kernel.Digest) bool {
	return meta.Basis == basis && meta.AccessDigest == access && physicalMatches(eng, meta)
}

func (idx *Index) rebuild(eng Engine, repo knowledge.Repository, commit kernel.CommitID, spec retrieval.AccessSpec, cause string) (IndexSync, error) {
	meta := projectionMeta(eng, commit, spec.AccessDigest, IndexModeRebuild, cause)
	if streaming, ok := eng.(StreamingProjectionMaintainer); ok {
		session, err := streaming.BeginRebuild(meta)
		if err != nil {
			return IndexSync{}, err
		}
		const batchSize = 500
		batch := make([]CompiledDoc, 0, batchSize)
		count := 0
		flush := func() error {
			if len(batch) == 0 {
				return nil
			}
			if err := session.Append(batch); err != nil {
				return err
			}
			count += len(batch)
			batch = batch[:0]
			return nil
		}
		err = knowledgemaintenance.WalkRepository(repo, commit, func(value knowledge.KnowledgeValue) error {
			doc, include, err := compileValue(repo, value, spec)
			if err != nil {
				return err
			}
			if include {
				batch = append(batch, doc)
			}
			if len(batch) == batchSize {
				return flush()
			}
			return nil
		})
		if err == nil {
			err = flush()
		}
		if err != nil {
			_ = session.Abort(err)
			return IndexSync{}, err
		}
		if err := session.Commit(); err != nil {
			return IndexSync{}, err
		}
		return IndexSync{
			Mode: IndexModeRebuild, Cause: cause, Repository: repo.ID(), BasisCommit: commit,
			AccessDigest: spec.AccessDigest, PhysicalDigest: meta.PhysicalDigest, ObjectCount: count, Updated: count,
		}, nil
	}
	var docs []CompiledDoc
	err := knowledgemaintenance.WalkRepository(repo, commit, func(value knowledge.KnowledgeValue) error {
		doc, ok, err := compileValue(repo, value, spec)
		if err != nil {
			return err
		}
		if ok {
			docs = append(docs, doc)
		}
		return nil
	})
	if err != nil {
		return IndexSync{}, err
	}
	if err := eng.Rebuild(docs, meta); err != nil {
		return IndexSync{}, err
	}
	return IndexSync{
		Mode: IndexModeRebuild, Cause: cause, Repository: repo.ID(), BasisCommit: commit,
		AccessDigest: spec.AccessDigest, PhysicalDigest: meta.PhysicalDigest, ObjectCount: len(docs), Updated: len(docs),
	}, nil
}

func (idx *Index) apply(eng Engine, repo knowledge.Repository, from, to kernel.CommitID, spec retrieval.AccessSpec, objectIDs []knowledge.ObjectID, cause string) (IndexSync, error) {
	return idx.applyHydrated(eng, repo, from, to, spec, objectIDs, nil, nil, cause)
}

func (idx *Index) applyHydrated(eng Engine, repo knowledge.Repository, from, to kernel.CommitID, spec retrieval.AccessSpec, objectIDs []knowledge.ObjectID, fromPaths, toPaths map[knowledge.ObjectID][]string, cause string) (IndexSync, error) {
	var upserts []CompiledDoc
	var deletes []knowledge.ObjectID
	seen := map[knowledge.ObjectID]struct{}{}
	ids := make([]knowledge.ObjectID, 0, len(objectIDs))
	for _, id := range objectIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	// Recompile only the changed identities at the two proven bases. This
	// needs neither an engine-specific digest port nor a repository scan.
	const batchSize = 500
	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]
		before, err := compileProjectionBatch(repo, from, batch, spec, fromPaths)
		if err != nil {
			return IndexSync{}, err
		}
		after, err := compileProjectionBatch(repo, to, batch, spec, toPaths)
		if err != nil {
			return IndexSync{}, err
		}
		for _, id := range batch {
			oldDoc, wasIndexed := before[id]
			newDoc, isIndexed := after[id]
			switch {
			case isIndexed && (!wasIndexed || oldDoc.ObjectDigest != newDoc.ObjectDigest):
				upserts = append(upserts, newDoc)
			case wasIndexed && !isIndexed:
				deletes = append(deletes, id)
			}
		}
	}
	meta := projectionMeta(eng, to, spec.AccessDigest, IndexModeIncremental, cause)
	if err := eng.Apply(upserts, deletes, meta); err != nil {
		return IndexSync{}, err
	}
	count, err := eng.Count()
	if err != nil {
		return IndexSync{}, err
	}
	return IndexSync{
		Mode: IndexModeIncremental, Cause: cause, Repository: repo.ID(), BasisCommit: to,
		AccessDigest: spec.AccessDigest, PhysicalDigest: meta.PhysicalDigest, ObjectCount: count, Updated: len(upserts), Removed: len(deletes),
	}, nil
}

func compileProjectionBatch(repo knowledge.Repository, commit kernel.CommitID, ids []knowledge.ObjectID, spec retrieval.AccessSpec, paths map[knowledge.ObjectID][]string) (map[knowledge.ObjectID]CompiledDoc, error) {
	var values map[knowledge.ObjectID]knowledge.KnowledgeValue
	var err error
	if hydrator, ok := repo.(knowledge.UnitPathsHydrator); ok && paths != nil {
		values, err = hydrator.ReadManyAtPaths(ids, commit, paths)
	} else {
		values, err = hydrateMany(repo, commit, ids)
	}
	if err != nil {
		return nil, err
	}
	docs := make(map[knowledge.ObjectID]CompiledDoc, len(values))
	for _, id := range ids {
		value, exists := values[id]
		if !exists {
			continue
		}
		if value.Repository != repo.ID() || value.Commit != commit || value.Address.ObjectID != id {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "projection input %s does not match its repository and fixed basis", id)
		}
		doc, include, err := compileValue(repo, value, spec)
		if err != nil {
			return nil, err
		}
		if include {
			docs[id] = doc
		}
	}
	return docs, nil
}
