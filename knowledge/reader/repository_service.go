package reader

import (
	"sort"
	"strings"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/unitcodec"
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
	tree, ok := snapshot.TreeReaderOf(store)
	if !ok {
		return nil, kernel.Fail(code, "repository %s has no immutable tree access for knowledge interpretation", store.ID())
	}
	locator, ok := store.(knowledge.UnitLocator)
	if !ok {
		directory, _ := snapshot.DirectoryReaderOf(store)
		locator = &treeManifestLocator{tree: tree, directory: directory}
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
	tree    snapshot.TreeReader
	locator knowledge.UnitLocator
}

var (
	_ knowledge.Repository          = (*treeRepository)(nil)
	_ knowledge.BatchReadStore      = (*treeRepository)(nil)
	_ knowledge.FastChanges         = (*treeRepository)(nil)
	_ knowledge.FastObjectChanges   = (*treeRepository)(nil)
	_ knowledge.SnapshotObjectPager = (*treeRepository)(nil)
	_ knowledge.UnitPathsHydrator   = (*treeRepository)(nil)
	_ knowledge.KnowledgeFileReader = (*treeRepository)(nil)
	_ knowledge.UnitLocator         = (*treeRepository)(nil)
)

func (r *treeRepository) ID() kernel.RepositoryID { return r.base.ID() }

// StoreDigest binds this knowledge view to its concrete authority instance:
// two deployments of one logical repository id (two lakeFS endpoints serving
// "the same" repository) produce different digests, so per-repository state
// such as retrieval projections can never be shared between them.
func (r *treeRepository) StoreDigest() kernel.Digest {
	if origin, ok := r.base.(snapshot.StoreOrigin); ok {
		if coordinate := strings.TrimSpace(origin.Origin()); coordinate != "" {
			return kernel.CanonicalDigest(map[string]any{
				"repository": string(r.base.ID()), "origin": coordinate,
			})
		}
	}
	return ""
}

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

func (r *treeRepository) ObjectUnitPaths(objectID knowledge.ObjectID, commit kernel.CommitID) ([]string, error) {
	paths, err := r.objectUnitPathsMany([]knowledge.ObjectID{objectID}, commit)
	if err != nil {
		return nil, err
	}
	return paths[objectID], nil
}

func (r *treeRepository) SchemaObjectIDs(commit kernel.CommitID) ([]knowledge.ObjectID, error) {
	locator, ok := r.locator.(knowledge.SchemaStore)
	if !ok {
		locator, ok = r.base.(knowledge.SchemaStore)
	}
	if !ok {
		directory, _ := snapshot.DirectoryReaderOf(r.base)
		locator = &treeManifestLocator{tree: r.tree, directory: directory}
	}
	return locator.SchemaObjectIDs(commit)
}

func (r *treeRepository) BindingSchemaObjectIDs(commit kernel.CommitID) ([]knowledge.ObjectID, error) {
	locator, ok := r.locator.(knowledge.BindingLocator)
	if !ok {
		locator, ok = r.base.(knowledge.BindingLocator)
	}
	if !ok {
		directory, _ := snapshot.DirectoryReaderOf(r.base)
		locator = &treeManifestLocator{tree: r.tree, directory: directory}
	}
	return locator.BindingSchemaObjectIDs(commit)
}

func (r *treeRepository) SchemaReferrerAddresses(schema knowledge.ObjectID, commit kernel.CommitID) ([]knowledge.Address, error) {
	locator, ok := r.locator.(knowledge.SchemaReferrerLocator)
	if !ok {
		locator, ok = r.base.(knowledge.SchemaReferrerLocator)
	}
	if !ok {
		directory, _ := snapshot.DirectoryReaderOf(r.base)
		locator = &treeManifestLocator{tree: r.tree, directory: directory}
	}
	return locator.SchemaReferrerAddresses(schema, commit)
}

// treeManifestLocator gives Gitea and other tree authorities bounded layer ②
// reads without teaching layer ⓪ about object_id. The Writer versions this
// manifest in the same commit as the units; it is not a relation/search index.
type treeManifestLocator struct {
	tree      snapshot.TreeReader
	directory snapshot.DirectoryReader
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
	return repofile.ReadObjectLocator(l.tree, objectID, commit)
}

func (l *treeManifestLocator) SchemaObjectIDs(commit kernel.CommitID) ([]knowledge.ObjectID, error) {
	if locatorLayoutComplete(l.tree, commit) {
		raw, err := l.tree.ReadFile(repofile.LocatorSchemaIndexPath, commit)
		if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
			return []knowledge.ObjectID{}, nil
		}
		if err != nil {
			return nil, err
		}
		ids, err := repofile.DecodeSchemaIndex(raw)
		if err != nil {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed,
				"invalid schema locator at %s: %v", commit, err)
		}
		return ids, nil
	}
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

