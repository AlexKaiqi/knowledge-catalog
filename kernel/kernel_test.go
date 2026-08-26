package kernel

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestCanonicalDigestIgnoresMapOrderAndJSONRepresentation(t *testing.T) {
	a := map[string]any{"b": []any{true, 2.0}, "a": "value"}
	b := json.RawMessage(`{"a":"value","b":[true,2]}`)
	if CanonicalDigest(a) != CanonicalDigest(b) {
		t.Fatalf("equivalent JSON values must have one digest: %s != %s", CanonicalDigest(a), CanonicalDigest(b))
	}
	if CanonicalDigest(a) == CanonicalDigest(map[string]any{"a": "different", "b": []any{true, 2}}) {
		t.Fatal("different values must not share a digest")
	}
}

func TestErrorNormalizationPreservesProtocolCode(t *testing.T) {
	protocol := Fail(ErrNonFastForward, "expected %s", "old")
	if CodeOf(protocol) != ErrNonFastForward || Normalize(protocol) != AsIngress(protocol) {
		t.Fatalf("protocol error was not preserved: %#v", Normalize(protocol))
	}
	plain := Normalize(errors.New("bad flag"))
	if plain.Code != ErrUsageInvalid || plain.Message != "bad flag" {
		t.Fatalf("plain errors must normalize at ingress: %#v", plain)
	}
	envelope := FaultJSON(protocol)
	if envelope["error"] != AsIngress(protocol) {
		t.Fatalf("fault envelope must carry the normalized error: %#v", envelope)
	}
}
