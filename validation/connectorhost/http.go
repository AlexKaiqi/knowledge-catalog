package connectorhost

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (h *Host) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeHTTPJSON(w, http.StatusOK, map[string]any{"ok": true, "repo": h.config.RepoPath, "kcUrl": h.config.KCURL})
	})
	mux.HandleFunc("GET /api/connectors", func(w http.ResponseWriter, r *http.Request) {
		items, err := h.Connectors(r.Context(), false)
		if err != nil {
			writeHTTPError(w, err)
			return
		}
		writeHTTPJSON(w, http.StatusOK, items)
	})
	mux.HandleFunc("GET /api/connectors/{id}", func(w http.ResponseWriter, r *http.Request) {
		loaded, err := h.Connector(r.PathValue("id"))
		if err != nil {
			writeHTTPError(w, err)
			return
		}
		state, err := h.store.LoadState(loaded.Manifest.Metadata.ID)
		if err != nil {
			writeHTTPError(w, err)
			return
		}
		writeHTTPJSON(w, http.StatusOK, ConnectorInfo{Manifest: loaded.Manifest, Path: loaded.Dir, Generation: loaded.Generation, State: state})
	})
	mux.HandleFunc("GET /api/connectors/{id}/runs", func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 50
		}
		runs, err := h.store.Runs(r.PathValue("id"), limit)
		if err != nil {
			writeHTTPError(w, err)
			return
		}
		writeHTTPJSON(w, http.StatusOK, runs)
	})
	mux.HandleFunc("POST /api/connectors/{id}/validate", func(w http.ResponseWriter, r *http.Request) {
		loaded, err := h.Connector(r.PathValue("id"))
		if err == nil {
			err = ValidateConnector(r.Context(), loaded)
		}
		if err != nil {
			writeHTTPError(w, err)
			return
		}
		writeHTTPJSON(w, http.StatusOK, map[string]any{"valid": true, "generation": loaded.Generation})
	})
	mux.HandleFunc("POST /api/connectors/{id}/activate", func(w http.ResponseWriter, r *http.Request) {
		state, err := h.Activate(r.Context(), r.PathValue("id"))
		if err != nil {
			writeHTTPError(w, err)
			return
		}
		writeHTTPJSON(w, http.StatusOK, state)
	})
	mux.HandleFunc("POST /api/connectors/{id}/pause", func(w http.ResponseWriter, r *http.Request) {
		state, err := h.Pause(r.PathValue("id"))
		if err != nil {
			writeHTTPError(w, err)
			return
		}
		writeHTTPJSON(w, http.StatusOK, state)
	})
	mux.HandleFunc("POST /api/connectors/{id}/run", func(w http.ResponseWriter, r *http.Request) {
		preview := r.URL.Query().Get("preview") == "true"
		run, err := h.Run(r.Context(), r.PathValue("id"), RunTrigger{Kind: "manual", At: nowString(time.Now())}, preview, false)
		if err != nil {
			writeHTTPJSON(w, http.StatusUnprocessableEntity, run)
			return
		}
		writeHTTPJSON(w, http.StatusOK, run)
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		items, err := h.Connectors(r.Context(), false)
		if err != nil {
			writeHTTPError(w, err)
			return
		}
		var rows []dashboardRow
		for _, item := range items {
			runs, _ := h.store.Runs(item.Manifest.Metadata.ID, 10)
			rows = append(rows, dashboardRow{Info: item, Runs: runs})
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = dashboardTemplate.Execute(w, map[string]any{"Rows": rows, "Repo": h.config.RepoPath, "KCURL": h.config.KCURL})
	})
	return mux
}

type dashboardRow struct {
	Info ConnectorInfo
	Runs []RunRecord
}

func writeHTTPJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeHTTPError(w http.ResponseWriter, err error) {
	writeHTTPJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": err.Error()}})
}

