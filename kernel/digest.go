package kernel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

func CanonicalDigest(value any) Digest {
	return Digest(hex.EncodeToString(sha256Sum([]byte(stableStringify(value)))))
}

func sha256Sum(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}

func stableStringify(value any) string {
	if value == nil {
		return "null"
	}
	switch v := value.(type) {
	case json.RawMessage:
		var parsed any
		if err := json.Unmarshal(v, &parsed); err == nil {
			return stableStringify(parsed)
		}
		return string(v)
	case []any:
		out := "["
		for i, item := range v {
			if i > 0 {
				out += ","
			}
			out += stableStringify(item)
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
			out += string(kb) + ":" + stableStringify(v[k])
		}
		return out + "}"
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		var parsed any
		if err := json.Unmarshal(b, &parsed); err == nil {
			switch parsed.(type) {
			case map[string]any, []any:
				return stableStringify(parsed)
			}
		}
		return string(b)
	}
}
