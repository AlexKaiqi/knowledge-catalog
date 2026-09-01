package reader

import (
	"sort"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// Lookup creates one shared layer ② interpreter over a Snapshot lookup seam.
// It is used when the caller owns membership (for example Catalog) but does
// not expose its Registry.
func Lookup(base func(kernel.RepositoryID) (snapshot.Store, error)) MemberLookup {
	service := NewReader(nil)
	return service.Lookup(base)
}

// Require resolves a mounted Snapshot and exposes its layer ② capability
// through the shared Knowledge service. Catalog remains Snapshot-only.
func (r *Reader) Require(repositoryID kernel.RepositoryID, code kernel.ErrorCode) (knowledge.Repository, error) {
	store, err := r.store.Require(repositoryID, code)
	if err != nil {
		return nil, err
	}
	return r.Wrap(store, code)
}

// Wrap converts one Catalog/Snapshot member into the process-wide Knowledge
// read service wrapper. Interpretation remains here;
// the underlying adapter exposes only immutable tree bytes.
func (r *Reader) Wrap(store snapshot.Store, code kernel.ErrorCode) (knowledge.Repository, error) {
	if native, ok := store.(knowledge.NativeRepository); ok {
		r.mu.Lock()
		r.repos[store.ID()] = native
		r.mu.Unlock()
		return native, nil
	}
	tree, ok := snapshot.TreeStoreOf(store)
	if !ok {
		return nil, kernel.Fail(code, "repository %s has no immutable tree access for knowledge interpretation", store.ID())
	}
	locator, ok := store.(knowledge.UnitLocator)
	if !ok {
		locator = &treeManifestLocator{tree: tree}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.repos[store.ID()]; ok {
		return existing, nil
	}
	wrapped := &treeRepository{base: store, tree: tree, locator: locator}
	r.repos[store.ID()] = wrapped
	return wrapped, nil
}

// Lookup preserves Catalog membership checks while moving knowledge
// interpretation to this service boundary.
func (r *Reader) Lookup(base func(kernel.RepositoryID) (snapshot.Store, error)) MemberLookup {
	return func(id kernel.RepositoryID) (knowledge.Repository, error) {
		store, err := base(id)
		if err != nil {
			return nil, err
		}
		return r.Wrap(store, kernel.ErrCapabilityUnsatisfied)
	}
}

type treeRepository struct {
	base    snapshot.Store
	tree    snapshot.TreeStore
	locator knowledge.UnitLocator
}

var (
	_ knowledge.Repository          = (*treeRepository)(nil)
	_ knowledge.BatchReadStore      = (*treeRepository)(nil)
	_ knowledge.FastChanges         = (*treeRepository)(nil)
	_ knowledge.SnapshotObjectPager = (*treeRepository)(nil)
)

func (r *treeRepository) ID() kernel.RepositoryID                   { return r.base.ID() }
func (r *treeRepository) Head(ref string) (kernel.CommitID, error)  { return r.base.Head(ref) }
func (r *treeRepository) GetRef(ref string) (kernel.CommitID, bool) { return r.base.GetRef(ref) }
func (r *treeRepository) HasCommit(commit kernel.CommitID) bool     { return r.base.HasCommit(commit) }
func (r *treeRepository) CreateRef(ref string, commit kernel.CommitID) error {
	return r.base.CreateRef(ref, commit)
}
func (r *treeRepository) Merge(ref string, candidate, expected kernel.CommitID) (kernel.CommitID, error) {
	return r.base.Merge(ref, candidate, expected)
}
func (r *treeRepository) Archived() bool { return r.base.Archived() }
func (r *treeRepository) Archive() error { return r.base.Archive() }

func (r *treeRepository) SchemaObjectIDs(commit kernel.CommitID) ([]knowledge.ObjectID, error) {
	locator, ok := r.locator.(knowledge.SchemaStore)
	if !ok {
		locator, ok = r.base.(knowledge.SchemaStore)
	}
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s does not provide schema namespace location", r.ID())
	}
	return locator.SchemaObjectIDs(commit)
}

func (r *treeRepository) BindingSchemaObjectIDs(commit kernel.CommitID) ([]knowledge.ObjectID, error) {
	locator, ok := r.locator.(knowledge.BindingLocator)
	if !ok {
		locator, ok = r.base.(knowledge.BindingLocator)
	}
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s does not provide Binding schema location", r.ID())
	}
	return locator.BindingSchemaObjectIDs(commit)
}

