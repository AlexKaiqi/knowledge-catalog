package plugin_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	kcclient "kc/client"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/server"
)

// U23: the deployment's readTimeout is shared by all external units of one
// public READ; units that do not fit report a timeout at their own address.
func TestServerReadTimeoutIsSharedByExternalUnits(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(150 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		basis := objectmodel.ObservationBasis{BindingGeneration: "source", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "source-17", ObservedAt: "2026-10-14T12:00:00Z"}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"value": 17}, "basis": basis})
	}))
	t.Cleanup(source.Close)
	f := newAcceptanceFixture(t, server.HTTPServerOptions{Authenticator: acceptanceAuthenticator})
	f.stop(t)
	f.config.LiveAccess.CredentialOrigins = []string{source.URL}
	f.config.LiveAccess.LookupTimeout = "5s"
	f.config.LiveAccess.ReadTimeout = "250ms"
	f.writeConfig(t)
	f.start(t)
	f.publish(t, acceptanceR1, source.URL)
	started := time.Now()
	row, err := f.read(t, "alice", "shared-read-timeout", kcclient.ObjectReadRequest{Reference: acceptanceReference(t, acceptanceR1, ""), Repository: string(acceptanceR1), Commit: string(f.commits[acceptanceR1])})
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("timed-out units failed the whole read: %v", err)
	}
	timeouts := 0
	for _, failure := range row.Failures {
		if failure.Code == kernel.ErrTemporaryUnavailable && strings.Contains(failure.Message, "timed out") {
			timeouts++
		}
	}
	if len(row.Observations) == 0 || timeouts == 0 || len(row.Observations)+timeouts != 4 {
		t.Fatalf("want delivered units plus per-address timeouts for all four external units: %#v", row)
	}
	if definition := row.Value.(map[string]any)["definition"]; definition == nil {
		t.Fatalf("static aspect lost: %#v", row.Value)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("read took %v; the shared limit did not bound it", elapsed)
	}
}
