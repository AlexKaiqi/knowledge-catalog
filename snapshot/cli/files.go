package cli

import (
	"context"
	"encoding/json"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"

	"kc/kernel"
	"kc/snapshot"
	kcclient "kc/snapshot/client"
	"kc/snapshot/dataset"
)

type deliveredCLIEntry struct {
	snapshot.DirectoryEntry
	Path string `json:"path"`
}

func runRemoteFiles(ctx context.Context, client kcclient.Client, command string, flags map[string]any, options kcclient.RequestOptions) (any, error) {
	id, revision, delivered, err := fileCommandInput(command, flags)
	if err != nil {
		return nil, err
	}
	switch command {
	case "files list":
		limit, cursor, err := fileListControls(flags, revision)
		if err != nil {
			return nil, err
		}
		coordinate, pin, err := resolveFilesCoordinate(ctx, client, id, revision, options)
		if err != nil {
			return nil, err
		}
		var page kcclient.WorkspaceFileDirectoryResponse
		err = client.WorkspaceFilesService().Directory(ctx, kcclient.WorkspaceFileDirectoryRequest{WorkspaceFileCoordinate: coordinate, Path: delivered, Limit: limit, Continuation: cursor}, options, &page)
		if err != nil {
			return nil, err
		}
		return shapeDeliveredPage(pin, delivered, limit, page)
	case "files read":
		return runRemoteFileRead(ctx, client, id, revision, delivered, flags, options)
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "unsupported file command %s", command)
	}
}

func fileCommandInput(command string, flags map[string]any) (string, *int, string, error) {
	id, err := requireRemoteFlag(flags, "dataset")
	if err != nil {
		return "", nil, "", err
	}
	revision, err := snapshotRevision(flags)
	if err != nil {
		return "", nil, "", err
	}
	delivered := flagString(flags, "path")
	if (command == "files read" && delivered == "") || (delivered != "" && !validRelativePath(delivered)) {
		return "", nil, "", kernel.Fail(kernel.ErrUsageInvalid, "--path must identify a relative Dataset delivered path without traversal")
	}
	return id, revision, delivered, nil
}

func fileListControls(flags map[string]any, revision *int) (int, string, error) {
	limit, err := pageLimit(flags, defaultHistoryLimit, maxHistoryPageSize)
	if err != nil {
		return 0, "", err
	}
	cursor := flagString(flags, "continuation")
	if cursor != "" && revision == nil {
		return 0, "", kernel.Fail(kernel.ErrUsageInvalid, "continued file listing requires the --revision returned by the first page")
	}
	return limit, cursor, nil
}

func resolveFilesCoordinate(ctx context.Context, client kcclient.Client, id string, revision *int, options kcclient.RequestOptions) (kcclient.WorkspaceFileCoordinate, dataset.ResolvedKnowledgeSet, error) {
	var pin dataset.ResolvedKnowledgeSet
	if err := client.SnapshotService().ResolveKnowledgeSet(ctx, id, kcclient.KnowledgeSetResolveRequest{Revision: revision}, options, &pin); err != nil {
		return kcclient.WorkspaceFileCoordinate{}, pin, err
	}
	if pin.SetID != id || pin.Revision <= 0 || pin.PinID == "" || len(pin.Repositories) == 0 || (revision != nil && pin.Revision != *revision) {
		return kcclient.WorkspaceFileCoordinate{}, pin, kernel.Fail(kernel.ErrPreconditionFailed, "Server returned a different or incomplete Dataset publication")
	}
	raw, err := json.Marshal(pin)
	return kcclient.WorkspaceFileCoordinate{Dataset: id, View: "repository", Pin: raw}, pin, err
}

func sameFilePublication(expected, actual dataset.ResolvedKnowledgeSet) bool {
	return expected.SetID == actual.SetID && expected.Revision == actual.Revision && expected.Ref == actual.Ref && expected.PinID == actual.PinID &&
		maps.Equal(expected.Repositories, actual.Repositories) && slices.Equal(expected.Items, actual.Items)
}

func shapeDeliveredPage(pin dataset.ResolvedKnowledgeSet, dir string, limit int, page kcclient.WorkspaceFileDirectoryResponse) (any, error) {
	if !sameFilePublication(pin, page.Pin) || len(page.Entries) > limit || page.Exhausted != (page.Continuation == "") {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "file directory response changed its publication or paging contract")
	}
	entries := make([]deliveredCLIEntry, 0, len(page.Entries))
	seen := map[string]bool{}
	for _, entry := range page.Entries {
		if !validRelativePath(entry.Name) || strings.Contains(entry.Name, "/") || seen[entry.Name] {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "file directory response contains invalid or repeated child names")
		}
		seen[entry.Name] = true
		entries = append(entries, deliveredCLIEntry{DirectoryEntry: entry, Path: path.Join(dir, entry.Name)})
	}
	output := filePublicationOutput(pin, dir)
	output["entries"], output["exhausted"] = entries, page.Exhausted
	if page.Continuation != "" {
		output["continuation"] = page.Continuation
	}
	return output, nil
}

func filePublicationOutput(pin dataset.ResolvedKnowledgeSet, delivered string) map[string]any {
	return map[string]any{"dataset": pin.SetID, "revision": pin.Revision, "pinId": pin.PinID, "repositories": pin.Repositories, "path": delivered}
}

func optionalFileNumber(flags map[string]any, name string, fallback int64) (int64, error) {
	if _, present := flags[name]; !present {
		return fallback, nil
	}
	value, err := strconv.ParseInt(flagString(flags, name), 10, 64)
	if err != nil || value < 0 {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "--%s must be a non-negative integer", name)
	}
	return value, nil
}
