package local

import (
	"encoding/json"
	"strings"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/repository"
)

func (r *FileGitRepository) Resolve(objectID kernel.ObjectID, commitID kernel.CommitID) (repository.Resolution, error) {
	idx, err := r.scanAt(commitID)
	if err != nil {
		return repository.Resolution{}, err
	}
	units := idx.ObjectUnits(objectID)
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
			Repository: r.repositoryID,
			Commit:     commitID,
			ObjectID:   objectID,
			Address:    kernel.Address{Kind: kernel.KindEntity, ObjectID: objectID},
			PathHint:   repofile.EntityPathHint(units, objectID),
			Digest:     kernel.CanonicalDigest(assembled),
			SchemaRef:  schema,
			Status:     repository.StatusResolved,
		}, nil
	}
	status := repository.StatusUnresolved
	if r.everExisted(objectID) {
		status = repository.StatusRemoved
	}
	return repository.Resolution{
		Repository: r.repositoryID,
		Commit:     commitID,
		ObjectID:   objectID,
		Address:    kernel.Address{Kind: kernel.KindEntity, ObjectID: objectID},
		Status:     status,
	}, nil
}

func (r *FileGitRepository) Read(objectID kernel.ObjectID, commitID kernel.CommitID) (repository.KnowledgeValue, error) {
	idx, err := r.scanAt(commitID)
	if err != nil {
		return repository.KnowledgeValue{}, err
	}
	units := idx.ObjectUnits(objectID)
	if len(units) == 0 {
		return repository.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "%s not resolvable at %s", objectID, commitID)
	}
	assembled, err := repofile.Assemble(units)
	if err != nil {
		return repository.KnowledgeValue{}, err
	}
	kv := repository.KnowledgeValue{
		KnowledgeRef: kernel.KnowledgeRef{Repository: r.repositoryID, Object: objectID},
		Repository:   r.repositoryID,
		Commit:       commitID,
		Address:      kernel.Address{Kind: kernel.KindEntity, ObjectID: objectID},
		Value:        assembled,
	}
	if len(units) == 1 {
		kv.Provenance = units[0].Provenance
	}
	for _, u := range units {
		if u.Address.AspectName != "" {
			for _, unit := range units {
				kv.Units = append(kv.Units, unit.Address)
			}
			break
		}
	}
	return kv, nil
}

func (r *FileGitRepository) ResolveAddress(address kernel.Address, commitID kernel.CommitID) (repository.Resolution, error) {
	if err := kernel.AssertWritable(address); err != nil {
		return repository.Resolution{}, err
	}
	idx, err := r.scanAt(commitID)
	if err != nil {
		return repository.Resolution{}, err
	}
	if unit, ok := idx.Units[kernel.AddressKey(address)]; ok {
		hint := unit.PathHint
		if hint == "" {
			hint = unit.Path
		}
		return repository.Resolution{
			Repository: r.repositoryID, Commit: commitID, ObjectID: address.ObjectID,
			Address: unit.Address, PathHint: hint, Digest: unit.Digest, SchemaRef: unit.SchemaRef, Status: repository.StatusResolved,
		}, nil
	}
	status := repository.StatusUnresolved
	if r.everExisted(address.ObjectID) {
		status = repository.StatusRemoved
	}
	return repository.Resolution{Repository: r.repositoryID, Commit: commitID, ObjectID: address.ObjectID, Address: address, Status: status}, nil
}

func (r *FileGitRepository) ReadAddress(address kernel.Address, commitID kernel.CommitID) (repository.KnowledgeValue, error) {
	if err := kernel.AssertWritable(address); err != nil {
		return repository.KnowledgeValue{}, err
	}
	idx, err := r.scanAt(commitID)
	if err != nil {
		return repository.KnowledgeValue{}, err
	}
	unit, ok := idx.Units[kernel.AddressKey(address)]
	if !ok {
		return repository.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "%s not resolvable at %s", kernel.AddressKey(address), commitID)
	}
	return repository.KnowledgeValue{
		KnowledgeRef: kernel.KnowledgeRef{Repository: r.repositoryID, Object: address.ObjectID},
		Repository:   r.repositoryID,
		Commit:       commitID,
		Address:      unit.Address,
		Value:        unit.Value,
		Provenance:   unit.Provenance,
	}, nil
}

func (r *FileGitRepository) GetProvenance(objectID kernel.ObjectID, commitID kernel.CommitID) (repository.ProvenanceTrace, error) {
	idx, err := r.scanAt(commitID)
	if err != nil {
		return repository.ProvenanceTrace{}, err
	}
	units := idx.ObjectUnits(objectID)
	if len(units) == 0 {
		return repository.ProvenanceTrace{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "%s not resolvable at %s", objectID, commitID)
	}
	sorted := append([]repofile.Unit{}, units...)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if kernel.AddressKey(sorted[j].Address) < kernel.AddressKey(sorted[i].Address) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	chain := []kernel.ProvenanceEnvelope{}
	for _, u := range sorted {
		if u.Provenance != nil {
			chain = append(chain, *u.Provenance)
		}
	}
	return repository.ProvenanceTrace{Repository: r.repositoryID, Commit: commitID, ObjectID: objectID, Chain: chain}, nil
}

func (r *FileGitRepository) Search(query string, commitID kernel.CommitID) ([]repository.KnowledgeValue, error) {
	idx, err := r.scanAt(commitID)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(query)
	var out []repository.KnowledgeValue
	for objectID := range idx.ByObject {
		value, err := r.Read(objectID, commitID)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(value.Value)
		if strings.Contains(strings.ToLower(string(b)), needle) {
			out = append(out, value)
		}
	}
	return out, nil
}

func (r *FileGitRepository) List(commitID kernel.CommitID) ([]repository.KnowledgeValue, error) {
	idx, err := r.scanAt(commitID)
	if err != nil {
		return nil, err
	}
	var out []repository.KnowledgeValue
	for objectID := range idx.ByObject {
		value, err := r.Read(objectID, commitID)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}
