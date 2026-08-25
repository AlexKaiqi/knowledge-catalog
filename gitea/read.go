package gitea

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/repository"
)

func (r *Repository) scanAt(commitID kernel.CommitID) (*repofile.Tree, map[string]string, error) {
	r.mu.Lock()
	if idx, ok := r.scan[commitID]; ok {
		blobs := r.blobs[commitID]
		r.mu.Unlock()
		return idx, blobs, nil
	}
	r.mu.Unlock()

	idx := repofile.NewTree()
	blobs := map[string]string{}
	page := 1
	for {
		q := "?recursive=true&page=" + strconv.Itoa(page) + "&per_page=1000"
		var tree gitTree
		status, _, err := r.cli.do(http.MethodGet, r.ep.repoPath("git/trees/"+url.PathEscape(string(commitID))+q), nil, &tree)
		if missingCommit(status, err) {
			return nil, nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commitID)
		}
		if err != nil {
			return nil, nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "gitea tree at %s: %v", commitID, err)
		}
		for _, e := range tree.Tree {
			if e.Type != "blob" {
				continue
			}
			blobs[e.Path] = e.SHA
			if !repofile.KnowledgePath(e.Path) {
				continue
			}
			content, err := r.readBlob(e.SHA)
			if err != nil {
				return nil, nil, err
			}
			parsed := repofile.Parse(content)
			if parsed == nil {
				continue
			}
			if err := repofile.Ingest(idx, parsed, e.Path); err != nil {
				return nil, nil, err
			}
		}
		if !tree.Truncated {
			break
		}
		page++
	}
	r.mu.Lock()
	r.scan[commitID] = idx
	r.blobs[commitID] = blobs
	r.mu.Unlock()
	return idx, blobs, nil
}

func (r *Repository) readBlob(sha string) (string, error) {
	var blob gitBlob
	if _, _, err := r.cli.do(http.MethodGet, r.ep.repoPath("git/blobs/"+url.PathEscape(sha)), nil, &blob); err != nil {
		return "", kernel.Fail(kernel.ErrTemporaryUnavailable, "gitea blob %s: %v", sha, err)
	}
	if strings.EqualFold(blob.Encoding, "base64") {
		b, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(blob.Content), ""))
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	return blob.Content, nil
}

func (r *Repository) Resolve(objectID kernel.ObjectID, commitID kernel.CommitID) (repository.Resolution, error) {
	idx, _, err := r.scanAt(commitID)
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
			Repository: r.id, Commit: commitID, ObjectID: objectID,
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
	if r.everExisted(objectID) {
		status = repository.StatusRemoved
	}
	return repository.Resolution{
		Repository: r.id, Commit: commitID, ObjectID: objectID,
		Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: objectID}, Status: status,
	}, nil
}

func (r *Repository) Read(objectID kernel.ObjectID, commitID kernel.CommitID) (repository.KnowledgeValue, error) {
	idx, _, err := r.scanAt(commitID)
	if err != nil {
		return repository.KnowledgeValue{}, err
	}
	units := idx.ObjectUnits(objectID)
	if len(units) == 0 {
		return repository.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "object %s is missing at commit %s", objectID, commitID)
	}
	assembled, err := repofile.Assemble(units)
	if err != nil {
		return repository.KnowledgeValue{}, err
	}
	kv := repository.KnowledgeValue{
		KnowledgeRef: kernel.KnowledgeRef{Repository: r.id, Object: objectID},
		Repository:   r.id, Commit: commitID,
		Address:      kernel.Address{Kind: kernel.KindEntity, ObjectID: objectID},
		Value:        assembled,
		Declarations: repofile.Declarations(units),
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

func (r *Repository) ResolveAddress(address kernel.Address, commitID kernel.CommitID) (repository.Resolution, error) {
	if err := kernel.AssertWritable(address); err != nil {
		return repository.Resolution{}, err
	}
	idx, _, err := r.scanAt(commitID)
	if err != nil {
		return repository.Resolution{}, err
	}
	if unit, ok := idx.Units[kernel.AddressKey(address)]; ok {
		hint := unit.PathHint
		if hint == "" {
			hint = unit.Path
		}
		return repository.Resolution{
			Repository: r.id, Commit: commitID, ObjectID: address.ObjectID,
			Address: unit.Address, PathHint: hint, Digest: unit.Digest,
			DeclarationDigest: repository.DeclarationDigest(unit.SchemaRef, unit.ValueSource),
			SchemaRef:         unit.SchemaRef, ValueSource: unit.ValueSource, Status: repository.StatusResolved,
		}, nil
	}
	status := repository.StatusUnresolved
	if r.everExisted(address.ObjectID) {
		status = repository.StatusRemoved
	}
	return repository.Resolution{Repository: r.id, Commit: commitID, ObjectID: address.ObjectID, Address: address, Status: status}, nil
}

func (r *Repository) ReadAddress(address kernel.Address, commitID kernel.CommitID) (repository.KnowledgeValue, error) {
	if err := kernel.AssertWritable(address); err != nil {
		return repository.KnowledgeValue{}, err
	}
	idx, _, err := r.scanAt(commitID)
	if err != nil {
		return repository.KnowledgeValue{}, err
	}
	unit, ok := idx.Units[kernel.AddressKey(address)]
	if !ok {
		return repository.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "address %s is missing at commit %s", kernel.AddressKey(address), commitID)
	}
	return repository.KnowledgeValue{
		KnowledgeRef: kernel.KnowledgeRef{Repository: r.id, Object: address.ObjectID},
		Repository:   r.id, Commit: commitID, Address: unit.Address, Value: unit.Value, Provenance: unit.Provenance,
		Declarations: []repository.UnitDeclaration{repofile.DeclarationOf(unit)},
	}, nil
}

func (r *Repository) GetProvenance(objectID kernel.ObjectID, commitID kernel.CommitID) (repository.ProvenanceTrace, error) {
	idx, _, err := r.scanAt(commitID)
	if err != nil {
		return repository.ProvenanceTrace{}, err
	}
	units := idx.ObjectUnits(objectID)
	if len(units) == 0 {
		return repository.ProvenanceTrace{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "object %s is missing at commit %s", objectID, commitID)
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
	return repository.ProvenanceTrace{Repository: r.id, Commit: commitID, ObjectID: objectID, Chain: chain}, nil
}

func (r *Repository) Search(query string, commitID kernel.CommitID) ([]repository.KnowledgeValue, error) {
	idx, _, err := r.scanAt(commitID)
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

func (r *Repository) List(commitID kernel.CommitID) ([]repository.KnowledgeValue, error) {
	idx, _, err := r.scanAt(commitID)
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

func (r *Repository) everExisted(objectID kernel.ObjectID) bool {
	paths := []string{
		"objects/" + string(objectID) + ".json",
		"objects/" + string(objectID),
	}
	for _, p := range paths {
		var rows []commitRow
		q := "?limit=1&path=" + url.QueryEscape(p)
		status, _, err := r.cli.do(http.MethodGet, r.ep.repoPath("commits"+q), nil, &rows)
		if err == nil && status == http.StatusOK && len(rows) > 0 {
			return true
		}
	}
	return false
}
