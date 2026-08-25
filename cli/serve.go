package cli

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"kc/controlplane"
	"kc/kernel"
)

//go:embed console.html
var consoleHTML []byte

const defaultListen = "127.0.0.1:7380"

func runServe(flags map[string]FlagValue) RunResult {
	home, err := resolveHome(flags)
	if err != nil {
		return errorResult(err)
	}
	options, err := httpServerOptionsFromFlags(flags)
	if err != nil {
		return errorResult(err)
	}
	listen := FlagString(flags, "listen")
	if listen == "" {
		listen = defaultListen
	}
	authMode := "local-owner"
	identityLine := "header X-Kc-As → --as"
	if options.authenticated() {
		authMode = options.Authenticator.Name()
		identityLine = "Authorization → verified principal; X-Kc-As disabled"
	}
	_, _ = fmt.Fprintf(os.Stdout, "kc HTTP facade\n  home    %s\n  listen  http://%s/\n  auth    %s\n  POST    /v1/<verb>  JSON flags (CLI names; --home pinned here)\n  as      %s\n  corr    header X-Kc-Request-Id → --request-id\n", home, listen, authMode, identityLine)
	if err := http.ListenAndServe(listen, HTTPHandlerWithOptions(home, options)); err != nil {
		return errorResult(err)
	}
	return RunResult{Status: 0}
}

// HTTPHandler is the same facade as `kc` verbs, pinned to one --home.
func HTTPHandler(home string) http.Handler {
	return HTTPHandlerWithOptions(home, HTTPServerOptions{})
}

// HTTPHandlerWithOptions adds a trusted authentication boundary to the same
// verb facade. Without an Authenticator it is exactly the legacy local-owner
// handler used by CLI tests and single-user development.
func HTTPHandlerWithOptions(home string, options HTTPServerOptions) http.Handler {
	mux := http.NewServeMux()
	// Invoke opens the persisted Home for each request. Concurrent readers may
	// use independent snapshots, but every mutation is serialized so two
	// requests cannot both load the same writer/allow/catalog file and then
	// overwrite one another. Snapshot CAS still decides stale repository writes;
	// this lock protects the service's own file-backed control state.
	var invokeMu sync.RWMutex
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(consoleHTML)
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{"ok": true, "auth": "local-owner"}
		if options.authenticated() {
			body["auth"] = options.Authenticator.Name()
		} else {
			body["home"] = home
		}
		writeJSON(w, http.StatusOK, body)
	})
	mux.HandleFunc("GET /v1/_state", func(w http.ResponseWriter, r *http.Request) {
		id, ok := authenticateHTTPRequest(w, r, options)
		if !ok {
			return
		}
		if options.authenticated() {
			if strings.TrimSpace(r.Header.Get("X-Kc-As")) != "" {
				writeHTTPForbidden(w, "X-Kc-As is disabled when authentication is enabled")
				return
			}
			if !options.isAdmin(id) {
				writeJSON(w, http.StatusOK, authenticatedWorkspaceState(home, id))
				return
			}
		}
		as := strings.TrimSpace(r.Header.Get("X-Kc-As"))
		if options.authenticated() {
			as = id.Principal
		}
		writeInvoke(w, workspaceState(home, as))
	})
	mux.HandleFunc("GET /v1/_blob", func(w http.ResponseWriter, r *http.Request) {
		id, ok := authenticateHTTPRequest(w, r, options)
		if !ok {
			return
		}
		if options.authenticated() && !options.isAdmin(id) {
			writeHTTPForbidden(w, "%s is not allowed to inspect server files", id.Principal)
			return
		}
		code, body := blobStatus(home, r.URL.Query().Get("dir"), r.URL.Query().Get("ref"), r.URL.Query().Get("path"))
		writeJSON(w, code, body)
	})
	mux.HandleFunc("POST /v1/{verb}", func(w http.ResponseWriter, r *http.Request) {
		verb := r.PathValue("verb")
		// Same table the CLI dispatches on, so the two transports cannot drift
		// on which verbs exist. `serve` is not in it.
		if !Verb(verb) {
			writeJSON(w, http.StatusNotFound, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "unknown command %s", verb)))
			return
		}
		id, ok := authenticateHTTPRequest(w, r, options)
		if !ok {
			return
		}
		if options.authenticated() && strings.TrimSpace(r.Header.Get("X-Kc-As")) != "" {
			writeHTTPForbidden(w, "X-Kc-As is disabled when authentication is enabled")
			return
		}
		raw, err := decodeJSONBody(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, kernel.FaultJSON(err))
			return
		}
		if options.authenticated() && requiresHTTPAdmin(verb, raw, id) && !options.isAdmin(id) {
			writeHTTPForbidden(w, "%s is not allowed to administer kc", id.Principal)
			return
		}
		flags, cleanup, err := flagsFromRequest(home, raw)
		if cleanup != nil {
			defer cleanup()
		}
		if err != nil {
			writeJSON(w, http.StatusBadRequest, kernel.FaultJSON(err))
			return
		}
		if options.authenticated() {
			flags["as"] = id.Principal
			flags["auth-provider"] = id.Provider
			flags["auth-subject"] = id.Subject
			flags["auth-login"] = id.Login
		} else if as := strings.TrimSpace(r.Header.Get("X-Kc-As")); as != "" {
			flags["as"] = as
		}
		if reqID := strings.TrimSpace(r.Header.Get("X-Kc-Request-Id")); reqID != "" {
			flags["request-id"] = reqID
		}
		for header, flag := range map[string]string{
			"X-Kc-On-Behalf-Of":   "on-behalf-of",
			"X-Kc-Trace-Id":       "trace-id",
			"X-Kc-Span-Id":        "span-id",
			"X-Kc-Parent-Span-Id": "parent-span-id",
			"X-Kc-Session-Id":     "session-id",
		} {
			if value := strings.TrimSpace(r.Header.Get(header)); value != "" {
				flags[flag] = value
			}
		}
		if readOnlyHTTPVerb(verb) {
			invokeMu.RLock()
			defer invokeMu.RUnlock()
		} else {
			invokeMu.Lock()
			defer invokeMu.Unlock()
		}
		writeInvoke(w, Invoke(verb, flags))
	})
	return mux
}

