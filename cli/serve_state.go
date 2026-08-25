package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"kc/controlplane"
	"kc/kernel"
)

func authenticatedWorkspaceState(home string, id HTTPIdentity) map[string]any {
	return map[string]any{
		"ready":    homeReady(home),
		"tree":     workspaceTree(home, id.Principal),
		"identity": identityJSON(id),
	}
}

func identityJSON(id HTTPIdentity) map[string]any {
	return map[string]any{
		"principal": id.Principal,
		"provider":  id.Provider,
		"subject":   id.Subject,
		"login":     id.Login,
	}
}

func workspaceState(home, as string) RunResult {
	out := map[string]any{"home": home, "tree": workspaceTree(home, as)}
	status := Invoke("status", map[string]FlagValue{"home": home})
	if status.Status != 0 {
		var payload map[string]any
		_ = json.Unmarshal([]byte(status.Stdout), &payload)
		out["ready"] = false
		if payload != nil {
			if errObj, ok := payload["error"]; ok {
				out["error"] = errObj
			}
		}
		return RunResult{Status: 0, Stdout: jsonOut(out)}
	}
	var payload any
	if err := json.Unmarshal([]byte(status.Stdout), &payload); err != nil {
		return errorResult(err)
	}
	out["ready"] = true
	out["status"] = payload
	store := controlplane.NewFileControlState(filepath.Join(home, "control.json"))
	bundle, err := store.LoadBundle()
	if err != nil {
		return errorResult(err)
	}
	out["control"] = bundle
	return RunResult{Status: 0, Stdout: jsonOut(out)}
}

func blobStatus(home, dir, ref, path string) (int, map[string]any) {
	if path == "" {
		return http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "missing path"))
	}
	if dir == "" {
		full, err := safeRelPath(home, path)
		if err != nil {
			return http.StatusBadRequest, kernel.FaultJSON(err)
		}
		body, err := os.ReadFile(full)
		if err != nil {
			return http.StatusNotFound, kernel.FaultJSON(kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "%s", err.Error()))
		}
		if len(body) > 512<<10 {
			body = body[:512<<10]
		}
		return http.StatusOK, map[string]any{"path": path, "text": string(body)}
	}
	root, err := safeRelPath(home, dir)
	if err != nil {
		return http.StatusBadRequest, kernel.FaultJSON(err)
	}
	if ref == "" {
		ref = "HEAD"
	}
	if strings.ContainsAny(path, "\x00") || strings.Contains(path, "..") {
		return http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "bad path"))
	}
	text, err := gitOutput(root, "show", ref+":"+path)
	if err != nil {
		return http.StatusNotFound, kernel.FaultJSON(kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "not in %s", ref))
	}
	return http.StatusOK, map[string]any{"dir": dir, "ref": ref, "path": path, "text": text, "objectId": objectIDFromFrontmatter(text)}
}