func (r *treeRepository) SchemaReferrerAddresses(schema knowledge.ObjectID, commit kernel.CommitID) ([]knowledge.Address, error) {
	locator, ok := r.locator.(knowledge.SchemaReferrerLocator)
	if !ok {
		locator, ok = r.base.(knowledge.SchemaReferrerLocator)
	}
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s does not provide schema referrer location", r.ID())
	}
	return locator.SchemaReferrerAddresses(schema, commit)
}

// treeManifestLocator gives Gitea and other tree authorities bounded layer ②
// reads without teaching layer ⓪ about object_id. The Writer versions this
// manifest in the same commit as the units; it is not a relation/search index.
type treeManifestLocator struct {
	tree snapshot.TreeStore
}

func (l *treeManifestLocator) load(commit kernel.CommitID) (repofile.LocatorManifest, error) {
	raw, err := l.tree.ReadFile(repofile.LocatorManifestPath, commit)
	if err != nil {
		if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
			return repofile.LocatorManifest{Objects: map[knowledge.ObjectID][]string{}}, nil
		}
		return repofile.LocatorManifest{}, err
	}
	manifest, err := repofile.DecodeLocatorManifest(raw)
	if err != nil {
		return repofile.LocatorManifest{}, kernel.Fail(kernel.ErrPreconditionFailed,
			"invalid exact knowledge unit manifest at %s: %v", commit, err)
	}
	return manifest, nil
}

func (l *treeManifestLocator) ObjectUnitPaths(objectID knowledge.ObjectID, commit kernel.CommitID) ([]string, error) {
	manifest, err := l.load(commit)
	if err != nil {
		return nil, err
	}
	return append([]string(nil), manifest.Objects[objectID]...), nil
}

func (l *treeManifestLocator) SchemaObjectIDs(commit kernel.CommitID) ([]knowledge.ObjectID, error) {
	manifest, err := l.load(commit)
	if err != nil {
		return nil, err
	}
	return append([]knowledge.ObjectID(nil), manifest.Schemas...), nil
}

func (l *treeManifestLocator) BindingSchemaObjectIDs(commit kernel.CommitID) ([]knowledge.ObjectID, error) {
	manifest, err := l.load(commit)
	if err != nil {
		return nil, err
	}
	return append([]knowledge.ObjectID(nil), manifest.BindingSchemas...), nil
}

func (l *treeManifestLocator) SchemaReferrerAddresses(schema knowledge.ObjectID, commit kernel.CommitID) ([]knowledge.Address, error) {
	manifest, err := l.load(commit)
	if err != nil {
		return nil, err
	}
	return append([]knowledge.Address(nil), manifest.Referrers[schema]...), nil
}

func readObjectUnits(store snapshot.TreeStore, locator knowledge.UnitLocator, objectID knowledge.ObjectID, commit kernel.CommitID) ([]repofile.Unit, error) {
	paths, err := locator.ObjectUnitPaths(objectID, commit)
	if err != nil {
		return nil, err
	}
	tree := repofile.NewTree()
	for _, unitPath := range paths {
		if !repofile.KnowledgePath(unitPath) {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "unit locator returned non-knowledge path %s", unitPath)
		}
		content, err := store.ReadFile(unitPath, commit)
		if err != nil {
			return nil, err
		}
		unit := repofile.Parse(string(content))
		if unit == nil || unit.ObjectID != objectID {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "unit locator returned a mismatched unit for %s", objectID)
		}
		if err := repofile.Ingest(tree, unit, unitPath); err != nil {
			return nil, err
		}
	}
	return tree.ObjectUnits(objectID), nil
}

