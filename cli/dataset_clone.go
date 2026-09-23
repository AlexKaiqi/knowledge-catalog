package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	kcclient "kc/client"
	"kc/catalog"
	"kc/kernel"
)

// kc dataset clone materializes the current serving version of one published
// Dataset into a local directory: the composed delivered tree, raw bytes, no
// FUSE and no mount. The clone resolves the Dataset once (V-01) and then
// walks the delivered tree through the same closed File Gateway surface the
// file-system projection uses, so authorization and the frozen file list are
// identical to ordinary consumption. It never writes outside the target
// directory and it refuses a non-empty target: a clone does not overwrite
// existing project files (docs/reviewed/dataset.md, U8, "不复制、不覆盖").
//
// The receipt prints the version coordinates instead of writing a manifest:
// the accepted publication record stays the server-side authority for what
// was consumed, and consumers do not manage pin files (docs/CLI.md).
const (
	cloneChunkSize = 512 << 10
	clonePageLimit = 256
	cloneMaxDepth  = 512
)

func runRemoteDatasetClone(ctx context.Context, client *kcclient.Client, catalogID string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	setID := strings.TrimSpace(FlagString(flags, "dataset"))
	if setID == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "dataset clone requires <dataset> <dir>")
	}
	dir := FlagString(flags, "dir")
	if strings.TrimSpace(dir) == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "dataset clone requires a target directory: kc dataset clone <dataset> <dir>")
	}
	if err := cloneTargetAcceptable(dir); err != nil {
		return nil, err
	}
	coordinate, pin, err := cloneCoordinate(ctx, client, catalogID, setID, options)
	if err != nil {
		return nil, err
	}
	mounts, fileEntries, err := cloneSourceSummary(ctx, client, coordinate, pin.PinID, options)
	if err != nil {
		return nil, err
	}
	if len(mounts) == 0 && fileEntries == 0 {
		return nil, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "dataset %s publishes no delivered content", setID)
	}
	files, bytes, err := cloneWalk(ctx, client, coordinate, pin.PinID, dir, "", 0, options)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"dataset":     setID,
		"catalog":     catalogID,
		"ref":         pin.Ref,
		"revision":    pin.Revision,
		"pinId":       pin.PinID,
		"dir":         dir,
		"files":       files,
		"bytes":       bytes,
		"mounts":      mounts,
		"fileEntries": fileEntries,
	}, nil
}

// cloneCoordinate resolves the Dataset version exactly once and freezes it
// as the pin every subsequent gateway call replays. Product argv does not
// carry --pin (docs/CLI.md): clone consumes the current serving version, and
// historical review goes through the server-side task pin, not this verb.
func cloneCoordinate(ctx context.Context, client *kcclient.Client, catalogID, setID string, options kcclient.RequestOptions) (kcclient.WorkspaceFileCoordinate, catalog.ResolvedKnowledgeSet, error) {
	var pin catalog.ResolvedKnowledgeSet
	if err := client.CatalogService().ResolveKnowledgeSet(ctx, catalogID, setID, kcclient.KnowledgeSetResolveRequest{}, options, &pin); err != nil {
		return kcclient.WorkspaceFileCoordinate{}, catalog.ResolvedKnowledgeSet{}, err
	}
	marshaled, err := json.Marshal(pin)
	if err != nil {
		return kcclient.WorkspaceFileCoordinate{}, catalog.ResolvedKnowledgeSet{}, err
	}
	coordinate := kcclient.WorkspaceFileCoordinate{Catalog: catalogID, Dataset: setID, View: "repository", Pin: marshaled}
	return coordinate, pin, nil
}

// cloneTargetAcceptable refuses a target that already holds files: a clone
// materializes a view, it does not merge into or overwrite a project.
func cloneTargetAcceptable(dir string) error {
	info, err := os.Stat(dir)
	switch {
	case os.IsNotExist(err):
		return nil
	case err != nil:
		return err
	case !info.IsDir():
		return kernel.Fail(kernel.ErrUsageInvalid, "clone target %s is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return kernel.Fail(kernel.ErrPreconditionFailed,
			"clone target %s is not empty; clone never overwrites existing files", dir)
	}
	return nil
}

// cloneSourceSummary names every source version the pin delivers so the
// receipt can carry the consumption coordinates without a manifest file.
func cloneSourceSummary(ctx context.Context, client *kcclient.Client, coordinate kcclient.WorkspaceFileCoordinate, pinID string, options kcclient.RequestOptions) ([]map[string]any, int, error) {
	var out kcclient.WorkspaceFileMountsResponse
	if err := client.WorkspaceFilesService().Mounts(ctx, kcclient.WorkspaceFileMountsRequest{WorkspaceFileCoordinate: coordinate}, options, &out); err != nil {
		return nil, 0, err
	}
	if out.Pin.PinID != pinID {
		return nil, 0, kernel.Fail(kernel.ErrPreconditionFailed, "dataset %s moved from pin %s while cloning", coordinate.Dataset, pinID)
	}
	mounts := make([]map[string]any, 0, len(out.Mounts))
	for _, mount := range out.Mounts {
		mounts = append(mounts, map[string]any{"path": mount.Path, "repository": mount.Repository, "commit": mount.Commit})
	}
	fileEntries := 0
	for _, item := range out.Pin.Items {
		if item.Kind == catalog.DatasetItemFile {
			fileEntries++
		}
	}
	return mounts, fileEntries, nil
}

