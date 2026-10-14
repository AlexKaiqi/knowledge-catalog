package reader

import (
	"sort"
	"strings"

	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/snapshot"
)

// Lookup creates one shared layer ② interpreter over a Snapshot lookup seam.
// It is used when the caller owns membership (for example Dataset service) but does
// not expose its Registry.
func Lookup(base func(kernel.RepositoryID) (snapshot.Store, error)) MemberLookup {
	service := NewReader(nil)
	return service.Lookup(base)
}

// Require resolves a mounted Snapshot and exposes its layer ② capability
// through the shared Knowledge service. Dataset service remains Snapshot-only.
func (r *Reader) Require(repositoryID kernel.RepositoryID, code kernel.ErrorCode) (objectmodel.Repository, error) {
	store, err := r.store.Require(repositoryID, code)
	if err != nil {
		return nil, err
	}
	return r.Wrap(store, code)
}

// Wrap converts one Dataset service/Snapshot member into the process-wide Knowledge
// read service wrapper. Interpretation remains here;
// the underlying adapter exposes only immutable tree bytes.
func (r *Reader) Wrap(store snapshot.Store, code kernel.ErrorCode) (objectmodel.Repository, error) {
	if native, ok := store.(objectmodel.NativeRepository); ok {
		view := &nativeReadRepository{readRepository: native}
		r.mu.Lock()
		r.repos[store.ID()] = view
		r.mu.Unlock()
		return view, nil
	}
	tree, ok := snapshot.TreeReaderOf(store)
	if !ok {
		return nil, kernel.Fail(code, "repository %s has no immutable tree access for knowledge interpretation", store.ID())
	}
	locator, ok := store.(objectmodel.UnitLocator)
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

// Lookup preserves Dataset service membership checks while moving knowledge
// interpretation to this service boundary.
func (r *Reader) Lookup(base func(kernel.RepositoryID) (snapshot.Store, error)) MemberLookup {
	return func(id kernel.RepositoryID) (objectmodel.Repository, error) {
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
	locator objectmodel.UnitLocator
}

var (
	_ objectmodel.Repository          = (*treeRepository)(nil)
	_ objectmodel.BatchReadStore      = (*treeRepository)(nil)
	_ objectmodel.FastChanges         = (*treeRepository)(nil)
	_ objectmodel.FastObjectChanges   = (*treeRepository)(nil)
	_ objectmodel.SnapshotObjectPager = (*treeRepository)(nil)
	_ objectmodel.UnitPathsHydrator   = (*treeRepository)(nil)
	_ objectmodel.KnowledgeFileReader = (*treeRepository)(nil)
	_ objectmodel.UnitLocator         = (*treeRepository)(nil)
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
func (r *treeRepository) Archived() bool                            { return r.base.Archived() }

func (r *treeRepository) ObjectUnitPaths(objectID objectmodel.ObjectID, commit kernel.CommitID) ([]string, error) {
	paths, err := r.objectUnitPathsMany([]objectmodel.ObjectID{objectID}, commit)
	if err != nil {
		return nil, err
	}
	return paths[objectID], nil
}

func (r *treeRepository) SchemaObjectIDs(commit kernel.CommitID) ([]objectmodel.ObjectID, error) {
	locator, ok := r.locator.(objectmodel.SchemaStore)
	if !ok {
		locator, ok = r.base.(objectmodel.SchemaStore)
	}
	if !ok {
		directory, _ := snapshot.DirectoryReaderOf(r.base)
		locator = &treeManifestLocator{tree: r.tree, directory: directory}
	}
	return locator.SchemaObjectIDs(commit)
}

func (r *treeRepository) BindingSchemaObjectIDs(commit kernel.CommitID) ([]objectmodel.ObjectID, error) {
	return r.bindingSchemaCoordinates(commit)
}

func (r *treeRepository) bindingSchemaCoordinates(commit kernel.CommitID) ([]objectmodel.ObjectID, error) {
	locator := &treeManifestLocator{tree: r.tree}
	return locator.BindingSchemaObjectIDs(commit)
}

func (r *treeRepository) SchemaReferrerAddresses(schema objectmodel.ObjectID, commit kernel.CommitID) ([]objectmodel.Address, error) {
	return repofile.ReadSchemaReferrers(r.tree, schema, commit)
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
			return repofile.LocatorManifest{Objects: map[objectmodel.ObjectID][]string{}}, nil
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

func (l *treeManifestLocator) ObjectUnitPaths(objectID objectmodel.ObjectID, commit kernel.CommitID) ([]string, error) {
	return repofile.ReadObjectLocator(l.tree, objectID, commit)
}

func (l *treeManifestLocator) SchemaObjectIDs(commit kernel.CommitID) ([]objectmodel.ObjectID, error) {
	if locatorLayoutComplete(l.tree, commit) {
		raw, err := l.tree.ReadFile(repofile.LocatorSchemaIndexPath, commit)
		if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
			return []objectmodel.ObjectID{}, nil
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
	return append([]objectmodel.ObjectID(nil), manifest.Schemas...), nil
}

func (l *treeManifestLocator) BindingSchemaObjectIDs(commit kernel.CommitID) ([]objectmodel.ObjectID, error) {
	if locatorLayoutComplete(l.tree, commit) {
		raw, err := l.tree.ReadFile(repofile.LocatorBindingIndexPath, commit)
		if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
			ids, schemaErr := l.SchemaObjectIDs(commit)
			if schemaErr != nil || len(ids) == 0 {
				return ids, schemaErr
			}
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "knowledge layout has no binding namespace coordinates; explicitly rebuild locators")
		}
		if err != nil {
			return nil, err
		}
		ids, err := repofile.DecodeSchemaIndex(raw)
		if err != nil {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "invalid binding namespace coordinates at %s: %v", commit, err)
		}
		return ids, nil
	}
	manifest, err := l.load(commit)
	if err != nil {
		return nil, err
	}
	return append([]objectmodel.ObjectID(nil), manifest.BindingSchemas...), nil
}

func (l *treeManifestLocator) SchemaReferrerAddresses(schema objectmodel.ObjectID, commit kernel.CommitID) ([]objectmodel.Address, error) {
	return repofile.ReadSchemaReferrers(l.tree, schema, commit)
}

func readObjectUnitsAtPaths(store snapshot.TreeReader, repository kernel.RepositoryID, paths []string, objectID objectmodel.ObjectID, commit kernel.CommitID) ([]repofile.Unit, error) {
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
	return classifyMemberSources(store, repository, commit, tree.ObjectUnits(objectID))
}

func (r *treeRepository) objectUnits(objectID objectmodel.ObjectID, commit kernel.CommitID) ([]repofile.Unit, error) {
	paths, err := r.objectUnitPathsMany([]objectmodel.ObjectID{objectID}, commit)
	if err != nil {
		return nil, err
	}
	return readObjectUnitsAtPaths(r.tree, r.ID(), paths[objectID], objectID, commit)
}

func assembleKnowledgeValue(repository kernel.RepositoryID, objectID objectmodel.ObjectID, commit kernel.CommitID, units []repofile.Unit) (objectmodel.KnowledgeValue, error) {
	return objectmodel.AssembleObjectValue(repository, objectID, commit, repofile.ToObjectUnits(units))
}

func (r *treeRepository) ReadMany(objectIDs []objectmodel.ObjectID, commit kernel.CommitID) (map[objectmodel.ObjectID]objectmodel.KnowledgeValue, error) {
	out := map[objectmodel.ObjectID]objectmodel.KnowledgeValue{}
	seen := map[objectmodel.ObjectID]struct{}{}
	ids := make([]objectmodel.ObjectID, 0, len(objectIDs))
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
		units, err := readObjectUnitsAtPaths(r.tree, r.ID(), paths[objectID], objectID, commit)
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

func (r *treeRepository) objectUnitPathsMany(objectIDs []objectmodel.ObjectID, commit kernel.CommitID) (map[objectmodel.ObjectID][]string, error) {
	paths := make(map[objectmodel.ObjectID][]string, len(objectIDs))
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
	if raw, err := r.tree.ReadFile(objectmodel.RepositoryReadmePath, commit); err == nil {
		if unit := repofile.Parse(string(raw)); unit != nil {
			if _, wanted := paths[unit.ObjectID]; wanted {
				if !containsPath(paths[unit.ObjectID], objectmodel.RepositoryReadmePath) {
					paths[unit.ObjectID] = append(paths[unit.ObjectID], objectmodel.RepositoryReadmePath)
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

func (r *treeRepository) Read(objectID objectmodel.ObjectID, commit kernel.CommitID) (objectmodel.KnowledgeValue, error) {
	values, err := r.ReadMany([]objectmodel.ObjectID{objectID}, commit)
	if err != nil {
		return objectmodel.KnowledgeValue{}, err
	}
	if value, ok := values[objectID]; ok {
		return value, nil
	}
	return objectmodel.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "object %s is missing at commit %s", objectID, commit)
}

func (r *treeRepository) ObjectIDsPage(commit kernel.CommitID, limit int, continuation string) (objectmodel.ObjectIDPage, error) {
	if limit <= 0 {
		return objectmodel.ObjectIDPage{}, kernel.Fail(kernel.ErrUsageInvalid, "object identity page limit must be positive")
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
				return r.mergeReadmeObjectID(objectmodel.ObjectIDPage{Exhausted: true}, commit)
			}
			return objectmodel.ObjectIDPage{}, err
		}
		page := objectmodel.ObjectIDPage{
			Continuation: directoryPage.Continuation,
			Exhausted:    directoryPage.Exhausted,
		}
		for _, entry := range directoryPage.Entries {
			if entry.Kind != "file" {
				return objectmodel.ObjectIDPage{}, kernel.Fail(kernel.ErrPreconditionFailed,
					"object locator directory contains non-file entry %s", entry.Name)
			}
			raw, err := r.tree.ReadFile(repofile.LocatorObjectDirectory+"/"+entry.Name, commit)
			if err != nil {
				return objectmodel.ObjectIDPage{}, err
			}
			locatorEntry, err := repofile.DecodeObjectLocatorEntry(raw)
			if err != nil {
				return objectmodel.ObjectIDPage{}, kernel.Fail(kernel.ErrPreconditionFailed,
					"invalid object locator %s: %v", entry.Name, err)
			}
			page.ObjectIDs = append(page.ObjectIDs, locatorEntry.ObjectID)
		}
		return r.mergeReadmeObjectID(page, commit)
	}
	manifest, err := (&treeManifestLocator{tree: r.tree}).load(commit)
	if err != nil {
		return objectmodel.ObjectIDPage{}, err
	}
	ids := make([]objectmodel.ObjectID, 0, len(manifest.Objects))
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
	page := objectmodel.ObjectIDPage{ObjectIDs: pageIDs, Exhausted: end == len(ids)}
	if !page.Exhausted && len(pageIDs) > 0 {
		page.Continuation = string(pageIDs[len(pageIDs)-1])
	}
	return r.mergeReadmeObjectID(page, commit)
}

func (r *treeRepository) readmeObjectID(commit kernel.CommitID) (objectmodel.ObjectID, bool, error) {
	raw, err := r.tree.ReadFile(objectmodel.RepositoryReadmePath, commit)
	if err != nil {
		if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
			return "", false, nil
		}
		return "", false, err
	}
	unit := repofile.Parse(string(raw))
	if unit == nil || !objectmodel.IsReadmeAddress(unit.Address, unit.SchemaRef) {
		return "", false, nil
	}
	return unit.ObjectID, true, nil
}

func (r *treeRepository) mergeReadmeObjectID(page objectmodel.ObjectIDPage, commit kernel.CommitID) (objectmodel.ObjectIDPage, error) {
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
		return objectmodel.ObjectIDPage{}, err
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

func (r *treeRepository) Resolve(objectID objectmodel.ObjectID, commit kernel.CommitID) (objectmodel.Resolution, error) {
	units, err := r.objectUnits(objectID, commit)
	if err != nil {
		return objectmodel.Resolution{}, err
	}
	if len(units) == 0 {
		// Object-level Resolve keeps the protocol REMOVED status for objects
		// deleted before this basis (provider-independent conformance);
		// never-existing objects resolve as UNRESOLVED.
		status, err := r.missingStatus(objectID, commit)
		if err != nil {
			return objectmodel.Resolution{}, err
		}
		return objectmodel.Resolution{Repository: r.ID(), Commit: commit, ObjectID: objectID,
			Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectID}, Status: status}, nil
	}
	value, err := assembleKnowledgeValue(r.ID(), objectID, commit, units)
	if err != nil {
		return objectmodel.Resolution{}, err
	}
	resolution := objectmodel.Resolution{
		Repository: r.ID(), Commit: commit, ObjectID: objectID,
		Address: value.Address, Digest: kernel.CanonicalDigest(value.Value),
		PathHint:          repofile.EntityPathHint(units, objectID),
		DeclarationDigest: repofile.TreeDeclarationDigest(units),
		Status:            objectmodel.StatusResolved,
	}
	if len(units) == 1 && units[0].Address.Kind == objectmodel.KindRelation {
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

func (r *treeRepository) ResolveAddress(address objectmodel.Address, commit kernel.CommitID) (objectmodel.Resolution, error) {
	unit, ok, err := r.addressUnit(address, commit, nil)
	if err != nil {
		return objectmodel.Resolution{}, err
	}
	if !ok {
		return objectmodel.Resolution{Repository: r.ID(), Commit: commit, ObjectID: address.ObjectID, Address: address, Status: objectmodel.StatusUnresolved}, nil
	}
	hint := unit.PathHint
	if hint == "" {
		hint = unit.Path
	}
	return objectmodel.Resolution{
		Repository: r.ID(), Commit: commit, ObjectID: address.ObjectID,
		Address: unit.Address, PathHint: hint, Digest: unit.Digest,
		DeclarationDigest: objectmodel.DeclarationDigest(unit.SchemaRef, unit.ValueSource),
		SchemaRef:         unit.SchemaRef, ValueSource: unit.ValueSource, Status: objectmodel.StatusResolved,
	}, nil
}

func (r *treeRepository) ReadAddress(address objectmodel.Address, commit kernel.CommitID) (objectmodel.KnowledgeValue, error) {
	unit, ok, err := r.addressUnit(address, commit, nil)
	if err != nil {
		return objectmodel.KnowledgeValue{}, err
	}
	if !ok {
		return objectmodel.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "address %s is missing at commit %s", objectmodel.AddressKey(address), commit)
	}
	return objectmodel.KnowledgeValue{
		KnowledgeRef: objectmodel.KnowledgeRef{Repository: r.ID(), Object: address.ObjectID},
		Repository:   r.ID(), Commit: commit, Address: unit.Address,
		Value: unit.Value, Provenance: unit.Provenance,
		Declarations: []objectmodel.UnitDeclaration{repofile.DeclarationOf(unit)},
	}, nil
}

func (r *treeRepository) GetProvenance(objectID objectmodel.ObjectID, commit kernel.CommitID) (objectmodel.ProvenanceTrace, error) {
	units, err := r.objectUnits(objectID, commit)
	if err != nil {
		return objectmodel.ProvenanceTrace{}, err
	}
	units = append([]repofile.Unit{}, units...)
	if len(units) == 0 {
		return objectmodel.ProvenanceTrace{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "object %s is missing at commit %s", objectID, commit)
	}
	sort.Slice(units, func(i, j int) bool {
		return objectmodel.AddressKey(units[i].Address) < objectmodel.AddressKey(units[j].Address)
	})
	chain := []objectmodel.ProvenanceEnvelope{}
	for _, unit := range units {
		if unit.Provenance != nil {
			chain = append(chain, *unit.Provenance)
		}
	}
	return objectmodel.ProvenanceTrace{Repository: r.ID(), Commit: commit, ObjectID: objectID, Chain: chain}, nil
}
