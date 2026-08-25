package gitea

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
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

func (r *Repository) Resolve(objectID knowledge.ObjectID, commitID kernel.CommitID) (knowledge.Resolution, error) {
	idx, _, err := r.scanAt(commitID)
	if err != nil {
		return knowledge.Resolution{}, err
	}
	units := idx.ObjectUnits(objectID)
	if len(units) > 0 {
		assembled, err := repofile.Assemble(units)
		if err != nil {
			return knowledge.Resolution{}, err
		}
		schema := ""
		if len(units) == 1 {
			schema = units[0].SchemaRef
		}
		return knowledge.Resolution{
			Repository: r.id, Commit: commitID, ObjectID: objectID,
			Address:  knowledge.Address{Kind: knowledge.KindEntity, ObjectID: objectID},
			PathHint: repofile.EntityPathHint(units, objectID), Digest: kernel.CanonicalDigest(assembled),
			DeclarationDigest: repofile.TreeDeclarationDigest(units),
			SchemaRef:         schema, ValueSource: func() *knowledge.ValueSource {
				if len(units) == 1 {
					return units[0].ValueSource
				}
				return nil
			}(), Status: knowledge.StatusResolved,
		}, nil
	}
	status := knowledge.StatusUnresolved
	if r.everExisted(objectID) {
		status = knowledge.StatusRemoved
	}
	return knowledge.Resolution{
		Repository: r.id, Commit: commitID, ObjectID: objectID,
		Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: objectID}, Status: status,
	}, nil
}

func (r *Repository) Read(objectID knowledge.ObjectID, commitID kernel.CommitID) (knowledge.KnowledgeValue, error) {
	idx, _, err := r.scanAt(commitID)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	units := idx.ObjectUnits(objectID)
	if len(units) == 0 {
		return knowledge.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "object %s is missing at commit %s", objectID, commitID)
	}
	assembled, err := repofile.Assemble(units)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	kv := knowledge.KnowledgeValue{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: r.id, Object: objectID},
		Repository:   r.id, Commit: commitID,
		Address:      knowledge.Address{Kind: knowledge.KindEntity, ObjectID: objectID},
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

func (r *Repository) ResolveAddress(address knowledge.Address, commitID kernel.CommitID) (knowledge.Resolution, error) {
	if err := knowledge.AssertWritable(address); err != nil {
		return knowledge.Resolution{}, err
	}
	idx, _, err := r.scanAt(commitID)
	if err != nil {
		return knowledge.Resolution{}, err
	}
	if unit, ok := idx.Units[knowledge.AddressKey(address)]; ok {
		hint := unit.PathHint
		if hint == "" {
			hint = unit.Path
		}
		return knowledge.Resolution{
			Repository: r.id, Commit: commitID, ObjectID: address.ObjectID,
			Address: unit.Address, PathHint: hint, Digest: unit.Digest,
			DeclarationDigest: knowledge.DeclarationDigest(unit.SchemaRef, unit.ValueSource),
			SchemaRef:         unit.SchemaRef, ValueSource: unit.ValueSource, Status: knowledge.StatusResolved,
		}, nil
	}
	status := knowledge.StatusUnresolved
	if r.everExisted(address.ObjectID) {
		status = knowledge.StatusRemoved
	}
	return knowledge.Resolution{Repository: r.id, Commit: commitID, ObjectID: address.ObjectID, Address: address, Status: status}, nil
}

func (r *Repository) ReadAddress(address knowledge.Address, commitID kernel.CommitID) (knowledge.KnowledgeValue, error) {
	if err := knowledge.AssertWritable(address); err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	idx, _, err := r.scanAt(commitID)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	unit, ok := idx.Units[knowledge.AddressKey(address)]
	if !ok {
		return knowledge.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "address %s is missing at commit %s", knowledge.AddressKey(address), commitID)
	}
	return knowledge.KnowledgeValue{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: r.id, Object: address.ObjectID},
		Repository:   r.id, Commit: commitID, Address: unit.Address, Value: unit.Value, Provenance: unit.Provenance,
		Declarations: []knowledge.UnitDeclaration{repofile.DeclarationOf(unit)},
	}, nil
}

func (r *Repository) GetProvenance(objectID knowledge.ObjectID, commitID kernel.CommitID) (knowledge.ProvenanceTrace, error) {
	idx, _, err := r.scanAt(commitID)
	if err != nil {
		return knowledge.ProvenanceTrace{}, err
	}
	units := idx.ObjectUnits(objectID)
	if len(units) == 0 {
		return knowledge.ProvenanceTrace{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "object %s is missing at commit %s", objectID, commitID)
	}
	sorted := append([]repofile.Unit{}, units...)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if knowledge.AddressKey(sorted[j].Address) < knowledge.AddressKey(sorted[i].Address) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	chain := []knowledge.ProvenanceEnvelope{}
	for _, u := range sorted {
		if u.Provenance != nil {
			chain = append(chain, *u.Provenance)
		}
	}
	return knowledge.ProvenanceTrace{Repository: r.id, Commit: commitID, ObjectID: objectID, Chain: chain}, nil
}

func (r *Repository) List(commitID kernel.CommitID) ([]knowledge.KnowledgeValue, error) {
	idx, _, err := r.scanAt(commitID)
	if err != nil {
		return nil, err
	}
	var out []knowledge.KnowledgeValue
	for objectID := range idx.ByObject {
		value, err := r.Read(objectID, commitID)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

func (r *Repository) everExisted(objectID knowledge.ObjectID) bool {
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
