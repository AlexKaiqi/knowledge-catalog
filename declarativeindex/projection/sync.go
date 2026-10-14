package projection

import (
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	objectmaintenance "kc/object/maintenance"
)

func (idx *Index) Ensure(repo objectmodel.Repository, commit kernel.CommitID) (ports.IndexSync, error) {
	spec, err := specAtCommit(repo, commit)
	if err != nil {
		return ports.IndexSync{}, err
	}
	eng, err := idx.engine(authorityEngineID(repo))
	if err != nil {
		return ports.IndexSync{}, err
	}
	meta, err := eng.LoadMeta()
	if err != nil {
		return ports.IndexSync{}, err
	}
	if projectionMatches(eng, meta, commit, spec.AccessDigest) {
		return readySync(repo.ID(), commit, spec.AccessDigest, eng)
	}
	if meta.Basis == "" {
		return idx.rebuild(eng, repo, commit, spec, ports.IndexCauseCold)
	}
	if meta.AccessDigest != spec.AccessDigest || !physicalMatches(eng, meta) {
		return idx.rebuild(eng, repo, commit, spec, ports.IndexCauseSchema)
	}
	if !repo.HasCommit(meta.Basis) {
		// The projection belongs to a prior authority generation that reused
		// the logical repository identity. This is a proven divergence, not a
		// failed change-capability call, so an explicit rebuild is safe.
		return idx.rebuild(eng, repo, commit, spec, ports.IndexCauseDiverged)
	}
	if changed, ok := repo.(objectmodel.FastObjectChanges); ok {
		objects, err := changed.FastChangedObjects(meta.Basis, commit)
		if err != nil {
			return ports.IndexSync{}, err
		}
		return idx.applyChangedObjects(eng, repo, meta.Basis, commit, spec, objects, ports.IndexCauseContent)
	}
	ids, err := objectmaintenance.ChangedObjectIDs(repo, meta.Basis, commit)
	if err != nil {
		return ports.IndexSync{}, err
	}
	sync, err := idx.apply(eng, repo, meta.Basis, commit, spec, ids, ports.IndexCauseContent)
	if err != nil {
		return ports.IndexSync{}, err
	}
	return sync, nil
}

// EnsureAt builds a projection at commit without moving the live engine.
func (idx *Index) EnsureAt(repo objectmodel.Repository, commit kernel.CommitID) (ports.IndexSync, error) {
	idx.fixedBuildMu.Lock()
	defer idx.fixedBuildMu.Unlock()
	if commit == "" {
		return ports.IndexSync{}, kernel.Fail(kernel.ErrUsageInvalid, "EnsureAt requires a commit")
	}
	spec, err := specAtCommit(repo, commit)
	if err != nil {
		return ports.IndexSync{}, err
	}
	eng, err := idx.engineAt(authorityEngineID(repo), commit)
	if err != nil {
		return ports.IndexSync{}, err
	}
	meta, err := eng.LoadMeta()
	if err != nil {
		return ports.IndexSync{}, err
	}
	if projectionMatches(eng, meta, commit, spec.AccessDigest) {
		return readySync(repo.ID(), commit, spec.AccessDigest, eng)
	}
	cause := ports.IndexCauseCold
	if meta.Basis != "" {
		cause = ports.IndexCauseContent
	}
	return idx.rebuild(eng, repo, commit, spec, cause)
}

