package writer

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	snapshotpkg "kc/snapshot"
)

// Ingest / Reconcile preview a ChangeSet. They are not a Surface and not a
// Collector: they do not write. Confirm with Commit. The function is still
// named Ingest to match `kc ingest`; this file is not called ingestion.

type IngestPreview struct {
	ChangeSet objectmodel.CommitChangeSet `json:"changeSet"`
	Files     []IngestFile                `json:"files"`
}

type IngestFile struct {
	Path           string               `json:"path"`
	ObjectID       objectmodel.ObjectID `json:"objectId"`
	Address        objectmodel.Address  `json:"address"`
	SchemaRef      string               `json:"schemaRef,omitempty"`
	IdentitySource string               `json:"identitySource"`
}

func Ingest(dir string, repositoryID kernel.RepositoryID, baseCommit kernel.CommitID) (IngestPreview, error) {
	var operations []objectmodel.Operation
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
		return objectmodel.AddressKey(operations[i].Address) < objectmodel.AddressKey(operations[j].Address)
	})
	return IngestPreview{
		Files: files,
		ChangeSet: objectmodel.CommitChangeSet{
			TargetRepository:     repositoryID,
			TargetRef:            snapshotpkg.DefaultRef,
			BaseCommit:           baseCommit,
			ExpectedTargetCommit: baseCommit,
			Operations:           operations,
			Message:              "ingest " + dir,
			Provenance:           &objectmodel.ProvenanceEnvelope{OriginKind: objectmodel.OriginSource, SourceRefs: []string{dir}},
		},
	}, nil
}

// DesiredSummary is the scoped compare of a desired directory against one
// commit. Objects not in the directory are left untouched.
type DesiredSummary struct {
	Added     int `json:"added"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
}

// OmitUnchanged drops PUTs whose Canonical digest already matches current.
// current is AddressKey → digest at the compare commit. Missing keys are adds.
func OmitUnchanged(cs objectmodel.CommitChangeSet, current map[string]string) (objectmodel.CommitChangeSet, DesiredSummary) {
	var operations []objectmodel.Operation
	summary := DesiredSummary{}
	for _, op := range cs.Operations {
		if op.Op != objectmodel.OpPut {
			operations = append(operations, op)
			continue
		}
		digest := string(kernel.CanonicalDigest(op.Value))
		existing, ok := current[objectmodel.AddressKey(op.Address)]
		switch {
		case !ok:
			summary.Added++
			operations = append(operations, op)
		case existing != digest:
			summary.Updated++
			operations = append(operations, op)
		default:
			summary.Unchanged++
		}
	}
	cs.Operations = operations
	return cs, summary
}

func ingestFile(rel string, content []byte) (objectmodel.Operation, IngestFile, error) {
	if unit := repofile.Parse(string(content)); unit != nil {
		if err := unit.DeclarationError(); err != nil {
			return objectmodel.Operation{}, IngestFile{}, err
		}
		if unit.ValueSource.Normalized() != nil {
			return objectmodel.Operation{}, IngestFile{}, kernel.Fail(kernel.ErrUsageInvalid,
				"Bound State is declared on Domain Schema origin, not an instance value_source")
		}
		return objectmodel.Operation{
				Op:          objectmodel.OpPut,
				Address:     unit.Address,
				Value:       unit.Value,
				PathHint:    repofile.PathHintForIngest(unit.Address, unit.PathHint, rel),
				SchemaRef:   unit.SchemaRef,
				ValueSource: unit.ValueSource,
			}, IngestFile{
				Path: rel, ObjectID: unit.Address.ObjectID, Address: unit.Address,
				SchemaRef: unit.SchemaRef, IdentitySource: "frontmatter",
			}, nil
	}
	objectID := rel
	for _, ext := range []string{".md", ".json", ".okf", ".yaml", ".yml", ".txt"} {
		if strings.HasSuffix(strings.ToLower(objectID), ext) {
			objectID = objectID[:len(objectID)-len(ext)]
			break
		}
	}
	address := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(objectID)}
	value := any(string(content))
	if strings.HasSuffix(strings.ToLower(rel), ".json") {
		var parsed any
		if kernel.UnmarshalJSON(content, &parsed) == nil {
			value = parsed
		}
	}
	return objectmodel.Operation{
			Op:       objectmodel.OpPut,
			Address:  address,
			Value:    value,
			PathHint: repofile.PathHintForIngest(address, "", rel),
		}, IngestFile{
			Path: rel, ObjectID: address.ObjectID, Address: address,
			IdentitySource: "path",
		}, nil
}

type ReconcilePreview struct {
	ChangeSet objectmodel.CommitChangeSet `json:"changeSet"`
	Summary   ReconcileSummary            `json:"summary"`
}

type ReconcileSummary struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Removed int `json:"removed"`
}

func Reconcile(snapshot map[objectmodel.ObjectID]any, current map[objectmodel.ObjectID]string, repositoryID kernel.RepositoryID, baseCommit kernel.CommitID) ReconcilePreview {
	var operations []objectmodel.Operation
	added, updated, removed := 0, 0, 0
	for objectID, value := range snapshot {
		digest := string(kernel.CanonicalDigest(value))
		existing, ok := current[objectID]
		if !ok {
			operations = append(operations, objectmodel.Operation{
				Op:           objectmodel.OpPut,
				Address:      objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectID},
				Value:        value,
				Precondition: &objectmodel.Precondition{Type: objectmodel.IfAbsent},
			})
			added++
		} else if existing != digest {
			operations = append(operations, objectmodel.Operation{
				Op:           objectmodel.OpPut,
				Address:      objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectID},
				Value:        value,
				Precondition: &objectmodel.Precondition{Type: objectmodel.IfDigestEquals, Digest: kernel.Digest(existing)},
			})
			updated++
		}
	}
	for objectID := range current {
		if _, ok := snapshot[objectID]; !ok {
			operations = append(operations, objectmodel.Operation{
				Op:      objectmodel.OpRemove,
				Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectID},
				Reason:  "absent-from-snapshot",
			})
			removed++
		}
	}
	return ReconcilePreview{
		Summary: ReconcileSummary{Added: added, Updated: updated, Removed: removed},
		ChangeSet: objectmodel.CommitChangeSet{
			TargetRepository:     repositoryID,
			TargetRef:            snapshotpkg.DefaultRef,
			BaseCommit:           baseCommit,
			ExpectedTargetCommit: baseCommit,
			Operations:           operations,
			Message:              "reconcile",
		},
	}
}
