package cli

import (
	"net/http"
	"strings"
)

// withWorkspaceFiles is the telemetry aspect around VFS application calls.
// Workspace file handlers only choose an operation and supply its source work.
func (f *httpFacade) withWorkspaceFiles(w http.ResponseWriter, r *http.Request, operation string, coordinate workspaceFileCoordinate, requirePin bool, run func(*workspaceFileView) (any, error)) {
	identity, ok := f.serviceIdentity(w, r)
	if !ok {
		return
	}
	ctx, span, started := f.runtime.StartOperation(r.Context(), "vfs", operation)
	ended := false
	defer func() {
		if !ended {
			f.runtime.EndOperation(ctx, span, started, "vfs", operation, "error", "other")
		}
	}()
	view, err := openWorkspaceFileView(f.home, identity.Principal, coordinate, requirePin, func(decision string) {
		f.runtime.RecordAuthorization(ctx, operation, decision)
	})
	if err != nil {
		outcome, errorType := telemetryResult(err)
		f.runtime.EndOperation(ctx, span, started, "vfs", operation, outcome, errorType)
		ended = true
		writeInvoke(w, errorResult(err))
		return
	}
	defer view.Close()
	if value := strings.TrimSpace(r.Header.Get("X-Kc-Request-Id")); value != "" {
		view.flags["request-id"] = value
	}
	spanContext := span.SpanContext()
	if spanContext.IsValid() {
		view.flags["trace-id"] = spanContext.TraceID().String()
		view.flags["span-id"] = spanContext.SpanID().String()
		if parent := httpTraceContext(r).SpanID; parent != "" {
			view.flags["parent-span-id"] = parent
		}
	}
	result, err := run(view)
	outcome, errorType := telemetryResult(err)
	transferredBytes, directoryEntries := vfsVolume(result)
	f.runtime.RecordVFSVolume(ctx, operation, outcome, transferredBytes, directoryEntries)
	f.runtime.EndOperation(ctx, span, started, "vfs", operation, outcome, errorType)
	ended = true
	if err != nil {
		writeInvoke(w, errorResult(err))
		return
	}
	writeInvoke(w, RunResult{Stdout: jsonOut(result)})
}

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
