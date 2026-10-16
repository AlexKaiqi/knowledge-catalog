package kernel

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// CanonicalDigest hashes the stable JSON form of value. The stable form is
// streamed straight into the hasher: nothing proportional to the input is
// concatenated, so digesting a large ChangeSet stays linear in its size.
func CanonicalDigest(value any) Digest {
	h := sha256.New()
	w := bufio.NewWriterSize(h, 32<<10)
	writeStable(w, value)
	_ = w.Flush()
	return Digest(hex.EncodeToString(h.Sum(nil)))
}

func sha256Sum(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}

// stableStringify returns the stable JSON form that CanonicalDigest hashes.
func stableStringify(value any) string {
	var out strings.Builder
	w := bufio.NewWriter(&out)
	writeStable(w, value)
	_ = w.Flush()
	return out.String()
}

// stableWriter is satisfied by *bufio.Writer; writes into a hasher or a
// strings.Builder never fail, so errors are not threaded through.
type stableWriter interface {
	io.Writer
	io.StringWriter
	io.ByteWriter
}

func writeStable(w stableWriter, value any) {
	if value == nil {
		_, _ = w.WriteString("null")
		return
	}
	switch v := value.(type) {
	case json.Number:
		if canonical, ok := canonicalJSONNumber(v.String()); ok {
			_, _ = w.WriteString(canonical)
			return
		}
		_, _ = w.WriteString(v.String())
	case json.RawMessage:
		var parsed any
		if err := UnmarshalJSON(v, &parsed); err == nil {
			writeStable(w, parsed)
			return
		}
		_, _ = w.Write(v)
	case []any:
		_ = w.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				_ = w.WriteByte(',')
			}
			writeStable(w, item)
		}
		_ = w.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		_ = w.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				_ = w.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			_, _ = w.Write(kb)
			_ = w.WriteByte(':')
			writeStable(w, v[k])
		}
		_ = w.WriteByte('}')
	default:
		writeStableScalar(w, v)
	}
}

// writeStableScalar encodes any other value through encoding/json and, when
// that yields a JSON object or array, re-enters the stable form so struct
// field order and map order never leak into the digest.
func writeStableScalar(w stableWriter, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		_, _ = fmt.Fprintf(w, "%v", v)
		return
	}
	if canonical, ok := canonicalJSONNumber(string(b)); ok {
		_, _ = w.WriteString(canonical)
		return
	}
	if len(b) > 0 && (b[0] == '{' || b[0] == '[') {
		var parsed any
		if err := UnmarshalJSON(b, &parsed); err == nil {
			switch parsed.(type) {
			case map[string]any, []any:
				writeStable(w, parsed)
				return
			}
		}
	}
	_, _ = w.Write(b)
}