func httpServerOptionsFromFlags(flags map[string]FlagValue) (HTTPServerOptions, error) {
	mode := strings.TrimSpace(FlagString(flags, "auth"))
	url := strings.TrimSpace(FlagString(flags, "auth-url"))
	admins := FlagStrings(flags, "auth-admin")
	switch mode {
	case "":
		if url != "" || len(admins) > 0 {
			return HTTPServerOptions{}, fmt.Errorf("--auth-url/--auth-admin require --auth gitea")
		}
		return HTTPServerOptions{}, nil
	case "gitea":
		if url == "" {
			return HTTPServerOptions{}, fmt.Errorf("--auth gitea requires --auth-url")
		}
		authenticator, err := NewGiteaAuthenticator(url, nil)
		if err != nil {
			return HTTPServerOptions{}, err
		}
		return HTTPServerOptions{Authenticator: authenticator, AdminPrincipals: admins}, nil
	default:
		return HTTPServerOptions{}, fmt.Errorf("--auth must be gitea")
	}
}

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

func readOnlyHTTPVerb(verb string) bool {
	switch verb {
	case "help", "status", "store-ls", "whoami", "allowed", "receipt",
		"read", "list", "search", "provenance", "log", "stream",
		"describe-schema", "resolve", "inspect", "diff", "describe-index",
		"audit", "access-log", "trace", "hitmap", "hook-ls", "gate-ls", "vfs-read", "vfs-list":
		return true
	default:
		return false
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

func decodeJSONBody(r *http.Request) (map[string]any, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return map[string]any{}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("body must be a JSON object")
	}
	return raw, nil
}

func flagsFromRequest(home string, raw map[string]any) (map[string]FlagValue, func(), error) {
	flags := map[string]FlagValue{"home": home}
	var cleanup []string
	done := func() {
		for _, p := range cleanup {
			_ = os.Remove(p)
		}
	}
	for key, value := range raw {
		if key == "home" || key == "listen" {
			continue
		}
		if key == "changeset" {
			if obj, ok := value.(map[string]any); ok {
				b, err := json.Marshal(obj)
				if err != nil {
					return nil, done, err
				}
				f, err := os.CreateTemp(home, "changeset-*.json")
				if err != nil {
					return nil, done, err
				}
				if _, err := f.Write(append(b, '\n')); err != nil {
					_ = f.Close()
					return nil, done, err
				}
				if err := f.Close(); err != nil {
					return nil, done, err
				}
				cleanup = append(cleanup, f.Name())
				flags[key] = f.Name()
				continue
			}
		}
		fv, err := jsonToFlag(value)
		if err != nil {
			return nil, done, err
		}
		if fv != nil {
			flags[key] = fv
		}
	}
	return flags, done, nil
}

func jsonToFlag(value any) (FlagValue, error) {
	switch t := value.(type) {
	case nil:
		return nil, nil
	case bool:
		return t, nil
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t)), nil
		}
		return fmt.Sprint(t), nil
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s, err := jsonToString(item)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		return out, nil
	case map[string]any:
		b, err := json.Marshal(t)
		if err != nil {
			return nil, err
		}
		return string(b), nil
	default:
		return fmt.Sprint(t), nil
	}
}

func jsonToString(value any) (string, error) {
	switch t := value.(type) {
	case map[string]any, []any:
		b, err := json.Marshal(t)
		return string(b), err
	default:
		fv, err := jsonToFlag(value)
		if err != nil {
			return "", err
		}
		if fv == nil {
			return "", nil
		}
		return fmt.Sprint(fv), nil
	}
}

func writeInvoke(w http.ResponseWriter, result RunResult) {
	code := http.StatusOK
	if result.Status != 0 {
		code = http.StatusBadRequest
		var payload map[string]any
		if json.Unmarshal([]byte(result.Stdout), &payload) == nil {
			if errObj, ok := payload["error"].(map[string]any); ok {
				switch kernel.ErrorCode(fmt.Sprint(errObj["code"])) {
				case kernel.ErrUnauthenticated:
					code = http.StatusUnauthorized
				case kernel.ErrForbidden:
					code = http.StatusForbidden
				case kernel.ErrKnowledgeRefUnresolved, kernel.ErrVersionUnresolved:
					code = http.StatusNotFound
				case kernel.ErrTemporaryUnavailable:
					code = http.StatusServiceUnavailable
				case kernel.ErrNonFastForward, kernel.ErrIdempotencyConflict, kernel.ErrPreconditionFailed,
					kernel.ErrEventIDConflict, kernel.ErrCandidateMoved,
					kernel.ErrGateUnsatisfied, kernel.ErrCatalogArchived, kernel.ErrRepositoryArchived:
					code = http.StatusConflict
				}
			}
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(result.Stdout))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(jsonOut(value)))
}
