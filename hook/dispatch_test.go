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
	"time"

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
	err := hook.ValidateBinding(hook.Binding{On: "writer.commit", Phase: hook.PhasePre, URL: "http://example"})
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
	err = hook.ValidateBinding(hook.Binding{On: "read", Phase: hook.PhasePost, Run: "x.sh"})
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
	if err := hook.ValidateBinding(hook.Binding{On: "writer.commit", Phase: hook.PhasePre, Run: "x.sh"}); err != nil {
		t.Fatal(err)
	}
}

func TestPreExecDeny(t *testing.T) {
	home := testkit.TempDir(t)
	writeScript(t, home, "deny.sh", "#!/bin/sh\nexit 1\n")
	file := hook.File{Bindings: []hook.Binding{{
		ID: "hk_1", On: "writer.commit", Phase: hook.PhasePre, Repo: "kr://acme/physical", Run: "deny.sh",
	}}}
	if err := hook.Write(home, file); err != nil {
		t.Fatal(err)
	}
	err := hook.Pre(home, hook.Event{Action: "writer.commit", Repo: "kr://acme/physical"})
	testkit.ExpectCode(t, err, kernel.ErrHookDenied)
}

func TestPreExecAllowAndPostSkipOnEmpty(t *testing.T) {
	home := testkit.TempDir(t)
	writeScript(t, home, "ok.sh", "#!/bin/sh\ncat > captured.json\n")
	file := hook.File{Bindings: []hook.Binding{{
		ID: "hk_1", On: "writer.commit", Phase: hook.PhasePre, Repo: "kr://acme/physical", Run: "ok.sh",
	}}}
	if err := hook.Write(home, file); err != nil {
		t.Fatal(err)
	}
	if err := hook.Pre(home, hook.Event{Action: "writer.commit", Repo: "kr://acme/physical", As: "ingest-bot"}); err != nil {
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
	if err := hook.Post(home, hook.Event{Action: "writer.commit", Repo: "kr://acme/physical"}); err != nil {
		t.Fatal(err)
	}
}

func TestPostHTTPOutboxOnFailure(t *testing.T) {
	home := testkit.TempDir(t)
	file := hook.File{Bindings: []hook.Binding{{
		ID: "hk_1", On: "workspace.manage", Phase: hook.PhasePost, URL: "http://127.0.0.1:1/nope",
	}}}
	if err := hook.Write(home, file); err != nil {
		t.Fatal(err)
	}
	if err := hook.Post(home, hook.Event{Action: "workspace.manage", Catalog: "kr://acme/catalog", WorkspaceID: "G1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hook.OutboxPath(home)); err != nil {
		t.Fatal("expected outbox", err)
	}
}

func TestPreRunMustStayUnderHome(t *testing.T) {
	home := testkit.TempDir(t)
	file := hook.File{Bindings: []hook.Binding{{
		ID: "hk_1", On: "writer.commit", Phase: hook.PhasePre, Run: "../outside.sh",
	}}}
	if err := hook.Write(home, file); err != nil {
		t.Fatal(err)
	}
	err := hook.Pre(home, hook.Event{Action: "writer.commit"})
	testkit.ExpectCode(t, err, kernel.ErrHookDenied)
}

func TestPreMissingScriptDenied(t *testing.T) {
	home := testkit.TempDir(t)
	file := hook.File{Bindings: []hook.Binding{{
		ID: "hk_1", On: "writer.commit", Phase: hook.PhasePre, Run: "missing.sh",
	}}}
	if err := hook.Write(home, file); err != nil {
		t.Fatal(err)
	}
	err := hook.Pre(home, hook.Event{Action: "writer.commit"})
	testkit.ExpectCode(t, err, kernel.ErrHookDenied)
}

func TestPostHTTPNon2xxOutbox(t *testing.T) {
	home := testkit.TempDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	t.Cleanup(srv.Close)
	file := hook.File{Bindings: []hook.Binding{{
		ID: "hk_1", On: "workspace.manage", Phase: hook.PhasePost, URL: srv.URL,
	}}}
	if err := hook.Write(home, file); err != nil {
		t.Fatal(err)
	}
	observed := []string{}
	if err := hook.PostObserved(home, hook.Event{Action: "workspace.manage", WorkspaceID: "G1"}, func(phase, transport, outcome string) {
		observed = append(observed, phase+":"+transport+":"+outcome)
	}); err != nil {
		t.Fatal(err)
	}
	if len(observed) != 1 || observed[0] != "post:http:error" {
		t.Fatalf("dispatch observation %#v", observed)
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
		ID: "hk_1", On: "workspace.manage", Phase: hook.PhasePost, URL: srv.URL,
	}}}
	if err := hook.Write(home, file); err != nil {
		t.Fatal(err)
	}
	if err := hook.Post(home, hook.Event{Action: "workspace.manage"}); err != nil {
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
	b := hook.Binding{ID: "hk_1", On: "workspace.manage", Phase: hook.PhasePost, URL: srv.URL}
	if err := hook.AppendOutbox(home, b, hook.Event{Action: "workspace.manage", WorkspaceID: "G1"}, errBoom); err != nil {
		t.Fatal(err)
	}
	observed := []string{}
	if err := hook.PostObserved(home, hook.Event{Action: "writer.commit"}, func(phase, transport, outcome string) {
		observed = append(observed, phase+":"+transport+":"+outcome)
	}); err != nil {
		t.Fatal(err)
	}
	if len(observed) != 1 || observed[0] != "post:outbox:ok" {
		t.Fatalf("outbox observation %#v", observed)
	}
	if hits != 1 {
		t.Fatal(hits)
	}
	if _, err := os.Stat(hook.OutboxPath(home)); !os.IsNotExist(err) {
		t.Fatal("outbox should be cleared after flush")
	}
}

func TestOutboxLockIsScopedPerHome(t *testing.T) {
	homeA := testkit.TempDir(t)
	homeB := testkit.TempDir(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	binding := hook.Binding{ID: "slow", On: "writer.commit", Phase: hook.PhasePost, URL: server.URL}
	if err := hook.AppendOutbox(homeA, binding, hook.Event{Action: "writer.commit"}, errBoom); err != nil {
		t.Fatal(err)
	}
	flushed := make(chan error, 1)
	go func() { flushed <- hook.FlushOutbox(homeA) }()
	<-entered

	appended := make(chan error, 1)
	go func() {
		appended <- hook.AppendOutbox(homeB, binding, hook.Event{Action: "writer.commit"}, errBoom)
	}()
	select {
	case err := <-appended:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("a slow flush in one home blocked another home's outbox")
	}
	close(release)
	if err := <-flushed; err != nil {
		t.Fatal(err)
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
		ID: "hk_1", On: "workspace.manage", Phase: hook.PhasePost, URL: srv.URL,
	}}}
	if err := hook.Write(home, file); err != nil {
		t.Fatal(err)
	}
	if err := hook.Post(home, hook.Event{Action: "workspace.manage", WorkspaceID: "G9"}); err != nil {
		t.Fatal(err)
	}
	var event hook.Event
	if err := json.Unmarshal(got, &event); err != nil {
		t.Fatal(err, string(got))
	}
	if event.WorkspaceID != "G9" || event.Phase != hook.PhasePost {
		t.Fatal(event)
	}
}