var dashboardTemplate = template.Must(template.New("dashboard").Funcs(template.FuncMap{
	"short": func(value string) string {
		if len(value) > 20 {
			return value[:20] + "…"
		}
		return value
	},
	"join": strings.Join,
}).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>Connector Host</title>
  <style>
    :root{color-scheme:dark;background:#11151b;color:#e9eef5;font:15px/1.5 ui-sans-serif,system-ui,sans-serif}
    body{max-width:1180px;margin:0 auto;padding:32px} h1{margin:0 0 4px;font-size:30px} .sub{color:#94a3b8;margin-bottom:28px}
    .card{background:#171d26;border:1px solid #2a3544;border-radius:14px;padding:20px;margin:18px 0;box-shadow:0 10px 30px #0004}
    .top{display:flex;align-items:flex-start;justify-content:space-between;gap:16px}.id{font-size:21px;font-weight:700}.meta,.muted{color:#94a3b8}.pill{display:inline-block;padding:3px 9px;border-radius:999px;background:#243041;margin-right:6px;font-size:12px}.on{background:#113c2e;color:#86efac}.off{background:#3b2630;color:#fda4af}
    button{background:#2563eb;color:white;border:0;border-radius:8px;padding:8px 12px;margin-left:6px;cursor:pointer}button.alt{background:#334155}button.warn{background:#9f1239}
    table{width:100%;border-collapse:collapse;margin-top:14px;font-size:13px}th,td{text-align:left;padding:8px;border-top:1px solid #273241}th{color:#94a3b8}.ok{color:#86efac}.bad{color:#fda4af}.empty{color:#fcd34d}code{color:#c4b5fd}
  </style>
</head>
<body>
  <h1>Connector Host</h1>
  <div class="sub">Repo <code>{{.Repo}}</code> · Writer <code>{{.KCURL}}</code></div>
  {{range .Rows}}
  <section class="card" data-connector="{{.Info.Manifest.Metadata.ID}}">
    <div class="top"><div><div class="id">{{.Info.Manifest.Metadata.ID}}</div><div class="meta">{{.Info.Manifest.Metadata.Description}}</div></div>
      <div><span class="pill {{if .Info.State.Active}}on{{else}}off{{end}}">{{if .Info.State.Active}}ACTIVE{{else}}PAUSED{{end}}</span>
        <button class="alt" onclick="act('{{.Info.Manifest.Metadata.ID}}','run?preview=true')">Preview</button><button onclick="act('{{.Info.Manifest.Metadata.ID}}','run')">Run</button>
        {{if .Info.State.Active}}<button class="warn" onclick="act('{{.Info.Manifest.Metadata.ID}}','pause')">Pause</button>{{else}}<button onclick="act('{{.Info.Manifest.Metadata.ID}}','activate')">Activate</button>{{end}}</div></div>
    <p><span class="pill">{{.Info.Manifest.Spec.Maintenance.Representation}}</span><span class="pill">{{join .Info.Manifest.Spec.Target.Scope.Aspects ", "}}</span><span class="pill">generation {{short .Info.Generation}}</span></p>
    <div class="muted">Last success: {{.Info.State.LastSuccessAt}} · Last commit: <code>{{short .Info.State.LastCommit}}</code> · Next: {{.Info.State.NextRunAt}}</div>
    {{if .Info.State.LastError}}<p class="bad">{{.Info.State.LastError}}</p>{{end}}
    <table><thead><tr><th>Run</th><th>Trigger</th><th>Outcome</th><th>Summary</th><th>Commit</th><th>Finished</th></tr></thead><tbody>
      {{range .Runs}}<tr><td><code>{{short .RunID}}</code></td><td>{{.Trigger.Kind}}</td><td class="{{if eq .Outcome "SUCCEEDED"}}ok{{else if eq .Outcome "FAILED"}}bad{{else}}empty{{end}}">{{.Outcome}}</td><td>+{{.Summary.Added}} ~{{.Summary.Updated}} -{{.Summary.Removed}} ={{.Summary.Unchanged}}</td><td><code>{{short .TargetCommit}}</code></td><td>{{.FinishedAt}}</td></tr>{{end}}
    </tbody></table>
  </section>
  {{else}}<p>No connector.yaml found under <code>connectors/*/</code>.</p>{{end}}
  <script>
    async function act(id, action){const r=await fetch('/api/connectors/'+encodeURIComponent(id)+'/'+action,{method:'POST'});const body=await r.json();if(!r.ok)alert(JSON.stringify(body));location.reload()}
  </script>
</body></html>`))

func Serve(ctx context.Context, host *Host, listen string) error {
	server := &http.Server{Addr: listen, Handler: host.HTTPHandler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	go host.ServeScheduler(ctx)
	err := server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return fmt.Errorf("connector host serve: %w", err)
}
