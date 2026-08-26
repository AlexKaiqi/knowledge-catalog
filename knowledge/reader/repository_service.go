package reader

import (
	"container/list"
	"sort"
	"sync"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// The Reader is the layer ② service boundary over the layer ⓪ registry. It
// wraps Snapshot members once so every consumer (Reader, Workspace Serving,
// and retrieval hydration) shares the same Canonical object cache.
const defaultCanonicalCacheEntries = 2048

type canonicalKey struct {
	repository kernel.RepositoryID
	commit     kernel.CommitID
	objectID   knowledge.ObjectID
}

type canonicalEntry struct {
	key   canonicalKey
	value knowledge.KnowledgeValue
}

type canonicalCache struct {
	mu      sync.Mutex
	limit   int
	entries map[canonicalKey]*list.Element
	lru     *list.List
}

func newCanonicalCache(limit int) *canonicalCache {
	if limit < 1 {
		limit = 1
	}
	return &canonicalCache{limit: limit, entries: map[canonicalKey]*list.Element{}, lru: list.New()}
}

func (c *canonicalCache) get(key canonicalKey) (knowledge.KnowledgeValue, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[key]
	if !ok {
		return knowledge.KnowledgeValue{}, false
	}
	c.lru.MoveToFront(element)
	return element.Value.(canonicalEntry).value, true
}

func (c *canonicalCache) put(key canonicalKey, value knowledge.KnowledgeValue) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.entries[key]; ok {
		element.Value = canonicalEntry{key: key, value: value}
		c.lru.MoveToFront(element)
		return
	}
	element := c.lru.PushFront(canonicalEntry{key: key, value: value})
	c.entries[key] = element
	for c.lru.Len() > c.limit {
		oldest := c.lru.Back()
		entry := oldest.Value.(canonicalEntry)
		delete(c.entries, entry.key)
		c.lru.Remove(oldest)
	}
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
// read service wrapper. The underlying adapter may implement knowledge writes,
// but its read-side cache is never relied upon by callers.
func (r *Reader) Wrap(store snapshot.Store, code kernel.ErrorCode) (knowledge.Repository, error) {
	native, ok := knowledge.Of(store)
	if !ok {
		return nil, kernel.Fail(code, "repository %s is mounted as a plain snapshot and does not interpret knowledge files", store.ID())
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.repos[store.ID()]; ok {
		return existing, nil
	}
	wrapped := &cachedRepository{base: native, cache: r.cache}
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

type cachedRepository struct {
	base  knowledge.Repository
	cache *canonicalCache
}

var (
	_ knowledge.Repository     = (*cachedRepository)(nil)
	_ knowledge.BatchReadStore = (*cachedRepository)(nil)
	_ knowledge.FastChanges    = (*cachedRepository)(nil)
)

func (r *cachedRepository) ID() kernel.RepositoryID                   { return r.base.ID() }
func (r *cachedRepository) Head(ref string) (kernel.CommitID, error)  { return r.base.Head(ref) }
func (r *cachedRepository) GetRef(ref string) (kernel.CommitID, bool) { return r.base.GetRef(ref) }
func (r *cachedRepository) HasCommit(commit kernel.CommitID) bool     { return r.base.HasCommit(commit) }
func (r *cachedRepository) CreateRef(ref string, commit kernel.CommitID) error {
	return r.base.CreateRef(ref, commit)
}
func (r *cachedRepository) Merge(ref string, candidate, expected kernel.CommitID) (kernel.CommitID, error) {
	return r.base.Merge(ref, candidate, expected)
}
func (r *cachedRepository) Archived() bool { return r.base.Archived() }
func (r *cachedRepository) Archive() error { return r.base.Archive() }
func (r *cachedRepository) ApplyKnowledgeCommit(cs knowledge.ChangeSet) (kernel.CommitID, error) {
	return r.base.ApplyKnowledgeCommit(cs)
}

func (r *cachedRepository) cacheKey(commit kernel.CommitID, objectID knowledge.ObjectID) canonicalKey {
	return canonicalKey{repository: r.ID(), commit: commit, objectID: objectID}
}

func readKnowledgeTree(store snapshot.TreeStore, commit kernel.CommitID) (*repofile.Tree, error) {
	paths, err := store.ListFiles(commit)
	if err != nil {
		return nil, err
	}
	tree := repofile.NewTree()
	for _, path := range paths {
		if !repofile.KnowledgePath(path) {
			continue
		}
		content, err := store.ReadFile(path, commit)
		if err != nil {
			return nil, err
		}
		unit := repofile.Parse(string(content))
		if unit == nil {
			continue
		}
		if err := repofile.Ingest(tree, unit, path); err != nil {
			return nil, err
		}
	}
	return tree, nil
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

func (r *cachedRepository) ReadMany(objectIDs []knowledge.ObjectID, commit kernel.CommitID) (map[knowledge.ObjectID]knowledge.KnowledgeValue, error) {
	out := map[knowledge.ObjectID]knowledge.KnowledgeValue{}
	missing := make([]knowledge.ObjectID, 0, len(objectIDs))
	seen := map[knowledge.ObjectID]struct{}{}
	for _, objectID := range objectIDs {
		if objectID == "" {
			continue
		}
		if _, duplicate := seen[objectID]; duplicate {
			continue
		}
		seen[objectID] = struct{}{}
		if value, ok := r.cache.get(r.cacheKey(commit, objectID)); ok {
			out[objectID] = value
			continue
		}
		missing = append(missing, objectID)
	}
	if len(missing) == 0 {
		return out, nil
	}
	treeStore, ok := snapshot.TreeStoreOf(r.base)
	if !ok {
		for _, objectID := range missing {
			value, err := r.base.Read(objectID, commit)
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				continue
			}
			if err != nil {
				return nil, err
			}
			r.cache.put(r.cacheKey(commit, objectID), value)
			out[objectID] = value
		}
		return out, nil
	}
	tree, err := readKnowledgeTree(treeStore, commit)
	if err != nil {
		return nil, err
	}
	for _, objectID := range missing {
		units := tree.ObjectUnits(objectID)
		if len(units) == 0 {
			continue
		}
		value, err := assembleKnowledgeValue(r.ID(), objectID, commit, units)
		if err != nil {
			return nil, err
		}
		r.cache.put(r.cacheKey(commit, objectID), value)
		out[objectID] = value
	}
	return out, nil
}

func (r *cachedRepository) Read(objectID knowledge.ObjectID, commit kernel.CommitID) (knowledge.KnowledgeValue, error) {
	values, err := r.ReadMany([]knowledge.ObjectID{objectID}, commit)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	if value, ok := values[objectID]; ok {
		return value, nil
	}
	return knowledge.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "object %s is missing at commit %s", objectID, commit)
}

func (r *cachedRepository) Resolve(objectID knowledge.ObjectID, commit kernel.CommitID) (knowledge.Resolution, error) {
	treeStore, ok := snapshot.TreeStoreOf(r.base)
	if !ok {
		return r.base.Resolve(objectID, commit)
	}
	tree, err := readKnowledgeTree(treeStore, commit)
	if err != nil {
		return knowledge.Resolution{}, err
	}
	units := tree.ObjectUnits(objectID)
	if len(units) == 0 {
		return r.base.Resolve(objectID, commit)
	}
	value, err := assembleKnowledgeValue(r.ID(), objectID, commit, units)
	if err != nil {
		return knowledge.Resolution{}, err
	}
	r.cache.put(r.cacheKey(commit, objectID), value)
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

func (r *cachedRepository) ResolveAddress(address knowledge.Address, commit kernel.CommitID) (knowledge.Resolution, error) {
	if err := knowledge.AssertWritable(address); err != nil {
		return knowledge.Resolution{}, err
	}
	treeStore, ok := snapshot.TreeStoreOf(r.base)
	if !ok {
		return r.base.ResolveAddress(address, commit)
	}
	tree, err := readKnowledgeTree(treeStore, commit)
	if err != nil {
		return knowledge.Resolution{}, err
	}
	unit, ok := tree.Units[knowledge.AddressKey(address)]
	if !ok {
		return r.base.ResolveAddress(address, commit)
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

func (r *cachedRepository) ReadAddress(address knowledge.Address, commit kernel.CommitID) (knowledge.KnowledgeValue, error) {
	if err := knowledge.AssertWritable(address); err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	treeStore, ok := snapshot.TreeStoreOf(r.base)
	if !ok {
		return r.base.ReadAddress(address, commit)
	}
	tree, err := readKnowledgeTree(treeStore, commit)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	unit, ok := tree.Units[knowledge.AddressKey(address)]
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

func (r *cachedRepository) GetProvenance(objectID knowledge.ObjectID, commit kernel.CommitID) (knowledge.ProvenanceTrace, error) {
	treeStore, ok := snapshot.TreeStoreOf(r.base)
	if !ok {
		return r.base.GetProvenance(objectID, commit)
	}
	tree, err := readKnowledgeTree(treeStore, commit)
	if err != nil {
		return knowledge.ProvenanceTrace{}, err
	}
	units := append([]repofile.Unit{}, tree.ObjectUnits(objectID)...)
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

func (r *cachedRepository) List(commit kernel.CommitID) ([]knowledge.KnowledgeValue, error) {
	treeStore, ok := snapshot.TreeStoreOf(r.base)
	if !ok {
		return r.base.List(commit)
	}
	tree, err := readKnowledgeTree(treeStore, commit)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(tree.ByObject))
	for objectID := range tree.ByObject {
		ids = append(ids, string(objectID))
	}
	sort.Strings(ids)
	out := make([]knowledge.KnowledgeValue, 0, len(ids))
	for _, raw := range ids {
		objectID := knowledge.ObjectID(raw)
		value, err := assembleKnowledgeValue(r.ID(), objectID, commit, tree.ObjectUnits(objectID))
		if err != nil {
			return nil, err
		}
		r.cache.put(r.cacheKey(commit, objectID), value)
		out = append(out, value)
	}
	return out, nil
}

func (r *cachedRepository) Log(objectID knowledge.ObjectID, commit kernel.CommitID, limit int) ([]knowledge.ObjectRevision, error) {
	return r.base.Log(objectID, commit, limit)
}

func (r *cachedRepository) Diff(objectID knowledge.ObjectID, from, to kernel.CommitID) (knowledge.ObjectDiff, error) {
	read := func(commit kernel.CommitID) (*knowledge.KnowledgeValue, error) {
		value, err := r.Read(objectID, commit)
		if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &value, nil
	}
	left, err := read(from)
	if err != nil {
		return knowledge.ObjectDiff{}, err
	}
	right, err := read(to)
	if err != nil {
		return knowledge.ObjectDiff{}, err
	}
	return knowledge.ObjectDiff{ObjectID: objectID, FromCommit: from, ToCommit: to, From: left, To: right}, nil
}

func (r *cachedRepository) FastChangedObjectIDs(from, to kernel.CommitID) ([]knowledge.ObjectID, error) {
	if fast, ok := r.base.(knowledge.FastChanges); ok {
		return fast.FastChangedObjectIDs(from, to)
	}
	return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s has no fast changed-object scan", r.ID())
}
