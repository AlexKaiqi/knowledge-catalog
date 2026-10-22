package projection

import (
	"context"

	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	objectmaintenance "kc/object/maintenance"
)

// Ensure brings the live projection to commit. Cancellation is honoured
// between object reads and write batches; a cancelled rebuild is aborted and
// leaves the previous READY meta in place.
func (idx *Index) Ensure(ctx context.Context, repo objectmodel.Repository, commit kernel.CommitID) (ports.IndexSync, error) {
	if err := ctx.Err(); err != nil {
		return ports.IndexSync{}, err
	}
	repo = objectmodel.BindContext(ctx, repo)
	eng, err := idx.engine(authorityEngineID(repo))
	if err != nil {
		return ports.IndexSync{}, err
	}
	meta, err := loadMetaContext(ctx, eng)
	if err != nil {
		return ports.IndexSync{}, err
	}
	// Already projected at this commit: answer from metadata rather than
	// recompiling a contract whose basis cannot have changed.
	if digest, ready := idx.readyDigestAt(eng, meta, commit); ready {
		return idx.readySync(repo.ID(), commit, digest, eng)
	}
	spec, err := idx.specAtCommit(repo, commit)
	if err != nil {
		return ports.IndexSync{}, err
	}
	if err := ctx.Err(); err != nil {
		return ports.IndexSync{}, err
	}
	if idx.projectionMatches(eng, meta, commit, spec.AccessDigest) {
		return idx.readySync(repo.ID(), commit, spec.AccessDigest, eng)
	}
	if meta.Basis == "" {
		return idx.rebuild(ctx, eng, repo, commit, spec, ports.IndexCauseCold)
	}
	if meta.AccessDigest != spec.AccessDigest || !idx.physicalMatches(eng, meta) {
		return idx.rebuild(ctx, eng, repo, commit, spec, ports.IndexCauseSchema)
	}
	if !repo.HasCommit(meta.Basis) {
		// The projection belongs to a prior authority generation that reused
		// the logical repository identity. This is a proven divergence, not a
		// failed change-capability call, so an explicit rebuild is safe.
		return idx.rebuild(ctx, eng, repo, commit, spec, ports.IndexCauseDiverged)
	}
	if changed, ok := repo.(objectmodel.FastObjectChanges); ok {
		objects, err := changed.FastChangedObjects(meta.Basis, commit)
		if err != nil {
			return ports.IndexSync{}, err
		}
		return idx.applyChangedObjects(ctx, eng, repo, meta.Basis, commit, spec, objects, ports.IndexCauseContent)
	}
	ids, err := objectmaintenance.ChangedObjectIDs(repo, meta.Basis, commit)
	if err != nil {
		return ports.IndexSync{}, err
	}
	sync, err := idx.apply(ctx, eng, repo, meta.Basis, commit, spec, ids, ports.IndexCauseContent)
	if err != nil {
		return ports.IndexSync{}, err
	}
	return sync, nil
}

// EnsureAt builds a projection at commit without moving the live engine.
//
// The ready check runs before any build gate is taken and reads only what the
// projection already retained, so an already projected basis answers without
// touching the declaration authority. Only a build that must actually happen
// serializes, and only against builds of the same basis.
func (idx *Index) EnsureAt(ctx context.Context, repo objectmodel.Repository, commit kernel.CommitID) (ports.IndexSync, error) {
	if err := ctx.Err(); err != nil {
		return ports.IndexSync{}, err
	}
	repo = objectmodel.BindContext(ctx, repo)
	if commit == "" {
		return ports.IndexSync{}, kernel.Fail(kernel.ErrUsageInvalid, "EnsureAt requires a commit")
	}
	eng, err := idx.engineAt(authorityEngineID(repo), commit)
	if err != nil {
		return ports.IndexSync{}, err
	}
	meta, err := loadMetaContext(ctx, eng)
	if err != nil {
		return ports.IndexSync{}, err
	}
	if digest, ready := idx.readyDigestAt(eng, meta, commit); ready {
		return idx.readySync(repo.ID(), commit, digest, eng)
	}
	release, err := idx.enterFixedBuild(ctx, engineKey{repo: authorityEngineID(repo), commit: commit})
	if err != nil {
		return ports.IndexSync{}, err
	}
	defer release()
	// The basis may have become READY while this caller waited for the gate, so
	// re-read instead of paying for a second full build.
	meta, err = loadMetaContext(ctx, eng)
	if err != nil {
		return ports.IndexSync{}, err
	}
	if digest, ready := idx.readyDigestAt(eng, meta, commit); ready {
		return idx.readySync(repo.ID(), commit, digest, eng)
	}
	spec, err := idx.specAtCommit(repo, commit)
	if err != nil {
		return ports.IndexSync{}, err
	}
	if idx.projectionMatches(eng, meta, commit, spec.AccessDigest) {
		return idx.readySync(repo.ID(), commit, spec.AccessDigest, eng)
	}
	cause := ports.IndexCauseCold
	if meta.Basis != "" {
		cause = ports.IndexCauseContent
	}
	return idx.rebuild(ctx, eng, repo, commit, spec, cause)
}

