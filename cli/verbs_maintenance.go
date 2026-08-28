package cli

import (
	"bufio"
	"encoding/json"
	"os"

	"kc/kernel"
	"kc/knowledge/maintenance"
)

func maintenanceVerbs() map[string]command {
	return map[string]command{
		"snapshot-export": {stage: stageGoverned, run: verbSnapshotExport},
		"check-workspace": {stage: stageGoverned, run: verbCheckWorkspace},
	}
}

func verbCheckWorkspace(cx *invocation) (any, error) {
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	workspaceID, err := cx.workspaceID()
	if err != nil {
		return nil, err
	}
	resolved, err := resolveOrReplay(cx.WS, cx.Home, cat, workspaceID, cx.Flags)
	if err != nil {
		return nil, err
	}
	return cat.CheckResolved(resolved), nil
}

// verbSnapshotExport is the explicit maintenance replacement for public LIST.
// It streams JSON Lines to a caller-selected file and returns only a receipt.
func verbSnapshotExport(cx *invocation) (any, error) {
	repositoryID, commitID, err := pinCommit(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	repo, err := cx.WS.Reader.Require(repositoryID, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		return nil, err
	}
	scanner, err := maintenance.RequireScanner(repo)
	if err != nil {
		return nil, err
	}
	out, err := cx.require("out")
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		_ = file.Close()
		if !committed {
			_ = os.Remove(out)
		}
	}()
	writer := bufio.NewWriter(file)
	count := 0
	request := maintenance.ScanRequest{Limit: maintenance.MaxScanLimit}
	for {
		page, scanErr := scanner.ScanSnapshotPage(commitID, request)
		if scanErr != nil {
			return nil, scanErr
		}
		for _, value := range page.Values {
			raw, marshalErr := json.Marshal(value)
			if marshalErr != nil {
				return nil, marshalErr
			}
			if _, writeErr := writer.Write(append(raw, '\n')); writeErr != nil {
				return nil, writeErr
			}
			count++
		}
		if page.Exhausted {
			break
		}
		request.Continuation = page.Continuation
	}
	if err := writer.Flush(); err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	committed = true
	return map[string]any{"repository": repositoryID, "commit": commitID, "out": out, "objects": count}, nil
}