// cloneWalk enumerates the delivered tree through the File Gateway and writes
// every file under root. Each level pages through continuations until the
// gateway reports exhaustion.
func cloneWalk(ctx context.Context, client *kcclient.Client, coordinate kcclient.WorkspaceFileCoordinate, pinID, root, delivered string, depth int, options kcclient.RequestOptions) (int, int64, error) {
	if depth > cloneMaxDepth {
		return 0, 0, kernel.Fail(kernel.ErrPreconditionFailed, "delivered tree of dataset %s is deeper than %d levels", coordinate.Dataset, cloneMaxDepth)
	}
	files, bytes := 0, int64(0)
	continuation := ""
	seen := map[string]struct{}{}
	for {
		var page kcclient.WorkspaceFileDirectoryResponse
		request := kcclient.WorkspaceFileDirectoryRequest{
			WorkspaceFileCoordinate: coordinate, Path: delivered, Limit: clonePageLimit, Continuation: continuation,
		}
		if err := client.WorkspaceFilesService().Directory(ctx, request, options, &page); err != nil {
			return 0, 0, err
		}
		if page.Pin.PinID != pinID {
			return 0, 0, kernel.Fail(kernel.ErrPreconditionFailed, "dataset %s moved from pin %s while cloning", coordinate.Dataset, pinID)
		}
		for _, entry := range page.Entries {
			if entry.Name == "" || strings.ContainsRune(entry.Name, '/') {
				return 0, 0, kernel.Fail(kernel.ErrPreconditionFailed, "dataset gateway returned an invalid delivered child %q", entry.Name)
			}
			if _, dup := seen[entry.Name]; dup {
				continue
			}
			seen[entry.Name] = struct{}{}
			child := entry.Name
			if delivered != "" {
				child = delivered + "/" + entry.Name
			}
			switch entry.Kind {
			case "file":
				written, err := cloneFile(ctx, client, coordinate, pinID, root, child, options)
				if err != nil {
					return 0, 0, err
				}
				files++
				bytes += written
			case "directory":
				subFiles, subBytes, err := cloneWalk(ctx, client, coordinate, pinID, root, child, depth+1, options)
				if err != nil {
					return 0, 0, err
				}
				files += subFiles
				bytes += subBytes
			default:
				return 0, 0, kernel.Fail(kernel.ErrPreconditionFailed, "dataset gateway returned unknown delivered kind %q", entry.Kind)
			}
		}
		if page.Exhausted {
			return files, bytes, nil
		}
		if page.Continuation == "" || page.Continuation == continuation {
			return 0, 0, kernel.Fail(kernel.ErrPreconditionFailed, "dataset gateway returned a non-advancing continuation")
		}
		continuation = page.Continuation
	}
}

// cloneFile streams one delivered file in chunks and writes it below root.
// Paths are re-validated locally: the gateway normalizes, but the clone never
// trusts a server string to place bytes on disk.
func cloneFile(ctx context.Context, client *kcclient.Client, coordinate kcclient.WorkspaceFileCoordinate, pinID, root, delivered string, options kcclient.RequestOptions) (int64, error) {
	if !validKnowledgeSetFSRelative(delivered) {
		return 0, kernel.Fail(kernel.ErrPreconditionFailed, "dataset gateway returned an invalid delivered path %q", delivered)
	}
	target := filepath.Join(root, filepath.FromSlash(delivered))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, err
	}
	var written int64
	offset := int64(0)
	for {
		var response kcclient.WorkspaceFileReadResponse
		request := kcclient.WorkspaceFileReadRequest{
			WorkspaceFileCoordinate: coordinate, Path: delivered, Offset: offset, Length: cloneChunkSize,
		}
		if err := client.WorkspaceFilesService().Read(ctx, request, options, &response); err != nil {
			return 0, err
		}
		if response.Pin.PinID != pinID || response.Path != delivered || response.Offset != offset {
			return 0, kernel.Fail(kernel.ErrPreconditionFailed, "dataset %s moved while reading %s", coordinate.Dataset, delivered)
		}
		if len(response.Content) > 0 {
			if offset == 0 {
				if err := os.WriteFile(target, response.Content, 0o644); err != nil {
					return 0, err
				}
			} else {
				file, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0o644)
				if err != nil {
					return 0, err
				}
				_, writeErr := file.Write(response.Content)
				closeErr := file.Close()
				if writeErr != nil {
					return 0, writeErr
				}
				if closeErr != nil {
					return 0, closeErr
				}
			}
			written += int64(len(response.Content))
		}
		if response.EOF {
			return written, nil
		}
		if len(response.Content) == 0 {
			return 0, kernel.Fail(kernel.ErrPreconditionFailed, "dataset gateway stopped advancing while reading %s", delivered)
		}
		offset += int64(len(response.Content))
	}
}
