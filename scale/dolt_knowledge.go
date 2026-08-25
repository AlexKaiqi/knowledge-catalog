package scale

import (
	"encoding/json"
	"sort"
	"strings"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/repository"
)

func (r *DoltRepository) scanAtLocked(commit kernel.CommitID) (*repofile.Tree, error) {
	files, err := r.snapshotFiles(commit)
	if err != nil {
		return nil, err
	}
	tree := repofile.NewTree()
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if !repofile.KnowledgePath(path) {
			continue
		}
		unit := repofile.Parse(string(files[path]))
		if unit == nil {
			continue
		}
		if err := repofile.Ingest(tree, unit, path); err != nil {
			return nil, err
		}
	}
	return tree, nil
}

func (r *DoltRepository) ApplyCommit(cs repository.CommitChangeSet) (kernel.CommitID, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	if r.archivedLocked() {
		return "", kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", r.repositoryID)
	}
	if err := kernel.ValidateProvenance(cs.Provenance); err != nil {
		return "", err
	}
	if cs.TargetRepository != r.repositoryID {
		return "", kernel.Fail(kernel.ErrTargetRepositoryDenied, "target %s does not match %s", cs.TargetRepository, r.repositoryID)
	}
	if cs.BaseCommit != cs.ExpectedTargetCommit {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "baseCommit must equal expectedTargetCommit")
	}
	branch, ok := doltBranch(cs.TargetRef)
	if !ok {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "unsupported Dolt ref %s", cs.TargetRef)
	}
	current, err := r.queryHash(branch)
	if err != nil || current != cs.ExpectedTargetCommit {
		return "", kernel.Fail(kernel.ErrNonFastForward, "ref %s moved: expected commit %s, actual %s", cs.TargetRef, cs.ExpectedTargetCommit, current)
	}
	tree, err := r.scanAtLocked(cs.BaseCommit)
	if err != nil {
		return "", err
	}
	toWrite := map[string]string{}
	toDelete := map[string]struct{}{}
	for _, op := range cs.Operations {
		if err := repofile.Apply(tree, op, cs.Provenance, toWrite, toDelete); err != nil {
			return "", err
		}
	}
	changes := make([]repository.RawFileChange, 0, len(toWrite)+len(toDelete))
	for path := range toDelete {
		if _, replaced := toWrite[path]; !replaced {
			changes = append(changes, repository.RawFileChange{Path: path, Remove: true})
		}
	}
	for path, content := range toWrite {
		changes = append(changes, repository.RawFileChange{Path: path, Content: []byte(content)})
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return r.applyRawLocked(repository.RawFileChangeSet{
		TargetRepository: r.repositoryID, TargetRef: cs.TargetRef,
		BaseCommit: cs.BaseCommit, ExpectedTargetCommit: cs.ExpectedTargetCommit,
		Changes: changes, Message: cs.Message, Author: cs.Author,
		RequestID: cs.RequestID, RuleID: cs.RuleID,
	})
}

func (r *DoltRepository) Resolve(objectID kernel.ObjectID, commit kernel.CommitID) (repository.Resolution, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.resolveLocked(objectID, commit)
}

func (r *DoltRepository) resolveLocked(objectID kernel.ObjectID, commit kernel.CommitID) (repository.Resolution, error) {
	tree, err := r.scanAtLocked(commit)
	if err != nil {
		return repository.Resolution{}, err
	}
	units := tree.ObjectUnits(objectID)
	if len(units) > 0 {
		assembled, err := repofile.Assemble(units)
		if err != nil {
			return repository.Resolution{}, err
		}
		schema := ""
		if len(units) == 1 {
			schema = units[0].SchemaRef
		}
		return repository.Resolution{
			Repository: r.repositoryID, Commit: commit, ObjectID: objectID,
			Address:  kernel.Address{Kind: kernel.KindEntity, ObjectID: objectID},
			PathHint: repofile.EntityPathHint(units, objectID), Digest: kernel.CanonicalDigest(assembled),
			DeclarationDigest: repofile.TreeDeclarationDigest(units),
			SchemaRef:         schema, ValueSource: func() *repository.ValueSource {
				if len(units) == 1 {
					return units[0].ValueSource
				}
				return nil
			}(), Status: repository.StatusResolved,
		}, nil
	}
	status := repository.StatusUnresolved
	if r.everExistedLocked(objectID) {
		status = repository.StatusRemoved
	}
	return repository.Resolution{
		Repository: r.repositoryID, Commit: commit, ObjectID: objectID,
		Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: objectID}, Status: status,
	}, nil
}

