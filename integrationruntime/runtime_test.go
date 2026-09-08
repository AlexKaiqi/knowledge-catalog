package integrationruntime

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kc/connector"
	"kc/kernel"
	"kc/knowledge"
)

func TestTickHonorsSchedulePauseAndArtifactIntegrity(t *testing.T) {
	runtime, writer, manifest := runtimeFixture(t, `{"desired":[],"sourceRefs":["source://empty"],"cursor":"one"}`)
	rows, err := runtime.Tick(context.Background())
	if err != nil || len(rows) != 1 {
		t.Fatalf("first tick: %#v %v", rows, err)
	}
	rows, err = runtime.Tick(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatal("tick ignored the persisted interval")
	}
	if _, err := runtime.Pause(context.Background(), manifest.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := runtime.edit(manifest.ID, func(s *state) error { s.NextRunAt = time.Now().Add(-time.Minute); return nil }); err != nil {
		t.Fatal(err)
	}
	rows, err = runtime.Tick(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatal("scheduler executed a paused integration")
	}
	if _, err := runtime.Pause(context.Background(), manifest.ID, false); err != nil {
		t.Fatal(err)
	}
	artifact, err := runtime.artifact(manifest.ArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(artifact.Executable, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifact.Executable, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Run(context.Background(), manifest.ID); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("changed artifact ran: %v", err)
	}
	if writer.calls != 0 {
		t.Fatal("integrity failure published")
	}
}

func TestCredentialsStayOutsideLedgerAndRotateAtRuntime(t *testing.T) {
	runtime, _, manifest := runtimeFixture(t, `{"desired":[],"sourceRefs":["source://empty"],"cursor":"one"}`)
	artifact, err := runtime.artifact(manifest.ArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	artifact.CredentialEnv = []string{"SOURCE_TOKEN"}
	if err := runtime.edit(manifest.ID, func(s *state) error { s.Artifact = artifact; return nil }); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SOURCE_TOKEN", "")
	if _, err := runtime.Run(context.Background(), manifest.ID); kernel.CodeOf(err) != kernel.ErrUnauthenticated {
		t.Fatalf("missing credential did not block collection: %v", err)
	}
	t.Setenv("SOURCE_TOKEN", "new-source-credential")
	if _, err := runtime.Run(context.Background(), manifest.ID); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(runtime.Directory, "runtime.db"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("new-source-credential")) {
		t.Fatal("runtime persisted a credential value")
	}
	if err := os.Remove(filepath.Join(runtime.Directory, "runtime.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := New(runtime.Directory, runtime.Factory); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("lost checkpoint ledger was recreated: %v", err)
	}
}

type testWriter struct {
	calls    int
	fail     bool
	reject   error
	commands []string
	changes  []knowledge.CommitChangeSet
}

func (*testWriter) Head(context.Context, kernel.RepositoryID, string) (kernel.CommitID, error) {
	return "base", nil
}
func (w *testWriter) Commit(_ context.Context, command string, change knowledge.CommitChangeSet) (kernel.CommitID, error) {
	w.calls++
	w.commands = append(w.commands, command)
	w.changes = append(w.changes, change)
	if w.reject != nil {
		err := w.reject
		w.reject = nil
		return "", err
	}
	if w.fail {
		w.fail = false
		return "", kernel.Fail(kernel.ErrTemporaryUnavailable, "lost committed response")
	}
	return "published", nil
}

func runtimeFixture(t *testing.T, output string) (*Runtime, *testWriter, Manifest) {
	t.Helper()
	writer := &testWriter{}
	runtime, err := New(t.TempDir(), func(context.Context, string) (Writer, string, error) { return writer, "kaiqidong", nil })
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	executable := filepath.Join(directory, "collect")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s\\n' '"+output+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Build(context.Background(), ArtifactSpec{ID: "source", Directory: directory, Executable: executable}); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{ID: "notes-sync", ArtifactID: "source", Server: "http://kc.example.test", Repository: "kr://kaiqidong/notes", Mode: connector.ModePatch, Scope: connector.Scope{Aspects: []string{"content"}}, IntervalSeconds: 60}
	if _, err := runtime.Activate(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}
	return runtime, writer, manifest
}

func TestPendingRecoveryKeepsCommandAndCheckpoint(t *testing.T) {
	runtime, writer, manifest := runtimeFixture(t, `{"desired":[{"address":{"kind":"Aspect","objectId":"note/one","aspectName":"content"},"value":{"n":9007199254740993}}],"observed":[],"sourceRefs":["source://batch/1"],"cursor":"cursor-1"}`)
	writer.fail = true
	if _, err := runtime.Run(context.Background(), manifest.ID); err == nil {
		t.Fatal("lost response was hidden")
	}
	status, err := runtime.Status(manifest.ID)
	if err != nil || status.Checkpoint.Cursor != "" || status.PendingCommandID == "" {
		t.Fatalf("checkpoint advanced before receipt: %#v %v", status, err)
	}
	restarted, err := New(runtime.Directory, runtime.Factory)
	if err != nil {
		t.Fatal(err)
	}
	status, err = restarted.Run(context.Background(), manifest.ID)
	if err != nil || status.Checkpoint.Cursor != "cursor-1" || status.Checkpoint.Commit != "published" || status.PendingCommandID != "" {
		t.Fatalf("pending recovery failed: %#v %v", status, err)
	}
	if writer.calls != 2 || writer.commands[0] != writer.commands[1] || kernel.CanonicalDigest(writer.changes[0]) != kernel.CanonicalDigest(writer.changes[1]) {
		t.Fatal("retry changed the reserved Writer request")
	}
}

func TestOutOfScopeCollectionCannotPublishOrAdvance(t *testing.T) {
	runtime, writer, manifest := runtimeFixture(t, `{"desired":[{"address":{"kind":"Aspect","objectId":"note/one","aspectName":"private"},"value":1}],"sourceRefs":["source://batch/1"],"cursor":"forbidden"}`)
	if _, err := runtime.Run(context.Background(), manifest.ID); kernel.CodeOf(err) != kernel.ErrScopeDenied {
		t.Fatalf("scope escape: %v", err)
	}
	status, err := runtime.Status(manifest.ID)
	if err != nil || writer.calls != 0 || status.Checkpoint.Cursor != "" {
		t.Fatal("scope failure had publication effects")
	}
}

func TestEmptyPreviewAndPauseDoNotCommit(t *testing.T) {
	runtime, writer, manifest := runtimeFixture(t, `{"desired":[],"sourceRefs":["source://empty"],"cursor":"empty"}`)
	status, err := runtime.Run(context.Background(), manifest.ID)
	if err != nil || writer.calls != 0 || status.Checkpoint.Cursor != "empty" || status.Phase != "UNCHANGED" {
		t.Fatalf("empty preview: %#v %v", status, err)
	}
	if _, err := runtime.Pause(context.Background(), manifest.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Run(context.Background(), manifest.ID); err == nil {
		t.Fatal("paused runtime executed")
	}
	runtime.Factory = func(context.Context, string) (Writer, string, error) { return nil, "", errors.New("login expired") }
	if _, err := runtime.Status(manifest.ID); err != nil {
		t.Fatal("offline status lost its last durable result")
	}
}

func TestRebuildRepairsInterruptedArtifactCopy(t *testing.T) {
	runtime, _, manifest := runtimeFixture(t, `{"desired":[],"sourceRefs":["source://empty"]}`)
	artifact, err := runtime.artifact(manifest.ArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(artifact.Executable)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	executable := filepath.Join(directory, "collect")
	if err := os.WriteFile(executable, original, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(artifact.Executable, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifact.Executable, []byte("partial artifact"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Build(context.Background(), ArtifactSpec{ID: manifest.ArtifactID, Directory: directory, Executable: executable}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Run(context.Background(), manifest.ID); err != nil {
		t.Fatalf("explicit rebuild did not restore immutable artifact: %v", err)
	}
}

func TestCollectorReceivesSessionLocationAndPinnedServerWithoutTokens(t *testing.T) {
	runtime, _, manifest := runtimeFixture(t, `{"desired":[],"sourceRefs":["source://empty"]}`)
	t.Setenv("KC_CONFIG_DIR", "/private/kc-config")
	t.Setenv("KC_SERVER_URL", "https://wrong-server.example")
	t.Setenv("KC_AUTH_TOKEN", "must-not-inherit-user-token")
	t.Setenv("KC_SERVICE_CLIENT_SECRET", "must-not-inherit-app-secret")
	directory := t.TempDir()
	executable := filepath.Join(directory, "collect")
	script := `#!/bin/sh
[ "$KC_CONFIG_DIR" = /private/kc-config ] || exit 10
[ "$KC_SERVER_URL" = http://kc.example.test ] || exit 11
[ -n "$HOME" ] || exit 12
[ -z "$KC_AUTH_TOKEN" ] || exit 13
[ -z "$KC_SERVICE_CLIENT_SECRET" ] || exit 14
printf '%s\n' '{"desired":[],"sourceRefs":["source://empty"],"cursor":"same-login"}'
`
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Build(context.Background(), ArtifactSpec{ID: manifest.ArtifactID, Directory: directory, Executable: executable}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Activate(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}
	status, err := runtime.Run(context.Background(), manifest.ID)
	if err != nil || status.Checkpoint.Cursor != "same-login" {
		t.Fatalf("collector could not reuse saved KC login: %#v %v", status, err)
	}
}

func TestArtifactCannotOverrideKCSessionOrRuntimeEnvironment(t *testing.T) {
	runtime, _, manifest := runtimeFixture(t, `{"desired":[],"sourceRefs":["source://empty"]}`)
	artifact, err := runtime.artifact(manifest.ArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"KC_SERVER_URL", "KC_AUTH_TOKEN", "KC_CONFIG_DIR", "HOME", "PATH", "SOURCE\nTOKEN", "3SOURCE"} {
		t.Run(name, func(t *testing.T) {
			_, err := runtime.Build(context.Background(), ArtifactSpec{ID: "bad", Directory: filepath.Dir(artifact.Executable), Executable: artifact.Executable, CredentialEnv: []string{name}})
			if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
				t.Fatalf("accepted reserved or invalid credential reference %q: %v", name, err)
			}
		})
	}
}

func TestIndeterminateWriterErrorsKeepTheOriginalPendingCommand(t *testing.T) {
	for _, rejection := range []error{
		kernel.Fail(kernel.ErrTemporaryUnavailable, "lost response"),
		kernel.Fail(kernel.ErrPreconditionFailed, "durable command outcome unresolved"),
		kernel.Fail(kernel.ErrUsageInvalid, "normalized storage failure"),
		kernel.Fail(kernel.ErrUnauthenticated, "token expired before receipt lookup"),
		kernel.Fail(kernel.ErrForbidden, "permission revoked before receipt lookup"),
		kernel.Fail(kernel.ErrRepositoryArchived, "archived before receipt lookup"),
		kernel.Fail(kernel.ErrIdempotencyConflict, "unknown command owner"),
		errors.New("HTTP 400 without a protocol rejection"),
	} {
		t.Run(rejection.Error(), func(t *testing.T) {
			runtime, writer, manifest := runtimeFixture(t, `{"desired":[{"address":{"kind":"Aspect","objectId":"note/one","aspectName":"content"},"value":1}],"sourceRefs":["source://batch/1"],"cursor":"cursor-1"}`)
			writer.reject = rejection
			if _, err := runtime.Run(context.Background(), manifest.ID); err == nil {
				t.Fatal("Writer failure hidden")
			}
			status, err := runtime.Status(manifest.ID)
			if err != nil {
				t.Fatal(err)
			}
			if status.PendingCommandID == "" || status.Checkpoint.Cursor != "" {
				t.Fatalf("indeterminate result discarded or advanced: %#v", status)
			}
			if _, err := runtime.Activate(context.Background(), manifest); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatal("activation replaced an unknown publication")
			}
			restarted, err := New(runtime.Directory, runtime.Factory)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := restarted.Run(context.Background(), manifest.ID); err != nil {
				t.Fatal(err)
			}
			if writer.calls != 2 || writer.commands[0] != writer.commands[1] || kernel.CanonicalDigest(writer.changes[0]) != kernel.CanonicalDigest(writer.changes[1]) {
				t.Fatal("unknown result was retried under a different command or payload")
			}
		})
	}
}