func (idx *Index) Apply(repo objectmodel.Repository, from, to kernel.CommitID, objectIDs []objectmodel.ObjectID) (ports.IndexSync, error) {
	spec, err := specAtCommit(repo, to)
	if err != nil {
		return ports.IndexSync{}, err
	}
	eng, err := idx.engine(authorityEngineID(repo))
	if err != nil {
		return ports.IndexSync{}, err
	}
	meta, err := eng.LoadMeta()
	if err != nil {
		return ports.IndexSync{}, err
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
	return idx.apply(eng, repo, from, to, spec, objectIDs, ports.IndexCauseContent)
}

func (idx *Index) applyChangedObjects(eng ports.Engine, repo objectmodel.Repository, from, to kernel.CommitID, spec retrieval.AccessSpec, changes []objectmodel.ChangedObject, cause string) (ports.IndexSync, error) {
	ids := make([]objectmodel.ObjectID, 0, len(changes))
	fromPaths := map[objectmodel.ObjectID][]string{}
	toPaths := map[objectmodel.ObjectID][]string{}
	for _, change := range changes {
		ids = append(ids, change.ObjectID)
		fromPaths[change.ObjectID] = change.FromPaths
		toPaths[change.ObjectID] = change.ToPaths
	}
	return idx.applyHydrated(eng, repo, from, to, spec, ids, fromPaths, toPaths, cause)
}

func classify(eng ports.Engine, meta ports.Meta, from kernel.CommitID, digest kernel.Digest) (cause string, rebuild bool) {
	if meta.Basis == "" {
		return ports.IndexCauseCold, true
	}
	if meta.AccessDigest != digest || !physicalMatches(eng, meta) {
		return ports.IndexCauseSchema, true
	}
	if from != "" && meta.Basis != from {
		return ports.IndexCauseDiverged, true
	}
	return ports.IndexCauseContent, false
}

func (idx *Index) Rebuild(repo objectmodel.Repository, commit kernel.CommitID) (ports.IndexSync, error) {
	spec, err := specAtCommit(repo, commit)
	if err != nil {
		return ports.IndexSync{}, err
	}
	eng, err := idx.engine(authorityEngineID(repo))
	if err != nil {
		return ports.IndexSync{}, err
	}
	cause := ports.IndexCauseCold
	if meta, err := eng.LoadMeta(); err == nil && meta.Basis != "" {
		if meta.AccessDigest != spec.AccessDigest || !physicalMatches(eng, meta) {
			cause = ports.IndexCauseSchema
		} else {
			cause = ports.IndexCauseContent
		}
	}
	return idx.rebuild(eng, repo, commit, spec, cause)
}

func readySync(id kernel.RepositoryID, commit kernel.CommitID, digest kernel.Digest, eng ports.Engine) (ports.IndexSync, error) {
	count, err := eng.Count()
	if err != nil {
		return ports.IndexSync{}, err
	}
	return ports.IndexSync{
		Mode: ports.IndexModeReady, Cause: ports.IndexCauseReady, Repository: id,
		BasisCommit: commit, AccessDigest: digest, PhysicalDigest: projectionPhysicalDigest(eng), ObjectCount: count,
	}, nil
}

func projectionMeta(eng ports.Engine, basis kernel.CommitID, spec retrieval.AccessSpec, mode, cause string) ports.Meta {
	meta := ports.Meta{Basis: basis, AccessDigest: spec.AccessDigest, SchemaAdoptions: spec.AdoptionWitnesses(), State: ports.ProjectionStateReady, Coverage: 1, Mode: mode, Cause: cause}
	if provider, ok := eng.(ports.ProviderIdentity); ok {
		meta.ProviderRevision = provider.ProviderRevision()
		meta.PhysicalDigest = provider.PhysicalDigest()
	}
	return meta
}

func projectionPhysicalDigest(eng ports.Engine) kernel.Digest {
	if provider, ok := eng.(ports.ProviderIdentity); ok {
		return provider.PhysicalDigest()
	}
	return ""
}

func physicalMatches(eng ports.Engine, meta ports.Meta) bool {
	if meta.State != "" && meta.State != ports.ProjectionStateReady {
		return false
	}
	provider, ok := eng.(ports.ProviderIdentity)
	if !ok {
		return true
	}
	return meta.PhysicalDigest == provider.PhysicalDigest() && meta.ProviderRevision == provider.ProviderRevision()
}

func projectionMatches(eng ports.Engine, meta ports.Meta, basis kernel.CommitID, access kernel.Digest) bool {
	return meta.Basis == basis && meta.AccessDigest == access && physicalMatches(eng, meta)
}

func (idx *Index) rebuild(eng ports.Engine, repo objectmodel.Repository, commit kernel.CommitID, spec retrieval.AccessSpec, cause string) (ports.IndexSync, error) {
	meta := projectionMeta(eng, commit, spec, ports.IndexModeRebuild, cause)
	if streaming, ok := eng.(ports.StreamingProjectionMaintainer); ok {
		session, err := streaming.BeginRebuild(meta)
		if err != nil {
			return ports.IndexSync{}, err
		}
		const batchSize = 500
		batch := make([]ports.CompiledDoc, 0, batchSize)
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
		err = objectmaintenance.WalkRepository(repo, commit, func(value objectmodel.ObjectValue) error {
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
			return ports.IndexSync{}, err
		}
		if err := session.Commit(); err != nil {
			return ports.IndexSync{}, err
		}
		return ports.IndexSync{
			Mode: ports.IndexModeRebuild, Cause: cause, Repository: repo.ID(), BasisCommit: commit,
			AccessDigest: spec.AccessDigest, PhysicalDigest: meta.PhysicalDigest, ObjectCount: count, Updated: count,
		}, nil
	}
	var docs []ports.CompiledDoc
	err := objectmaintenance.WalkRepository(repo, commit, func(value objectmodel.ObjectValue) error {
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
		return ports.IndexSync{}, err
	}
	if err := eng.Rebuild(docs, meta); err != nil {
		return ports.IndexSync{}, err
	}
	return ports.IndexSync{
		Mode: ports.IndexModeRebuild, Cause: cause, Repository: repo.ID(), BasisCommit: commit,
		AccessDigest: spec.AccessDigest, PhysicalDigest: meta.PhysicalDigest, ObjectCount: len(docs), Updated: len(docs),
	}, nil
}

func (idx *Index) apply(eng ports.Engine, repo objectmodel.Repository, from, to kernel.CommitID, spec retrieval.AccessSpec, objectIDs []objectmodel.ObjectID, cause string) (ports.IndexSync, error) {
	return idx.applyHydrated(eng, repo, from, to, spec, objectIDs, nil, nil, cause)
}

func (idx *Index) applyHydrated(eng ports.Engine, repo objectmodel.Repository, from, to kernel.CommitID, spec retrieval.AccessSpec, objectIDs []objectmodel.ObjectID, fromPaths, toPaths map[objectmodel.ObjectID][]string, cause string) (ports.IndexSync, error) {
	var upserts []ports.CompiledDoc
	var deletes []objectmodel.ObjectID
	seen := map[objectmodel.ObjectID]struct{}{}
	ids := make([]objectmodel.ObjectID, 0, len(objectIDs))
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
			return ports.IndexSync{}, err
		}
		after, err := compileProjectionBatch(repo, to, batch, spec, toPaths)
		if err != nil {
			return ports.IndexSync{}, err
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
	meta := projectionMeta(eng, to, spec, ports.IndexModeIncremental, cause)
	if err := eng.Apply(upserts, deletes, meta); err != nil {
		return ports.IndexSync{}, err
	}
	count, err := eng.Count()
	if err != nil {
		return ports.IndexSync{}, err
	}
	return ports.IndexSync{
		Mode: ports.IndexModeIncremental, Cause: cause, Repository: repo.ID(), BasisCommit: to,
		AccessDigest: spec.AccessDigest, PhysicalDigest: meta.PhysicalDigest, ObjectCount: count, Updated: len(upserts), Removed: len(deletes),
	}, nil
}

func compileProjectionBatch(repo objectmodel.Repository, commit kernel.CommitID, ids []objectmodel.ObjectID, spec retrieval.AccessSpec, paths map[objectmodel.ObjectID][]string) (map[objectmodel.ObjectID]ports.CompiledDoc, error) {
	var values map[objectmodel.ObjectID]objectmodel.ObjectValue
	var err error
	if hydrator, ok := repo.(objectmodel.UnitPathsHydrator); ok && paths != nil {
		values, err = hydrator.ReadManyAtPaths(ids, commit, paths)
	} else {
		values, err = hydrateMany(repo, commit, ids)
	}
	if err != nil {
		return nil, err
	}
	docs := make(map[objectmodel.ObjectID]ports.CompiledDoc, len(values))
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
