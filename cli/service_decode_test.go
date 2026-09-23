package cli

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDecodeServiceRequestAcceptsGzipBody pins the server side of transport
// compression: a request marked Content-Encoding: gzip decodes transparently.
func TestDecodeServiceRequestAcceptsGzipBody(t *testing.T) {
	payload, err := json.Marshal(map[string]any{"repository": "kr://kaiqidong/x", "commandId": "c1"})
	if err != nil {
		t.Fatal(err)
	}
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/writer/v1/repositories/kr://kaiqidong/x/commits", &compressed)
	request.Header.Set("Content-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	var target struct {
		Repository string `json:"repository"`
		CommandID  string `json:"commandId"`
	}
	if !decodeServiceRequest(recorder, request, &target) {
		t.Fatalf("gzip request rejected: %s", recorder.Body.String())
	}
	if target.Repository != "kr://kaiqidong/x" || target.CommandID != "c1" {
		t.Fatalf("decoded payload mismatch: %#v", target)
	}
}

// TestDecodeServiceRequestEnforcesLimitOnDecompressedStream pins the guard: the
// size cap applies to the DECOMPRESSED bytes, so a small gzip bomb cannot
// bypass maxServiceRequestBytes.
func TestDecodeServiceRequestEnforcesLimitOnDecompressedStream(t *testing.T) {
	huge := `{"repository":"` + strings.Repeat("k", maxServiceRequestBytes+1024) + `"}`
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write([]byte(huge)); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/x", &compressed)
	request.Header.Set("Content-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	var target map[string]any
	if decodeServiceRequest(recorder, request, &target) {
		t.Fatal("oversized decompressed body was accepted")
	}
	if recorder.Code != 400 {
		t.Fatalf("oversized gzip body returned HTTP %d", recorder.Code)
	}
}

// TestDecodeServiceRequestRejectsBrokenGzip keeps a mislabeled body fail-closed
// instead of decoding garbage.
func TestDecodeServiceRequestRejectsBrokenGzip(t *testing.T) {
	request := httptest.NewRequest("POST", "/x", strings.NewReader("not gzip at all"))
	request.Header.Set("Content-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	var target map[string]any
	if decodeServiceRequest(recorder, request, &target) {
		t.Fatal("broken gzip body was accepted")
	}
	if recorder.Code != 400 {
		t.Fatalf("broken gzip body returned HTTP %d", recorder.Code)
	}
}

// TestDecodeServiceRequestBodyCommitLimitAcceptsLargeChangeSet pins the
// commit-route cap semantics: a ChangeSet larger than the 8 MiB per-route
// service default decodes under the commit cap and is rejected under it.
func TestDecodeServiceRequestBodyCommitLimitAcceptsLargeChangeSet(t *testing.T) {
	big := `{"commandId":"big-1","changeSet":{"targetRepository":"kr://kaiqidong/big","targetRef":"refs/heads/main","operations":[{"op":"PUT","address":{"kind":"Entity","objectId":"obj/big"},"value":{"body":"` + strings.Repeat("k", maxServiceRequestBytes+4096) + `"}}]}}`
	newRequest := func() *http.Request {
		return httptest.NewRequest("POST", "/writer/v1/repositories/kr://kaiqidong/big/commits", strings.NewReader(big))
	}
	recorder := httptest.NewRecorder()
	var accepted writerCommitRequest
	if !decodeServiceRequestBody(recorder, newRequest(), &accepted, maxCommitRequestBytes()) {
		t.Fatalf("commit cap rejected a legitimate scale ChangeSet: %s", recorder.Body.String())
	}
	if accepted.CommandID != "big-1" || accepted.ChangeSet.TargetRepository != "kr://kaiqidong/big" {
		t.Fatalf("decoded payload mismatch: %#v", accepted.CommandID)
	}
	recorder = httptest.NewRecorder()
	var rejected map[string]any
	if decodeServiceRequestBody(recorder, newRequest(), &rejected, maxServiceRequestBytes) {
		t.Fatal("per-route service default accepted an oversized commit body")
	}
	if recorder.Code != 400 {
		t.Fatalf("oversized body under service default returned HTTP %d", recorder.Code)
	}
	if maxCommitRequestBytes() <= maxServiceRequestBytes {
		t.Fatalf("commit cap %d does not exceed the service default %d", maxCommitRequestBytes(), maxServiceRequestBytes)
	}
}

// TestMaxCommitRequestBytesOverride pins the commit-route body cap: 512 MiB by
// default (about twice the measured 228 MB scale ChangeSet), overridable per
// deployment via KC_MAX_COMMIT_REQUEST_BYTES.
func TestMaxCommitRequestBytesOverride(t *testing.T) {
	if got := maxCommitRequestBytes(); got != defaultMaxCommitRequestBytes {
		t.Fatalf("default commit cap = %d, want %d", got, defaultMaxCommitRequestBytes)
	}
	t.Setenv("KC_MAX_COMMIT_REQUEST_BYTES", "32MiB")
	if got := maxCommitRequestBytes(); got != 32<<20 {
		t.Fatalf("override commit cap = %d, want %d", got, 32<<20)
	}
	t.Setenv("KC_MAX_COMMIT_REQUEST_BYTES", "nonsense")
	if got := maxCommitRequestBytes(); got != defaultMaxCommitRequestBytes {
		t.Fatalf("invalid override fell back wrong: %d", got)
	}
}
