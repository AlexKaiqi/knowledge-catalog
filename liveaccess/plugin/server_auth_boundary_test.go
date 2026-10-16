package plugin_test

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	kcclient "kc/client"
	"kc/internal/bindingfixture"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/server"
)

// U16: selected targets, redirects and source reflections cross the actual
// authenticated public READ, including local failure and independent evidence.
func TestServerLiveAccessCredentialBoundariesPreserveSiblingsAndSafeEvidence(t *testing.T) {
	var otherCalls atomic.Int64
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherCalls.Add(1)
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Tai-Identity") != "" {
			t.Error("inapplicable or redirected target received proof")
		}
		if r.Header.Get("X-Resource-Principal") != "alice" {
			t.Error("untrusted target lost the unproven principal")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"value": 99}, "basis": objectmodel.ObservationBasis{BindingGeneration: "other", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "other-99", ObservedAt: "2026-10-14T12:00:00Z"}})
	}))
	t.Cleanup(other.Close)
	var mode atomic.Value
	mode.Store("normal")
	var calls atomic.Int64
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer alice-proof" || r.Header.Get("X-Resource-Principal") != "alice" {
			t.Error("appropriate target lost current verified proof")
		}
		var request struct {
			Binding struct {
				Address objectmodel.Address `json:"address"`
			} `json:"binding"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		basis := objectmodel.ObservationBasis{BindingGeneration: "source", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "source-17", ObservedAt: "2026-10-14T12:00:00Z"}
		var value any = map[string]any{"value": 17}
		if request.Binding.Address.AspectName == "empty" {
			value = nil
		}
		if request.Binding.Address.AspectName == "broken" {
			switch mode.Load().(string) {
			case "redirect":
				http.Redirect(w, r, other.URL+"/v1/access?reflection=alice-proof", http.StatusTemporaryRedirect)
				return
			case "value":
				value = map[string]any{"value": r.Header.Get("Authorization")}
			case "basis":
				basis.SourceRevision = r.Header.Get("Authorization")
			case "error":
				w.WriteHeader(http.StatusServiceUnavailable)
				// The HTTP decoder must sanitize the decoded credential, not
				// merely remove its unescaped spelling from response bytes.
				_, _ = w.Write([]byte(`{"error":{"message":"source refused Bearer \u0061lice-proof"}}`))
				return
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": value, "basis": basis})
	}))
	t.Cleanup(source.Close)
	f := newAcceptanceFixture(t, server.HTTPServerOptions{Authenticator: acceptanceAuthenticator})
	f.stop(t)
	f.config.LiveAccess.CredentialOrigins = []string{source.URL}
	f.writeConfig(t)
	f.start(t)
	f.publish(t, acceptanceR1, source.URL)
	changeOrigin := func(origin string) {
		f.commit(t, acceptanceR1, []objectmodel.Operation{bindingfixture.PutDescriptor(acceptanceDescriptor(acceptanceR1, "broken"), acceptanceBody(acceptanceR1, origin))})
	}
	assertFailure := func(t *testing.T, requestID string, code kernel.ErrorCode) {
		t.Helper()
		row, err := f.read(t, "alice", requestID, kcclient.ObjectReadRequest{Reference: acceptanceReference(t, acceptanceR1, ""), Repository: string(acceptanceR1), Commit: string(f.commits[acceptanceR1])})
		if err != nil {
			t.Fatalf("source boundary rejected unrelated siblings: %#v %v", row, err)
		}
		assertAcceptanceMixed(t, *row, 17, code)
		entries, err := f.audit(t, "auditor", requestID)
		if err != nil || len(entries) != 1 || entries[0].Results == nil || len(entries[0].Results.Units) != 4 || entries[0].ResultFailure != nil {
			t.Fatalf("safe evidence lost original local failure: %#v %v", entries, err)
		}
		for _, output := range []any{row, entries} {
			raw, err := json.Marshal(output)
			if err != nil || strings.Contains(string(raw), "alice-proof") {
				t.Fatalf("caller proof entered public result/audit: %s %v", raw, err)
			}
		}
	}
	// An initially untrusted declaration and a later changed declaration use
	// the same deployment policy: the source is called, never with the proof.
	assertProofFree := func(requestID string) {
		t.Helper()
		row, err := f.read(t, "alice", requestID, kcclient.ObjectReadRequest{Reference: acceptanceReference(t, acceptanceR1, ""), Repository: string(acceptanceR1), Commit: string(f.commits[acceptanceR1])})
		if err != nil || len(row.Failures) != 0 || len(row.Observations) != 4 {
			t.Fatalf("untrusted target was not called without proof: %#v %v", row, err)
		}
		if broken := row.Value.(map[string]any)["broken"]; !reflect.DeepEqual(broken, map[string]any{"value": float64(99)}) {
			t.Fatalf("untrusted target value not delivered at its address: %#v", broken)
		}
	}
	changeOrigin(other.URL)
	assertProofFree("credential-initial-target")
	changeOrigin(source.URL)
	row, err := f.read(t, "alice", "credential-appropriate-target", kcclient.ObjectReadRequest{Reference: acceptanceReference(t, acceptanceR1, ""), Repository: string(acceptanceR1), Commit: string(f.commits[acceptanceR1])})
	if err != nil || len(row.Failures) != 0 || len(row.Observations) != 4 {
		t.Fatalf("applicable target did not adopt actual result: %#v %v", row, err)
	}
	changeOrigin(other.URL)
	assertProofFree("credential-changed-target")
	if otherCalls.Load() != 2 || calls.Load() != 10 {
		t.Fatalf("untrusted target not called once per read or sibling lookup lost: trusted=%d other=%d", calls.Load(), otherCalls.Load())
	}
	changeOrigin(source.URL)
	// Even another independently trusted target cannot receive proof via 307.
	f.stop(t)
	f.config.LiveAccess.CredentialOrigins = []string{source.URL, other.URL}
	f.writeConfig(t)
	f.start(t)
	for _, test := range []struct {
		mode string
		code kernel.ErrorCode
	}{{"redirect", kernel.ErrForbidden}, {"value", kernel.ErrCapabilityUnsatisfied}, {"basis", kernel.ErrCapabilityUnsatisfied}, {"error", kernel.ErrTemporaryUnavailable}} {
		t.Run(test.mode, func(t *testing.T) {
			mode.Store(test.mode)
			assertFailure(t, "credential-"+test.mode, test.code)
		})
	}
	if otherCalls.Load() != 2 {
		t.Fatal("public read followed cross-origin redirect with current proof")
	}
	// Read the actual durable audit/process/retained-payload files, not a fake
	// collector. Correlation survives while reflected credentials do not.
	err = filepath.WalkDir(f.config.StateDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		raw, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(raw), "alice-proof") {
			t.Errorf("caller proof persisted in durable evidence %s", path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