func assembleKnowledgeValue(repository kernel.RepositoryID, objectID knowledge.ObjectID, commit kernel.CommitID, units []repofile.Unit) (knowledge.KnowledgeValue, error) {
	assembled, err := repofile.Assemble(units)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	value := knowledge.KnowledgeValue{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: repository, Object: objectID},
		Repository:   repository, Commit: commit,
		Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: objectID},
		Value:   assembled, Declarations: repofile.Declarations(units),
	}
	if len(units) == 1 {
		value.Provenance = units[0].Provenance
	}
	for _, unit := range units {
		if unit.Address.AspectName == "" {
			continue
		}
		for _, member := range units {
			value.Units = append(value.Units, member.Address)
		}
		break
	}
	return value, nil
}

func (r *treeRepository) ReadMany(objectIDs []knowledge.ObjectID, commit kernel.CommitID) (map[knowledge.ObjectID]knowledge.KnowledgeValue, error) {
	out := map[knowledge.ObjectID]knowledge.KnowledgeValue{}
	seen := map[knowledge.ObjectID]struct{}{}
	for _, objectID := range objectIDs {
		if objectID == "" {
			continue
		}
		if _, duplicate := seen[objectID]; duplicate {
			continue
		}
		seen[objectID] = struct{}{}
		units, err := readObjectUnits(r.tree, r.locator, objectID, commit)
		if err != nil {
			return nil, err
		}
		if len(units) == 0 {
			continue
		}
		value, err := assembleKnowledgeValue(r.ID(), objectID, commit, units)
		if err != nil {
			return nil, err
		}
		out[objectID] = value
	}
	return out, nil
}

func (r *treeRepository) Read(objectID knowledge.ObjectID, commit kernel.CommitID) (knowledge.KnowledgeValue, error) {
	values, err := r.ReadMany([]knowledge.ObjectID{objectID}, commit)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	if value, ok := values[objectID]; ok {
		return value, nil
	}
	return knowledge.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "object %s is missing at commit %s", objectID, commit)
}

