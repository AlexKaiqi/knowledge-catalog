package server

// The Snapshot module owns file Dataset DTOs and the raw file engine. This
// adapter retains the KC semantic view and evidence only; it does not perform
// another raw path routing or byte-range implementation.
import (
	"context"
	"encoding/json"
	"sort"

	"kc/identity"
	inv "kc/invocation"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
	files "kc/snapshot/service"
)

type workspaceFileMountsResponse = files.FileMountsResponse
type workspaceFileDirectoryResponse = files.FileDirectoryResponse
type workspaceFileReadResponse = files.FileReadResponse

func (f *httpFacade) snapshotSemanticView(ctx context.Context, opened *Home, caller identity.HTTPIdentity, coordinate files.FileCoordinate, cat dataset.Service, definition dataset.KnowledgeSet, pin dataset.ResolvedKnowledgeSet) (files.FileView, error) {
	if coordinate.View != inv.SemanticFileViewV1 && coordinate.View != "semantic" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "view must be repository or semantic")
	}
	flags := snapshotRequestFlags(ctx, caller, access.Query{Action: "file.read", Dataset: coordinate.Dataset})
	raw, err := json.Marshal(pin)
	if err != nil {
		return nil, err
	}
	flags["pin"] = string(raw)
	projections := map[kernel.RepositoryID]*inv.SemanticProjection{}
	mounts := []dataset.VirtualMount{}
	for _, mount := range inv.SemanticMounts(definition, pin) {
		allowed, err := inv.DatasetFSMayReadRepository(opened.Dir, flags, string(mount.Repository), opened)
		if err != nil {
			return nil, err
		}
		if !allowed {
			continue
		}
		repo, err := opened.Reader.Require(mount.Repository, kernel.ErrCapabilityUnsatisfied)
		if err != nil {
			return nil, err
		}
		store, err := opened.Store.Require(mount.Repository, kernel.ErrCapabilityUnsatisfied)
		if err != nil {
			return nil, err
		}
		projection, err := inv.SemanticProjectionFor(store, repo, pin)
		if err != nil {
			return nil, err
		}
		projections[mount.Repository] = projection
		mounts = append(mounts, mount)
	}
	sort.Slice(mounts, func(i, j int) bool { return mounts[i].Path < mounts[j].Path })
	return files.NewFileView(files.ViewOptions{Pin: pin, Mounts: mounts, Sources: opened.Store,
		Directory: func(mount dataset.VirtualMount, directory string, limit int, continuation string) (snapshot.DirectoryPage, error) {
			entries, next, exhausted, err := projections[mount.Repository].List(directory, limit, continuation)
			return snapshot.DirectoryPage{Entries: entries, Continuation: next, Exhausted: exhausted, Generation: string(mount.Commit)}, err
		},
		Read: func(mount dataset.VirtualMount, file string) ([]byte, error) {
			return projections[mount.Repository].Read(file)
		},
		Record: func(evidence files.FileReadEvidence) error {
			return f.snapshotRecordFileRead(ctx, opened, caller, evidence)
		},
	}), nil
}

func (f *httpFacade) snapshotRecordFileRead(ctx context.Context, opened *Home, caller identity.HTTPIdentity, evidence files.FileReadEvidence) error {
	flags := snapshotRequestFlags(ctx, caller, access.Query{Action: "file.read", Dataset: evidence.Pin.SetID, Repo: string(evidence.Repository)})
	flags["home"], flags["path"] = opened.Dir, evidence.Path
	raw, err := json.Marshal(evidence.Pin)
	if err != nil {
		return err
	}
	flags["pin"] = string(raw)
	result := map[string]any{"path": evidence.Path, "repository": evidence.Repository, "commit": evidence.Commit, "bytes": evidence.Bytes}
	if evidence.File != "" {
		result = map[string]any{"mountPath": evidence.MountPath, "file": evidence.File, "repository": evidence.Repository, "commit": evidence.Commit}
	}
	_, _, err = inv.RecordKnowledgeAccess(opened.Dir, "file-read", flags, result, evidence.Err)
	return err
}
