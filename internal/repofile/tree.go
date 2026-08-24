// Package repofile is the on-disk knowledge unit format used by local FileGit.
// It is not a store.
package repofile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"kc/kernel"
	"kc/repository"
)

var knowledgeFile = regexp.MustCompile(`(?i)\.(json|md|ya?ml|txt)$`)

// KnowledgePath is true when a tree path may hold a knowledge unit.
func KnowledgePath(rel string) bool {
	return knowledgeFile.MatchString(rel)
}

// Unit is one aspect/entity file in a snapshot tree.
type Unit struct {
	ObjectID   kernel.ObjectID            `json:"objectId"`
	Address    kernel.Address             `json:"address"`
	PathHint   string                     `json:"pathHint"`
	SchemaRef  string                     `json:"schemaRef,omitempty"`
	Provenance *kernel.ProvenanceEnvelope `json:"provenance,omitempty"`
	Value      any                        `json:"value"`
	Path       string                     `json:"path,omitempty"`
	Digest     kernel.Digest              `json:"digest,omitempty"`
}

// Tree is the assembled snapshot of units at one commit.
type Tree struct {
	Units    map[string]Unit
	ByObject map[kernel.ObjectID][]Unit
}

func NewTree() *Tree {
	return &Tree{
		Units:    map[string]Unit{},
		ByObject: map[kernel.ObjectID][]Unit{},
	}
}

func (idx *Tree) Upsert(unit Unit) {
	key := kernel.AddressKey(unit.Address)
	prev, had := idx.Units[key]
	idx.Units[key] = unit
	list := idx.ByObject[unit.Address.ObjectID]
	if had {
		next := list[:0]
		for _, u := range list {
			if kernel.AddressKey(u.Address) != key {
				next = append(next, u)
			}
		}
		list = append(next, unit)
		_ = prev
	} else {
		list = append(list, unit)
	}
	idx.ByObject[unit.Address.ObjectID] = list
}

func (idx *Tree) Remove(address kernel.Address) *Unit {
	key := kernel.AddressKey(address)
	prev, ok := idx.Units[key]
	if !ok {
		return nil
	}
	delete(idx.Units, key)
	list := idx.ByObject[address.ObjectID]
	next := list[:0]
	for _, u := range list {
		if kernel.AddressKey(u.Address) != key {
			next = append(next, u)
		}
	}
	if len(next) == 0 {
		delete(idx.ByObject, address.ObjectID)
	} else {
		idx.ByObject[address.ObjectID] = next
	}
	return &prev
}

func (idx *Tree) ObjectUnits(objectID kernel.ObjectID) []Unit {
	return idx.ByObject[objectID]
}

func Serialize(address kernel.Address, pathHint, schemaRef string, provenance *kernel.ProvenanceEnvelope, value any) (string, error) {
	fm := []string{"object_id: " + string(address.ObjectID)}
	if address.AspectName != "" {
		fm = append(fm, "aspect_name: "+address.AspectName)
	}
	if address.MemberKey != "" {
		fm = append(fm, "member_key: "+address.MemberKey)
	}
	if address.Kind != kernel.KindEntity {
		fm = append(fm, "kind: "+string(address.Kind))
	}
	if pathHint != "" {
		fm = append(fm, "path_hint: "+pathHint)
	}
	if schemaRef != "" {
		fm = append(fm, "schema_ref: "+schemaRef)
	}
	if provenance != nil {
		b, err := json.Marshal(provenance)
		if err != nil {
			return "", err
		}
		fm = append(fm, "provenance: "+string(b))
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return "---\n" + strings.Join(fm, "\n") + "\n---\n" + string(body) + "\n", nil
}

func Parse(content string) *Unit {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return nil
	}
	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			endIdx = i
			break
		}
	}
	if endIdx < 0 {
		return nil
	}
	obj := map[string]string{}
	var provenance *kernel.ProvenanceEnvelope
	for _, line := range lines[1:endIdx] {
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if key == "provenance" && value != "" {
			var p kernel.ProvenanceEnvelope
			if json.Unmarshal([]byte(value), &p) == nil {
				provenance = &p
			}
			continue
		}
		obj[key] = value
	}
	objectID := obj["object_id"]
	if objectID == "" {
		return nil
	}
	body := strings.TrimSpace(strings.Join(lines[endIdx+1:], "\n"))
	var value any
	if json.Unmarshal([]byte(body), &value) != nil {
		value = body
	}
	return &Unit{
		ObjectID:   kernel.ObjectID(objectID),
		Address:    kernel.InferAddress(kernel.ObjectID(objectID), obj["aspect_name"], obj["member_key"], obj["kind"]),
		PathHint:   obj["path_hint"],
		SchemaRef:  obj["schema_ref"],
		Provenance: provenance,
		Value:      value,
	}
}

