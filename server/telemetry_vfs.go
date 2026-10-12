package server

func vfsVolume(result any) (transferredBytes, directoryEntries int) {
	transferredBytes, directoryEntries = -1, -1
	switch value := result.(type) {
	case workspaceFileReadResponse:
		transferredBytes = len(value.Content)
	case workspaceFileDirectoryResponse:
		directoryEntries = len(value.Entries)
	case workspaceFileMountsResponse:
		directoryEntries = len(value.Mounts)
	}
	return
}