func (r *DoltRepository) Read(objectID kernel.ObjectID, commit kernel.CommitID) (repository.KnowledgeValue, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.readLocked(objectID, commit)
}

func (r *DoltRepository) readLocked(objectID kernel.ObjectID, commit kernel.CommitID) (repository.KnowledgeValue, error) {
	tree, err := r.scanAtLocked(commit)
	if err != nil {
		return repository.KnowledgeValue{}, err
	}
	units := tree.ObjectUnits(objectID)
	if len(units) == 0 {
		return repository.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "object %s is missing at commit %s", objectID, commit)
	}
	assembled, err := repofile.Assemble(units)
	if err != nil {
		return repository.KnowledgeValue{}, err
	}
	value := repository.KnowledgeValue{
		KnowledgeRef: kernel.KnowledgeRef{Repository: r.repositoryID, Object: objectID},
		Repository:   r.repositoryID, Commit: commit,
		Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: objectID}, Value: assembled,
		Declarations: repofile.Declarations(units),
	}
	if len(units) == 1 {
		value.Provenance = units[0].Provenance
	}
	for _, unit := range units {
		if unit.Address.AspectName != "" {
			for _, member := range units {
				value.Units = append(value.Units, member.Address)
			}
			break
		}
	}
	return value, nil
}

func (r *DoltRepository) ResolveAddress(address kernel.Address, commit kernel.CommitID) (repository.Resolution, error) {
	if err := kernel.AssertWritable(address); err != nil {
		return repository.Resolution{}, err
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	tree, err := r.scanAtLocked(commit)
	if err != nil {
		return repository.Resolution{}, err
	}
	if unit, ok := tree.Units[kernel.AddressKey(address)]; ok {
		hint := unit.PathHint
		if hint == "" {
			hint = unit.Path
		}
		return repository.Resolution{
			Repository: r.repositoryID, Commit: commit, ObjectID: address.ObjectID,
			Address: unit.Address, PathHint: hint, Digest: unit.Digest,
			DeclarationDigest: repository.DeclarationDigest(unit.SchemaRef, unit.ValueSource),
			SchemaRef:         unit.SchemaRef, ValueSource: unit.ValueSource, Status: repository.StatusResolved,
		}, nil
	}
	status := repository.StatusUnresolved
	if r.everExistedLocked(address.ObjectID) {
		status = repository.StatusRemoved
	}
	return repository.Resolution{Repository: r.repositoryID, Commit: commit, ObjectID: address.ObjectID, Address: address, Status: status}, nil
}

func (r *DoltRepository) ReadAddress(address kernel.Address, commit kernel.CommitID) (repository.KnowledgeValue, error) {
	if err := kernel.AssertWritable(address); err != nil {
		return repository.KnowledgeValue{}, err
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	tree, err := r.scanAtLocked(commit)
	if err != nil {
		return repository.KnowledgeValue{}, err
	}
	unit, ok := tree.Units[kernel.AddressKey(address)]
	if !ok {
		return repository.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "address %s is missing at commit %s", kernel.AddressKey(address), commit)
	}
	return repository.KnowledgeValue{
		KnowledgeRef: kernel.KnowledgeRef{Repository: r.repositoryID, Object: address.ObjectID},
		Repository:   r.repositoryID, Commit: commit, Address: unit.Address,
		Value: unit.Value, Provenance: unit.Provenance,
		Declarations: []repository.UnitDeclaration{repofile.DeclarationOf(unit)},
	}, nil
}

func (r *DoltRepository) GetProvenance(objectID kernel.ObjectID, commit kernel.CommitID) (repository.ProvenanceTrace, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	tree, err := r.scanAtLocked(commit)
	if err != nil {
		return repository.ProvenanceTrace{}, err
	}
	units := append([]repofile.Unit(nil), tree.ObjectUnits(objectID)...)
	if len(units) == 0 {
		return repository.ProvenanceTrace{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "object %s is missing at commit %s", objectID, commit)
	}
	sort.Slice(units, func(i, j int) bool { return kernel.AddressKey(units[i].Address) < kernel.AddressKey(units[j].Address) })
	chain := []kernel.ProvenanceEnvelope{}
	for _, unit := range units {
		if unit.Provenance != nil {
			chain = append(chain, *unit.Provenance)
		}
	}
	return repository.ProvenanceTrace{Repository: r.repositoryID, Commit: commit, ObjectID: objectID, Chain: chain}, nil
}

func (r *DoltRepository) Search(query string, commit kernel.CommitID) ([]repository.KnowledgeValue, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	tree, err := r.scanAtLocked(commit)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(query)
	out := []repository.KnowledgeValue{}
	for objectID := range tree.ByObject {
		value, err := r.readLocked(objectID, commit)
		if err != nil {
			return nil, err
		}
		raw, _ := json.Marshal(value.Value)
		if strings.Contains(strings.ToLower(string(raw)), needle) {
			out = append(out, value)
		}
	}
	return out, nil
}

func (r *DoltRepository) List(commit kernel.CommitID) ([]repository.KnowledgeValue, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	tree, err := r.scanAtLocked(commit)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(tree.ByObject))
	for objectID := range tree.ByObject {
		ids = append(ids, string(objectID))
	}
	sort.Strings(ids)
	out := make([]repository.KnowledgeValue, 0, len(ids))
	for _, id := range ids {
		value, err := r.readLocked(kernel.ObjectID(id), commit)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

func (r *DoltRepository) Log(objectID kernel.ObjectID, commit kernel.CommitID, limit int) ([]repository.ObjectRevision, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	if _, err := r.queryHash(string(commit)); err != nil {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	if limit <= 0 {
		limit = 50
	}
	commits, err := r.commitListLocked(string(commit))
	if err != nil {
		return nil, err
	}
	var out []repository.ObjectRevision
	previous := ""
	var introducing *repository.ObjectRevision
	for _, hash := range commits {
		resolution, err := r.resolveLocked(objectID, hash)
		if err != nil {
			return nil, err
		}
		if resolution.Status == repository.StatusUnresolved {
			if introducing != nil {
				out = append(out, *introducing)
			}
			break
		}
		key := string(resolution.Status) + ":" + string(resolution.Digest) + ":" + string(resolution.DeclarationDigest)
		revision := repository.ObjectRevision{Commit: hash, Status: resolution.Status, Digest: resolution.Digest, DeclarationDigest: resolution.DeclarationDigest}
		if key == previous {
			copyRevision := revision
			introducing = &copyRevision
			continue
		}
		if introducing != nil {
			out = append(out, *introducing)
			if len(out) >= limit {
				return out, nil
			}
		}
		previous = key
		copyRevision := revision
		introducing = &copyRevision
	}
	if introducing != nil && len(out) < limit {
		out = append(out, *introducing)
	}
	return out, nil
}

func (r *DoltRepository) Diff(objectID kernel.ObjectID, from, to kernel.CommitID) (repository.ObjectDiff, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	readSide := func(commit kernel.CommitID) (*repository.KnowledgeValue, error) {
		resolution, err := r.resolveLocked(objectID, commit)
		if err != nil {
			return nil, err
		}
		if resolution.Status != repository.StatusResolved {
			return nil, nil
		}
		value, err := r.readLocked(objectID, commit)
		return &value, err
	}
	left, err := readSide(from)
	if err != nil {
		return repository.ObjectDiff{}, err
	}
	right, err := readSide(to)
	if err != nil {
		return repository.ObjectDiff{}, err
	}
	return repository.ObjectDiff{ObjectID: objectID, FromCommit: from, ToCommit: to, From: left, To: right}, nil
}

func (r *DoltRepository) commitListLocked(ref string) ([]kernel.CommitID, error) {
	out, err := r.run("log", "--oneline", ref)
	if err != nil {
		return nil, err
	}
	var commits []kernel.CommitID
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) > 0 {
			commits = append(commits, kernel.CommitID(fields[0]))
		}
	}
	return commits, nil
}

func (r *DoltRepository) everExistedLocked(objectID kernel.ObjectID) bool {
	commits, err := r.commitListLocked("--all")
	if err != nil {
		return false
	}
	for _, commit := range commits {
		tree, err := r.scanAtLocked(commit)
		if err == nil && len(tree.ObjectUnits(objectID)) > 0 {
			return true
		}
	}
	return false
}
