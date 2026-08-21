package hook_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"kc/hook"
	"kc/internal/testkit"
	"kc/kernel"
)

var errBoom = errors.New("boom")

func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestValidateBinding(t *testing.T) {
	err := hook.ValidateBinding(hook.Binding{On: "put", Phase: hook.PhasePre, URL: "http://example"})
	testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)
	err = hook.ValidateBinding(hook.Binding{On: "read", Phase: hook.PhasePost, Run: "x.sh"})
	testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)
	if err := hook.ValidateBinding(hook.Binding{On: "put", Phase: hook.PhasePre, Run: "x.sh"}); err != nil {
		t.Fatal(err)
	}
}

func TestPreExecDeny(t *testing.T) {
	home := testkit.TempDir(t)
	writeScript(t, home, "deny.sh", "#!/bin/sh\nexit 1\n")
	file := hook.File{Bindings: []hook.Binding{{
		ID: "hk_1", On: "put", Phase: hook.PhasePre, Repo: "kr://acme/physical", Run: "deny.sh",
	}}}
	if err := hook.Write(home, file); err != nil {
		t.Fatal(err)
	}
	err := hook.Pre(home, hook.Event{Cmd: "put", Repo: "kr://acme/physical"})
	testkit.ExpectCode(t, err, kernel.ErrHookDenied)
}

func TestPreExecAllowAndPostSkipOnEmpty(t *testing.T) {
	home := testkit.TempDir(t)
	writeScript(t, home, "ok.sh", "#!/bin/sh\ncat > captured.json\n")
	file := hook.File{Bindings: []hook.Binding{{
		ID: "hk_1", On: "put", Phase: hook.PhasePre, Repo: "kr://acme/physical", Run: "ok.sh",
	}}}
	if err := hook.Write(home, file); err != nil {
		t.Fatal(err)
	}
	if err := hook.Pre(home, hook.Event{Cmd: "put", Repo: "kr://acme/physical", As: "ingest-bot"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(home, "captured.json"))
	if err != nil {
		t.Fatal(err)
	}
	var event hook.Event
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err, string(raw))
	}
	if event.As != "ingest-bot" || event.Phase != hook.PhasePre {
		t.Fatal(event)
	}
	if err := hook.Post(home, hook.Event{Cmd: "put", Repo: "kr://acme/physical"}); err != nil {
		t.Fatal(err)
	}
}

func TestPostHTTPOutboxOnFailure(t *testing.T) {
	home := testkit.TempDir(t)
	file := hook.File{Bindings: []hook.Binding{{
		ID: "hk_1", On: "define-view", Phase: hook.PhasePost, URL: "http://127.0.0.1:1/nope",
	}}}
	if err := hook.Write(home, file); err != nil {
		t.Fatal(err)
	}
	if err := hook.Post(home, hook.Event{Cmd: "define-view", Catalog: "kr://acme/catalog", GenerationID: "G1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hook.OutboxPath(home)); err != nil {
		t.Fatal("expected outbox", err)
	}
}

func TestPreRunMustStayUnderHome(t *testing.T) {
	home := testkit.TempDir(t)
	file := hook.File{Bindings: []hook.Binding{{
		ID: "hk_1", On: "put", Phase: hook.PhasePre, Run: "../outside.sh",
	}}}
	if err := hook.Write(home, file); err != nil {
		t.Fatal(err)
	}
	err := hook.Pre(home, hook.Event{Cmd: "put"})
	testkit.ExpectCode(t, err, kernel.ErrHookDenied)
}

func TestPreMissingScriptDenied(t *testing.T) {
	home := testkit.TempDir(t)
	file := hook.File{Bindings: []hook.Binding{{
		ID: "hk_1", On: "put", Phase: hook.PhasePre, Run: "missing.sh",
	}}}
	if err := hook.Write(home, file); err != nil {
		t.Fatal(err)
	}
	err := hook.Pre(home, hook.Event{Cmd: "put"})
	testkit.ExpectCode(t, err, kernel.ErrHookDenied)
}

func TestPostHTTPNon2xxOutbox(t *testing.T) {
	home := testkit.TempDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	t.Cleanup(srv.Close)
	file := hook.File{Bindings: []hook.Binding{{
		ID: "hk_1", On: "define-view", Phase: hook.PhasePost, URL: srv.URL,
	}}}
	if err := hook.Write(home, file); err != nil {
		t.Fatal(err)
	}
	if err := hook.Post(home, hook.Event{Cmd: "define-view", GenerationID: "G1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hook.OutboxPath(home)); err != nil {
		t.Fatal(err)
	}
}

func TestPostHTTPRedirectOutbox(t *testing.T) {
	home := testkit.TempDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/nope", http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	file := hook.File{Bindings: []hook.Binding{{
		ID: "hk_1", On: "define-view", Phase: hook.PhasePost, URL: srv.URL,
	}}}
	if err := hook.Write(home, file); err != nil {
		t.Fatal(err)
	}
	if err := hook.Post(home, hook.Event{Cmd: "define-view"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hook.OutboxPath(home)); err != nil {
		t.Fatal(err)
	}
}

func TestFlushOutboxOnLaterPost(t *testing.T) {
	home := testkit.TempDir(t)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(204)
	}))
	t.Cleanup(srv.Close)
	b := hook.Binding{ID: "hk_1", On: "define-view", Phase: hook.PhasePost, URL: srv.URL}
	if err := hook.AppendOutbox(home, b, hook.Event{Cmd: "define-view", GenerationID: "G1"}, errBoom); err != nil {
		t.Fatal(err)
	}
	if err := hook.Post(home, hook.Event{Cmd: "put"}); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatal(hits)
	}
	if _, err := os.Stat(hook.OutboxPath(home)); !os.IsNotExist(err) {
		t.Fatal("outbox should be cleared after flush")
	}
}

func TestPostHTTPOK(t *testing.T) {
	home := testkit.TempDir(t)
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = body
		w.WriteHeader(204)
	}))
	t.Cleanup(srv.Close)
	file := hook.File{Bindings: []hook.Binding{{
		ID: "hk_1", On: "define-view", Phase: hook.PhasePost, URL: srv.URL,
	}}}
	if err := hook.Write(home, file); err != nil {
		t.Fatal(err)
	}
	if err := hook.Post(home, hook.Event{Cmd: "define-view", GenerationID: "G9"}); err != nil {
		t.Fatal(err)
	}
	var event hook.Event
	if err := json.Unmarshal(got, &event); err != nil {
		t.Fatal(err, string(got))
	}
	if event.GenerationID != "G9" || event.Phase != hook.PhasePost {
		t.Fatal(event)
	}
}