func (idx *Index) Apply(repo objectmodel.Repository, from, to kernel.CommitID, objectIDs []objectmodel.ObjectID) (ports.IndexSync, error) {
	spec, err := idx.specAtCommit(repo, to)
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
	cause, needRebuild := idx.classify(eng, meta, from, spec.AccessDigest)
	if needRebuild {
		return idx.rebuild(context.Background(), eng, repo, to, spec, cause)
	}
	if meta.Basis == to {
		return idx.readySync(repo.ID(), to, spec.AccessDigest, eng)
	}
	if from == "" {
		from = meta.Basis
	}
	return idx.apply(context.Background(), eng, repo, from, to, spec, objectIDs, ports.IndexCauseContent)
}

func (idx *Index) applyChangedObjects(ctx context.Context, eng ports.Engine, repo objectmodel.Repository, from, to kernel.CommitID, spec retrieval.AccessSpec, changes []objectmodel.ChangedObject, cause string) (ports.IndexSync, error) {
	ids := make([]objectmodel.ObjectID, 0, len(changes))
	fromPaths := map[objectmodel.ObjectID][]string{}
	toPaths := map[objectmodel.ObjectID][]string{}
	for _, change := range changes {
		ids = append(ids, change.ObjectID)
		fromPaths[change.ObjectID] = change.FromPaths
		toPaths[change.ObjectID] = change.ToPaths
	}
	return idx.applyHydrated(ctx, eng, repo, from, to, spec, ids, fromPaths, toPaths, cause)
}

func (idx *Index) classify(eng ports.Engine, meta ports.Meta, from kernel.CommitID, digest kernel.Digest) (cause string, rebuild bool) {
	if meta.Basis == "" {
		return ports.IndexCauseCold, true
	}
	if meta.AccessDigest != digest || !idx.physicalMatches(eng, meta) {
		return ports.IndexCauseSchema, true
	}
	if from != "" && meta.Basis != from {
		return ports.IndexCauseDiverged, true
	}
	return ports.IndexCauseContent, false
}

func (idx *Index) Rebuild(repo objectmodel.Repository, commit kernel.CommitID) (ports.IndexSync, error) {
	spec, err := idx.specAtCommit(repo, commit)
	if err != nil {
		return ports.IndexSync{}, err
	}
	eng, err := idx.engine(authorityEngineID(repo))
	if err != nil {
		return ports.IndexSync{}, err
	}
	cause := ports.IndexCauseCold
	if meta, err := eng.LoadMeta(); err == nil && meta.Basis != "" {
		if meta.AccessDigest != spec.AccessDigest || !idx.physicalMatches(eng, meta) {
			cause = ports.IndexCauseSchema
		} else {
			cause = ports.IndexCauseContent
		}
	}
	return idx.rebuild(context.Background(), eng, repo, commit, spec, cause)
}

func (idx *Index) readySync(id kernel.RepositoryID, commit kernel.CommitID, digest kernel.Digest, eng ports.Engine) (ports.IndexSync, error) {
	count, err := eng.Count()
	if err != nil {
		return ports.IndexSync{}, err
	}
	return ports.IndexSync{
		Mode: ports.IndexModeReady, Cause: ports.IndexCauseReady, Repository: id,
		BasisCommit: commit, AccessDigest: digest, PhysicalDigest: idx.projectionPhysicalDigest(eng), ObjectCount: count,
	}, nil
}

