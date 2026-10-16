package kernel

import (
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// legacyStableStringify is the concatenating implementation CanonicalDigest
// used before streaming. Digests are persisted in receipts, ledgers and
// command intents, so the streamed form must stay byte-identical to it.
func legacyStableStringify(value any) string {
	if value == nil {
		return "null"
	}
	switch v := value.(type) {
	case json.Number:
		if canonical, ok := canonicalJSONNumber(v.String()); ok {
			return canonical
		}
		return v.String()
	case json.RawMessage:
		var parsed any
		if err := UnmarshalJSON(v, &parsed); err == nil {
			return legacyStableStringify(parsed)
		}
		return string(v)
	case []any:
		out := "["
		for i, item := range v {
			if i > 0 {
				out += ","
			}
			out += legacyStableStringify(item)
		}
		return out + "]"
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := "{"
		for i, k := range keys {
			if i > 0 {
				out += ","
			}
			kb, _ := json.Marshal(k)
			out += string(kb) + ":" + legacyStableStringify(v[k])
		}
		return out + "}"
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		if canonical, ok := canonicalJSONNumber(string(b)); ok {
			return canonical
		}
		var parsed any
		if err := UnmarshalJSON(b, &parsed); err == nil {
			switch parsed.(type) {
			case map[string]any, []any:
				return legacyStableStringify(parsed)
			}
		}
		return string(b)
	}
}

type digestStruct struct {
	Zeta  string            `json:"zeta"`
	Alpha []int             `json:"alpha"`
	Inner map[string]string `json:"inner,omitempty"`
	Skip  string            `json:"-"`
	Ptr   *digestStruct     `json:"ptr,omitempty"`
}

type digestText string

func (t digestText) MarshalText() ([]byte, error) { return []byte("text:" + string(t)), nil }

func TestStreamedCanonicalFormMatchesLegacy(t *testing.T) {
	values := []any{
		nil, true, false, 0, -1, 1.5, 1e21, 12345678901234567, "plain", "html <&> \u2028 \"quoted\"",
		json.Number("1.0"), json.Number("1e2"), json.Number("-0"), json.Number("bogus"),
		json.RawMessage(`{"b":1,"a":[2,1]}`), json.RawMessage(`not json`),
		[]any{}, map[string]any{}, []any{nil, "x", 2, []any{map[string]any{"z": 1, "a": nil}}},
		map[string]any{"é": 1, "a": json.Number("2.50"), "b": map[string]any{"y": []any{"q"}, "x": true}},
		digestStruct{Zeta: "z", Alpha: []int{3, 1}, Inner: map[string]string{"k": "v", "a": "b"}, Skip: "x",
			Ptr: &digestStruct{Zeta: "p"}},
		[]string{"b", "a"}, map[string]int{"b": 2, "a": 1}, map[digestText]int{"k": 1},
		digestText("t"), time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC), make(chan int),
		map[string]any{"raw": json.RawMessage(`[1, 2.0, {"b":2,"a":1}]`), "n": []any{json.Number("10")}},
	}
	for index, value := range values {
		if got, want := stableStringify(value), legacyStableStringify(value); got != want {
			t.Fatalf("value %d (%T): streamed %q != legacy %q", index, value, got, want)
		}
		if got, want := CanonicalDigest(value), Digest(fmt.Sprintf("%x", sha256Sum([]byte(legacyStableStringify(value))))); got != want {
			t.Fatalf("value %d (%T): digest %s != legacy %s", index, value, got, want)
		}
	}
}

// Digesting a whole ChangeSet must stay linear: the concatenating form copied
// the growing prefix on every element and dominated large uploads.
func TestCanonicalDigestLargeArrayIsLinear(t *testing.T) {
	items := make([]any, 0, 20000)
	for index := range 20000 {
		items = append(items, map[string]any{"objectId": fmt.Sprintf("note/%06d", index), "body": strings.Repeat("x", 256)})
	}
	value := map[string]any{"operations": items}
	stableBytes := len(stableStringify(value))
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_ = CanonicalDigest(value)
	runtime.ReadMemStats(&after)
	allocated := after.TotalAlloc - before.TotalAlloc
	// The concatenating form allocated ~117 GB here; 8x the stable form
	// leaves room for race-detector instrumentation.
	if allocated > uint64(stableBytes)*8 {
		t.Fatalf("digest of %d items allocated %d bytes for a %d-byte stable form", len(items), allocated, stableBytes)
	}
}