// Ingest adds one parsed unit at relPath. Duplicate Address or blob/aspect mix fails.
func Ingest(idx *Tree, parsed *Unit, relPath string) error {
	if parsed == nil {
		return nil
	}
	key := kernel.AddressKey(parsed.Address)
	if _, ok := idx.Units[key]; ok {
		return kernel.Fail(kernel.ErrObjectIDConflict, "duplicate address %s", key)
	}
	siblings := idx.ObjectUnits(parsed.ObjectID)
	incomingBlob := kernel.IsEntityBlob(parsed.Address)
	siblingBlob, siblingAspect := false, false
	for _, u := range siblings {
		if kernel.IsEntityBlob(u.Address) {
			siblingBlob = true
		} else {
			siblingAspect = true
		}
	}
	if (incomingBlob && siblingAspect) || (!incomingBlob && siblingBlob) {
		return kernel.Fail(kernel.ErrObjectIDConflict, "%s mixes entity blob and aspects", parsed.ObjectID)
	}
	parsed.Path = relPath
	parsed.Digest = kernel.CanonicalDigest(parsed.Value)
	idx.Upsert(*parsed)
	return nil
}

func DefaultPath(address kernel.Address) string {
	if address.MemberKey != "" && address.AspectName != "" {
		return "objects/" + string(address.ObjectID) + "/" + address.AspectName + "/" + address.MemberKey + ".json"
	}
	if address.AspectName != "" {
		return "objects/" + string(address.ObjectID) + "/" + address.AspectName + ".json"
	}
	return "objects/" + string(address.ObjectID) + ".json"
}

func SafeRelativePath(value string) (string, error) {
	if value == "" || filepath.IsAbs(value) {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "path must be relative: %s", value)
	}
	normalized := filepath.Clean(value)
	if normalized == ".." || strings.HasPrefix(normalized, ".."+string(os.PathSeparator)) {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "path escapes repository root: %s", value)
	}
	return normalized, nil
}

func Assemble(units []Unit) (any, error) {
	if len(units) == 0 {
		return nil, nil
	}
	var blobs, parts []Unit
	for _, u := range units {
		if kernel.IsEntityBlob(u.Address) {
			blobs = append(blobs, u)
		} else {
			parts = append(parts, u)
		}
	}
	if len(blobs) > 0 && len(parts) > 0 {
		return nil, kernel.Fail(kernel.ErrObjectIDConflict, "%s mixes entity blob and aspects", units[0].ObjectID)
	}
	if len(blobs) > 1 {
		return nil, kernel.Fail(kernel.ErrObjectIDConflict, "duplicate object_id %s", blobs[0].ObjectID)
	}
	if len(blobs) == 1 {
		return blobs[0].Value, nil
	}
	recordNames := map[string]struct{}{}
	memberNames := map[string]struct{}{}
	out := map[string]any{}
	members := map[string]map[string]any{}
	for _, unit := range parts {
		name := unit.Address.AspectName
		if name == "" {
			continue
		}
		if unit.Address.MemberKey != "" {
			memberNames[name] = struct{}{}
			bucket := members[name]
			if bucket == nil {
				bucket = map[string]any{}
				members[name] = bucket
			}
			bucket[unit.Address.MemberKey] = unit.Value
		} else {
			recordNames[name] = struct{}{}
			out[name] = unit.Value
		}
	}
	for name := range memberNames {
		if _, ok := recordNames[name]; ok {
			return nil, kernel.Fail(kernel.ErrObjectIDConflict, "aspect %s is both Record and Member", name)
		}
		out[name] = members[name]
	}
	return out, nil
}

func EntityPathHint(units []Unit, objectID kernel.ObjectID) string {
	if len(units) == 1 {
		if units[0].PathHint != "" {
			return units[0].PathHint
		}
		return units[0].Path
	}
	if len(units) == 0 {
		return ""
	}
	return "objects/" + string(objectID)
}

func AssertLayout(units []Unit, incoming kernel.Address) error {
	if len(units) == 0 {
		return nil
	}
	hasBlob, hasAspect := false, false
	for _, u := range units {
		if kernel.IsEntityBlob(u.Address) {
			hasBlob = true
		} else {
			hasAspect = true
		}
	}
	if hasBlob && hasAspect {
		return kernel.Fail(kernel.ErrObjectIDConflict, "%s mixes entity blob and aspects", incoming.ObjectID)
	}
	if kernel.IsEntityBlob(incoming) && hasAspect {
		return kernel.Fail(kernel.ErrObjectIDConflict, "cannot PUT an entity blob on %s; object already has aspects", incoming.ObjectID)
	}
	if !kernel.IsEntityBlob(incoming) && hasBlob {
		return kernel.Fail(kernel.ErrObjectIDConflict, "cannot PUT an aspect on %s; object is an entity blob", incoming.ObjectID)
	}
	return nil
}