func readObjectUnitsAtPaths(store snapshot.TreeReader, paths []string, objectID knowledge.ObjectID, commit kernel.CommitID) ([]repofile.Unit, error) {
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

func (r *treeRepository) objectUnits(objectID knowledge.ObjectID, commit kernel.CommitID) ([]repofile.Unit, error) {
	paths, err := r.objectUnitPathsMany([]knowledge.ObjectID{objectID}, commit)
	if err != nil {
		return nil, err
	}
	return readObjectUnitsAtPaths(r.tree, paths[objectID], objectID, commit)
}

func assembleKnowledgeValue(repository kernel.RepositoryID, objectID knowledge.ObjectID, commit kernel.CommitID, units []repofile.Unit) (knowledge.KnowledgeValue, error) {
	core := make([]unitcodec.Unit, 0, len(units))
	for _, unit := range units {
		core = append(core, unitcodec.Unit{
			ObjectID: unit.ObjectID, Address: unit.Address, PathHint: unit.PathHint,
			SchemaRef: unit.SchemaRef, ValueSource: unit.ValueSource,
			Provenance: unit.Provenance, Value: unit.Value, Digest: unit.Digest,
		})
	}
	return unitcodec.AssembleKnowledgeValue(repository, objectID, commit, core)
}

func (r *treeRepository) ReadMany(objectIDs []knowledge.ObjectID, commit kernel.CommitID) (map[knowledge.ObjectID]knowledge.KnowledgeValue, error) {
	out := map[knowledge.ObjectID]knowledge.KnowledgeValue{}
	seen := map[knowledge.ObjectID]struct{}{}
	ids := make([]knowledge.ObjectID, 0, len(objectIDs))
	for _, objectID := range objectIDs {
		if objectID == "" {
			continue
		}
		if _, duplicate := seen[objectID]; duplicate {
			continue
		}
		seen[objectID] = struct{}{}
		ids = append(ids, objectID)
	}
	paths, err := r.objectUnitPathsMany(ids, commit)
	if err != nil {
		return nil, err
	}
	for _, objectID := range ids {
		units, err := readObjectUnitsAtPaths(r.tree, paths[objectID], objectID, commit)
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

func (r *treeRepository) objectUnitPathsMany(objectIDs []knowledge.ObjectID, commit kernel.CommitID) (map[knowledge.ObjectID][]string, error) {
	paths := make(map[knowledge.ObjectID][]string, len(objectIDs))
	if len(objectIDs) == 0 {
		return paths, nil
	}
	for _, objectID := range objectIDs {
		unitPaths, err := r.locator.ObjectUnitPaths(objectID, commit)
		if err != nil {
			return nil, err
		}
		paths[objectID] = unitPaths
	}
	if raw, err := r.tree.ReadFile(knowledge.RepositoryReadmePath, commit); err == nil {
		if unit := repofile.Parse(string(raw)); unit != nil {
			if _, wanted := paths[unit.ObjectID]; wanted {
				if !containsPath(paths[unit.ObjectID], knowledge.RepositoryReadmePath) {
					paths[unit.ObjectID] = append(paths[unit.ObjectID], knowledge.RepositoryReadmePath)
				}
			}
		}
	} else if kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
		return nil, err
	}
	return paths, nil
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
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
	if directory, ok := snapshot.DirectoryReaderOf(r.base); ok && locatorLayoutComplete(r.tree, commit) {
		// DirectoryReader implementations share 500 as their portable maximum.
		// A caller's larger object page remains valid by returning a
		// continuation rather than forwarding an adapter-invalid limit.
		if limit > 500 {
			limit = 500
		}
		directoryPage, err := directory.ReadDirectory(snapshot.DirectoryRequest{
			Commit: commit, Directory: repofile.LocatorObjectDirectory,
			Limit: limit, Continuation: continuation,
		})
		if err != nil {
			// Git-style authorities cannot retain an empty directory. Once the
			// complete-layout marker exists, a missing object-locator directory
			// therefore represents an empty live object set, not a corrupt
			// locator layout. Other provider failures still fail closed.
			if continuation == "" && kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				return r.mergeReadmeObjectID(knowledge.ObjectIDPage{Exhausted: true}, commit)
			}
			return knowledge.ObjectIDPage{}, err
		}
		page := knowledge.ObjectIDPage{
			Continuation: directoryPage.Continuation,
			Exhausted:    directoryPage.Exhausted,
		}
		for _, entry := range directoryPage.Entries {
			if entry.Kind != "file" {
				return knowledge.ObjectIDPage{}, kernel.Fail(kernel.ErrPreconditionFailed,
					"object locator directory contains non-file entry %s", entry.Name)
			}
			raw, err := r.tree.ReadFile(repofile.LocatorObjectDirectory+"/"+entry.Name, commit)
			if err != nil {
				return knowledge.ObjectIDPage{}, err
			}
			locatorEntry, err := repofile.DecodeObjectLocatorEntry(raw)
			if err != nil {
				return knowledge.ObjectIDPage{}, kernel.Fail(kernel.ErrPreconditionFailed,
					"invalid object locator %s: %v", entry.Name, err)
			}
			page.ObjectIDs = append(page.ObjectIDs, locatorEntry.ObjectID)
		}
		return r.mergeReadmeObjectID(page, commit)
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
	return r.mergeReadmeObjectID(page, commit)
}

func (r *treeRepository) readmeObjectID(commit kernel.CommitID) (knowledge.ObjectID, bool, error) {
	raw, err := r.tree.ReadFile(knowledge.RepositoryReadmePath, commit)
	if err != nil {
		if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
			return "", false, nil
		}
		return "", false, err
	}
	unit := repofile.Parse(string(raw))
	if unit == nil || !knowledge.IsReadmeAddress(unit.Address, unit.SchemaRef) {
		return "", false, nil
	}
	return unit.ObjectID, true, nil
}

func (r *treeRepository) mergeReadmeObjectID(page knowledge.ObjectIDPage, commit kernel.CommitID) (knowledge.ObjectIDPage, error) {
	objectID, ok, err := r.readmeObjectID(commit)
	if err != nil || !ok {
		return page, err
	}
	for _, id := range page.ObjectIDs {
		if id == objectID {
			return page, nil
		}
	}
	located, err := repofile.ReadObjectLocator(r.tree, objectID, commit)
	if err != nil {
		return knowledge.ObjectIDPage{}, err
	}
	if len(located) > 0 || !page.Exhausted {
		return page, nil
	}
	page.ObjectIDs = append(page.ObjectIDs, objectID)
	return page, nil
}

func locatorLayoutComplete(tree snapshot.TreeReader, commit kernel.CommitID) bool {
	raw, err := tree.ReadFile(repofile.LocatorCompletePath, commit)
	return err == nil && string(raw) == repofile.LocatorCompleteBody
}

func (r *treeRepository) Resolve(objectID knowledge.ObjectID, commit kernel.CommitID) (knowledge.Resolution, error) {
	units, err := r.objectUnits(objectID, commit)
	if err != nil {
		return knowledge.Resolution{}, err
	}
	if len(units) == 0 {
		// Object-level Resolve keeps the protocol REMOVED status for objects
		// deleted before this basis (provider-independent conformance);
		// never-existing objects resolve as UNRESOLVED.
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
	if len(units) == 1 && units[0].Address.Kind == knowledge.KindRelation {
		// A Relation is an independent N-ary object, not an Entity-shaped
		// container. The composed KnowledgeValue keeps its object-root address,
		// while resolution must preserve the unit's public relation identity.
		resolution.Address = units[0].Address
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
	units, err := r.objectUnits(address.ObjectID, commit)
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
		return knowledge.Resolution{Repository: r.ID(), Commit: commit, ObjectID: address.ObjectID, Address: address, Status: knowledge.StatusUnresolved}, nil
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
	units, err := r.objectUnits(address.ObjectID, commit)
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
	units, err := r.objectUnits(objectID, commit)
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