func (idx *Index) projectionMeta(eng ports.Engine, basis kernel.CommitID, spec retrieval.AccessSpec, mode, cause string) ports.Meta {
	meta := ports.Meta{Basis: basis, AccessDigest: spec.AccessDigest, SchemaAdoptions: spec.AdoptionWitnesses(), State: ports.ProjectionStateReady, Coverage: 1, Mode: mode, Cause: cause}
	if provider, ok := eng.(ports.ProviderIdentity); ok {
		meta.ProviderRevision = provider.ProviderRevision()
	}
	meta.PhysicalDigest = idx.projectionPhysicalDigest(eng)
	return meta
}

func (idx *Index) projectionPhysicalDigest(eng ports.Engine) kernel.Digest {
	var driver kernel.Digest
	if provider, ok := eng.(ports.ProviderIdentity); ok {
		driver = provider.PhysicalDigest()
	}
	if idx.manifestDigest == "" {
		return driver
	}
	return kernel.CanonicalDigest(struct {
		Driver   kernel.Digest `json:"driver"`
		Manifest kernel.Digest `json:"manifest"`
	}{driver, idx.manifestDigest})
}

func (idx *Index) physicalMatches(eng ports.Engine, meta ports.Meta) bool {
	if meta.State != "" && meta.State != ports.ProjectionStateReady {
		return false
	}
	if meta.PhysicalDigest != idx.projectionPhysicalDigest(eng) {
		return false
	}
	provider, ok := eng.(ports.ProviderIdentity)
	return !ok || meta.ProviderRevision == provider.ProviderRevision()
}

func (idx *Index) projectionMatches(eng ports.Engine, meta ports.Meta, basis kernel.CommitID, access kernel.Digest) bool {
	return meta.Basis == basis && meta.AccessDigest == access && idx.physicalMatches(eng, meta)
}

func (idx *Index) rebuild(ctx context.Context, eng ports.Engine, repo objectmodel.Repository, commit kernel.CommitID, spec retrieval.AccessSpec, cause string) (ports.IndexSync, error) {
	meta := idx.projectionMeta(eng, commit, spec, ports.IndexModeRebuild, cause)
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
			if err := ctx.Err(); err != nil {
				return err
			}
			doc, include, err := compileValueComputed(ctx, eng, repo, value, spec)
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
			err = ctx.Err()
		}
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
		if err := ctx.Err(); err != nil {
			return err
		}
		doc, ok, err := compileValueComputed(ctx, eng, repo, value, spec)
		if err != nil {
			return err
		}
		if ok {
			docs = append(docs, doc)
		}
		return nil
	})
	if err == nil {
		err = ctx.Err()
	}
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

func (idx *Index) apply(ctx context.Context, eng ports.Engine, repo objectmodel.Repository, from, to kernel.CommitID, spec retrieval.AccessSpec, objectIDs []objectmodel.ObjectID, cause string) (ports.IndexSync, error) {
	return idx.applyHydrated(ctx, eng, repo, from, to, spec, objectIDs, nil, nil, cause)
}

func (idx *Index) applyHydrated(ctx context.Context, eng ports.Engine, repo objectmodel.Repository, from, to kernel.CommitID, spec retrieval.AccessSpec, objectIDs []objectmodel.ObjectID, fromPaths, toPaths map[objectmodel.ObjectID][]string, cause string) (ports.IndexSync, error) {
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
		if err := ctx.Err(); err != nil {
			return ports.IndexSync{}, err
		}
		batch := ids[start:end]
		before, err := compileProjectionBatch(ctx, eng, repo, from, batch, spec, fromPaths)
		if err != nil {
			return ports.IndexSync{}, err
		}
		after, err := compileProjectionBatch(ctx, eng, repo, to, batch, spec, toPaths)
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
	if err := ctx.Err(); err != nil {
		return ports.IndexSync{}, err
	}
	meta := idx.projectionMeta(eng, to, spec, ports.IndexModeIncremental, cause)
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

func compileProjectionBatch(ctx context.Context, eng ports.Engine, repo objectmodel.Repository, commit kernel.CommitID, ids []objectmodel.ObjectID, spec retrieval.AccessSpec, paths map[objectmodel.ObjectID][]string) (map[objectmodel.ObjectID]ports.CompiledDoc, error) {
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
		doc, include, err := compileValueComputed(ctx, eng, repo, value, spec)
		if err != nil {
			return nil, err
		}
		if include {
			docs[id] = doc
		}
	}
	return docs, nil
}
