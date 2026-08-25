package writer

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
	snapshotpkg "kc/snapshot"
)

// Ingest / Reconcile preview a ChangeSet. They are not a Surface and not a
// collector: they do not write. Confirm with Commit. The function is still
// named Ingest to match `kc ingest`; this file is not called ingestion.

type IngestPreview struct {
	ChangeSet knowledge.CommitChangeSet `json:"changeSet"`
	Files     []IngestFile              `json:"files"`
}

type IngestFile struct {
	Path     string             `json:"path"`
	ObjectID knowledge.ObjectID `json:"objectId"`
	Address  knowledge.Address  `json:"address"`
}

func Ingest(dir string, repositoryID kernel.RepositoryID, baseCommit kernel.CommitID) (IngestPreview, error) {
	var operations []knowledge.Operation
	var files []IngestFile
	err := filepath.WalkDir(dir, func(full string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() && full != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, full)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		content, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		op, file, err := ingestFile(rel, content)
		if err != nil {
			return err
		}
		operations = append(operations, op)
		files = append(files, file)
		return nil
	})
	if err != nil {
		return IngestPreview{}, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	sort.Slice(operations, func(i, j int) bool {
		return knowledge.AddressKey(operations[i].Address) < knowledge.AddressKey(operations[j].Address)
	})
	return IngestPreview{
		Files: files,
		ChangeSet: knowledge.CommitChangeSet{
			TargetRepository:     repositoryID,
			TargetRef:            snapshotpkg.DefaultRef,
			BaseCommit:           baseCommit,
			ExpectedTargetCommit: baseCommit,
			Operations:           operations,
			Message:              "ingest " + dir,
			Provenance:           &knowledge.ProvenanceEnvelope{OriginKind: knowledge.OriginSource, SourceRefs: []string{dir}},
		},
	}, nil
}

func ingestFile(rel string, content []byte) (knowledge.Operation, IngestFile, error) {
	if unit := repofile.Parse(string(content)); unit != nil {
		pathHint := unit.PathHint
		if pathHint == "" {
			pathHint = rel
		}
		return knowledge.Operation{
				Op:        knowledge.OpPut,
				Address:   unit.Address,
				Value:     unit.Value,
				PathHint:  pathHint,
				SchemaRef: unit.SchemaRef,
			}, IngestFile{
				Path:     rel,
				ObjectID: unit.Address.ObjectID,
				Address:  unit.Address,
			}, nil
	}
	objectID := rel
	for _, ext := range []string{".md", ".json", ".yaml", ".yml", ".txt"} {
		if strings.HasSuffix(strings.ToLower(objectID), ext) {
			objectID = objectID[:len(objectID)-len(ext)]
			break
		}
	}
	address := knowledge.Address{Kind: knowledge.KindEntity, ObjectID: knowledge.ObjectID(objectID)}
	value := any(string(content))
	if strings.HasSuffix(strings.ToLower(rel), ".json") {
		var parsed any
		if json.Unmarshal(content, &parsed) == nil {
			value = parsed
		}
	}
	return knowledge.Operation{
			Op:       knowledge.OpPut,
			Address:  address,
			Value:    value,
			PathHint: rel,
		}, IngestFile{
			Path:     rel,
			ObjectID: address.ObjectID,
			Address:  address,
		}, nil
}

type ReconcilePreview struct {
	ChangeSet knowledge.CommitChangeSet `json:"changeSet"`
	Summary   ReconcileSummary          `json:"summary"`
}

type ReconcileSummary struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Removed int `json:"removed"`
}

func Reconcile(snapshot map[knowledge.ObjectID]any, current map[knowledge.ObjectID]string, repositoryID kernel.RepositoryID, baseCommit kernel.CommitID) ReconcilePreview {
	var operations []knowledge.Operation
	added, updated, removed := 0, 0, 0
	for objectID, value := range snapshot {
		digest := string(kernel.CanonicalDigest(value))
		existing, ok := current[objectID]
		if !ok {
			operations = append(operations, knowledge.Operation{
				Op:           knowledge.OpPut,
				Address:      knowledge.Address{Kind: knowledge.KindEntity, ObjectID: objectID},
				Value:        value,
				Precondition: &knowledge.Precondition{Type: knowledge.IfAbsent},
			})
			added++
		} else if existing != digest {
			operations = append(operations, knowledge.Operation{
				Op:           knowledge.OpPut,
				Address:      knowledge.Address{Kind: knowledge.KindEntity, ObjectID: objectID},
				Value:        value,
				Precondition: &knowledge.Precondition{Type: knowledge.IfDigestEquals, Digest: kernel.Digest(existing)},
			})
			updated++
		}
	}
	for objectID := range current {
		if _, ok := snapshot[objectID]; !ok {
			operations = append(operations, knowledge.Operation{
				Op:      knowledge.OpRemove,
				Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: objectID},
				Reason:  "absent-from-snapshot",
			})
			removed++
		}
	}
	return ReconcilePreview{
		Summary: ReconcileSummary{Added: added, Updated: updated, Removed: removed},
		ChangeSet: knowledge.CommitChangeSet{
			TargetRepository:     repositoryID,
			TargetRef:            snapshotpkg.DefaultRef,
			BaseCommit:           baseCommit,
			ExpectedTargetCommit: baseCommit,
			Operations:           operations,
			Message:              "reconcile",
		},
	}
}