func (r *treeRepository) ObjectIDsPage(commit kernel.CommitID, limit int, continuation string) (knowledge.ObjectIDPage, error) {
	if limit <= 0 {
		return knowledge.ObjectIDPage{}, kernel.Fail(kernel.ErrUsageInvalid, "object identity page limit must be positive")
	}
	manifest, err := (&treeManifestLocator{tree: r.tree}).load(commit)
	if err != nil {
		return knowledge.ObjectIDPage{}, err
	}
	ids := make([]knowledge.ObjectID, 0, len(manifest.Objects))
	for objectID := range manifest.Objects {
		ids = append(ids, objectID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	start := sort.Search(len(ids), func(i int) bool { return string(ids[i]) > continuation })
	end := start + limit
	if end > len(ids) {
		end = len(ids)
	}
	pageIDs := ids[start:end]
	page := knowledge.ObjectIDPage{ObjectIDs: pageIDs, Exhausted: end == len(ids)}
	if !page.Exhausted && len(pageIDs) > 0 {
		page.Continuation = string(pageIDs[len(pageIDs)-1])
	}
	return page, nil
}

func (r *treeRepository) Resolve(objectID knowledge.ObjectID, commit kernel.CommitID) (knowledge.Resolution, error) {
	units, err := readObjectUnits(r.tree, r.locator, objectID, commit)
	if err != nil {
		return knowledge.Resolution{}, err
	}
	if len(units) == 0 {
		status, err := r.missingStatus(objectID, commit)
		if err != nil {
			return knowledge.Resolution{}, err
		}
		return knowledge.Resolution{Repository: r.ID(), Commit: commit, ObjectID: objectID,
			Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: objectID}, Status: status}, nil
	}
	value, err := assembleKnowledgeValue(r.ID(), objectID, commit, units)
	if err != nil {
		return knowledge.Resolution{}, err
	}
	resolution := knowledge.Resolution{
		Repository: r.ID(), Commit: commit, ObjectID: objectID,
		Address: value.Address, Digest: kernel.CanonicalDigest(value.Value),
		PathHint:          repofile.EntityPathHint(units, objectID),
		DeclarationDigest: repofile.TreeDeclarationDigest(units),
		Status:            knowledge.StatusResolved,
	}
	if len(units) == 1 {
		resolution.SchemaRef = units[0].SchemaRef
		resolution.ValueSource = units[0].ValueSource
	}
	return resolution, nil
}

func (r *treeRepository) ResolveAddress(address knowledge.Address, commit kernel.CommitID) (knowledge.Resolution, error) {
	if err := knowledge.AssertWritable(address); err != nil {
		return knowledge.Resolution{}, err
	}
	units, err := readObjectUnits(r.tree, r.locator, address.ObjectID, commit)
	if err != nil {
		return knowledge.Resolution{}, err
	}
	var unit repofile.Unit
	ok := false
	for _, candidate := range units {
		if knowledge.AddressKey(candidate.Address) == knowledge.AddressKey(address) {
			unit, ok = candidate, true
			break
		}
	}
	if !ok {
		status, err := r.missingStatus(address.ObjectID, commit)
		if err != nil {
			return knowledge.Resolution{}, err
		}
		return knowledge.Resolution{Repository: r.ID(), Commit: commit, ObjectID: address.ObjectID, Address: address, Status: status}, nil
	}
	hint := unit.PathHint
	if hint == "" {
		hint = unit.Path
	}
	return knowledge.Resolution{
		Repository: r.ID(), Commit: commit, ObjectID: address.ObjectID,
		Address: unit.Address, PathHint: hint, Digest: unit.Digest,
		DeclarationDigest: knowledge.DeclarationDigest(unit.SchemaRef, unit.ValueSource),
		SchemaRef:         unit.SchemaRef, ValueSource: unit.ValueSource, Status: knowledge.StatusResolved,
	}, nil
}

func (r *treeRepository) ReadAddress(address knowledge.Address, commit kernel.CommitID) (knowledge.KnowledgeValue, error) {
	if err := knowledge.AssertWritable(address); err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	units, err := readObjectUnits(r.tree, r.locator, address.ObjectID, commit)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	var unit repofile.Unit
	ok := false
	for _, candidate := range units {
		if knowledge.AddressKey(candidate.Address) == knowledge.AddressKey(address) {
			unit, ok = candidate, true
			break
		}
	}
	if !ok {
		return knowledge.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "address %s is missing at commit %s", knowledge.AddressKey(address), commit)
	}
	return knowledge.KnowledgeValue{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: r.ID(), Object: address.ObjectID},
		Repository:   r.ID(), Commit: commit, Address: unit.Address,
		Value: unit.Value, Provenance: unit.Provenance,
		Declarations: []knowledge.UnitDeclaration{repofile.DeclarationOf(unit)},
	}, nil
}

func (r *treeRepository) GetProvenance(objectID knowledge.ObjectID, commit kernel.CommitID) (knowledge.ProvenanceTrace, error) {
	units, err := readObjectUnits(r.tree, r.locator, objectID, commit)
	if err != nil {
		return knowledge.ProvenanceTrace{}, err
	}
	units = append([]repofile.Unit{}, units...)
	if len(units) == 0 {
		return knowledge.ProvenanceTrace{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "object %s is missing at commit %s", objectID, commit)
	}
	sort.Slice(units, func(i, j int) bool {
		return knowledge.AddressKey(units[i].Address) < knowledge.AddressKey(units[j].Address)
	})
	chain := []knowledge.ProvenanceEnvelope{}
	for _, unit := range units {
		if unit.Provenance != nil {
			chain = append(chain, *unit.Provenance)
		}
	}
	return knowledge.ProvenanceTrace{Repository: r.ID(), Commit: commit, ObjectID: objectID, Chain: chain}, nil
}