func TreeDigest(units []Unit) kernel.Digest {
	rows := make([]any, 0, len(units))
	for _, u := range units {
		rows = append(rows, map[string]any{
			"k": kernel.AddressKey(u.Address),
			"d": string(u.Digest),
		})
	}
	return kernel.CanonicalDigest(rows)
}

// Apply mutates the tree for one PUT/REMOVE. toWrite/toDelete collect path contents.
func Apply(idx *Tree, op repository.Operation, prov *kernel.ProvenanceEnvelope, toWrite map[string]string, toDelete map[string]struct{}) error {
	if err := kernel.AssertWritable(op.Address); err != nil {
		return err
	}
	if op.Op == repository.OpPut {
		siblings := idx.ObjectUnits(op.Address.ObjectID)
		existing, has := idx.Units[kernel.AddressKey(op.Address)]
		if err := AssertLayout(siblings, op.Address); err != nil {
			return err
		}
		if op.Precondition != nil {
			if op.Precondition.Type == repository.IfAbsent && has {
				return kernel.Fail(kernel.ErrPreconditionFailed, "address %s already exists", kernel.AddressKey(op.Address))
			}
			if (op.Precondition.Type == repository.IfObjectEquals || op.Precondition.Type == repository.IfDigestEquals) && op.Precondition.Digest != "" {
				if !has || existing.Digest != op.Precondition.Digest {
					actual := "missing"
					if has {
						actual = string(existing.Digest)
					}
					return kernel.Fail(kernel.ErrPreconditionFailed, "digest mismatch for %s: expected %s, actual %s", kernel.AddressKey(op.Address), op.Precondition.Digest, actual)
				}
			}
		}
		pathHint := op.PathHint
		if pathHint == "" && has {
			pathHint = existing.Path
		}
		if pathHint == "" {
			pathHint = DefaultPath(op.Address)
		}
		newPath, err := SafeRelativePath(pathHint)
		if err != nil {
			return err
		}
		if !KnowledgePath(newPath) {
			return kernel.Fail(kernel.ErrUsageInvalid, "path must use a readable knowledge file extension (.json, .md, .yaml, .yml, .txt): %s", newPath)
		}
		if has && existing.Path != newPath {
			toDelete[existing.Path] = struct{}{}
			delete(toWrite, existing.Path)
		}
		storedHint := existing.PathHint
		if op.PathHint != "" {
			storedHint = newPath
		}
		schema := op.SchemaRef
		if schema == "" && has {
			schema = existing.SchemaRef
		}
		envelope := prov
		if envelope == nil && has {
			envelope = existing.Provenance
		}
		unit := Unit{
			ObjectID:   op.Address.ObjectID,
			Address:    op.Address,
			PathHint:   storedHint,
			SchemaRef:  schema,
			Provenance: envelope,
			Value:      op.Value,
			Path:       newPath,
			Digest:     kernel.CanonicalDigest(op.Value),
		}
		content, err := Serialize(op.Address, unit.PathHint, unit.SchemaRef, unit.Provenance, op.Value)
		if err != nil {
			return err
		}
		toWrite[newPath] = content
		idx.Upsert(unit)
		return nil
	}

	if kernel.IsEntityBlob(op.Address) {
		units := idx.ObjectUnits(op.Address.ObjectID)
		if len(units) == 0 {
			return kernel.Fail(kernel.ErrPreconditionFailed, "object %s does not exist", op.Address.ObjectID)
		}
		if op.Precondition != nil && (op.Precondition.Type == repository.IfObjectEquals || op.Precondition.Type == repository.IfDigestEquals) && op.Precondition.Digest != "" {
			assembled, err := Assemble(units)
			if err != nil {
				return err
			}
			if kernel.CanonicalDigest(assembled) != op.Precondition.Digest {
				return kernel.Fail(kernel.ErrPreconditionFailed, "digest mismatch for %s: expected %s, actual %s", op.Address.ObjectID, op.Precondition.Digest, kernel.CanonicalDigest(assembled))
			}
		}
		for _, unit := range units {
			toDelete[unit.Path] = struct{}{}
			delete(toWrite, unit.Path)
			idx.Remove(unit.Address)
		}
		return nil
	}

	existing, has := idx.Units[kernel.AddressKey(op.Address)]
	if !has {
		return kernel.Fail(kernel.ErrPreconditionFailed, "address %s does not exist", kernel.AddressKey(op.Address))
	}
	if op.Precondition != nil && (op.Precondition.Type == repository.IfObjectEquals || op.Precondition.Type == repository.IfDigestEquals) && op.Precondition.Digest != existing.Digest {
		return kernel.Fail(kernel.ErrPreconditionFailed, "digest mismatch for %s: expected %s, actual %s", kernel.AddressKey(op.Address), op.Precondition.Digest, existing.Digest)
	}
	toDelete[existing.Path] = struct{}{}
	delete(toWrite, existing.Path)
	idx.Remove(op.Address)
	return nil
}
