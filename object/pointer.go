package object

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"kc/kernel"
)

// PointerTokens decodes RFC 6901 tokens once, without URI decoding. Callers
// handling a URI fragment must decode its percent escapes before this function.
func PointerTokens(locator string) ([]string, error) {
	if !utf8.ValidString(locator) || locator != "" && !strings.HasPrefix(locator, "/") {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "locator must be an empty or slash-prefixed JSON Pointer")
	}
	if locator == "" {
		return nil, nil
	}
	tokens := strings.Split(locator[1:], "/")
	for i, token := range tokens {
		var decoded strings.Builder
		for j := 0; j < len(token); j++ {
			if token[j] != '~' {
				decoded.WriteByte(token[j])
				continue
			}
			j++
			if j >= len(token) || token[j] != '0' && token[j] != '1' {
				return nil, kernel.Fail(kernel.ErrUsageInvalid, "locator has an invalid JSON Pointer escape")
			}
			if token[j] == '0' {
				decoded.WriteByte('~')
			} else {
				decoded.WriteByte('/')
			}
		}
		tokens[i] = decoded.String()
	}
	return tokens, nil
}

// SelectPointer distinguishes a present JSON null from a nonexistent node.
func SelectPointer(value any, locator string) (any, error) {
	tokens, err := PointerTokens(locator)
	if err != nil {
		return nil, err
	}
	for _, token := range tokens {
		switch node := value.(type) {
		case map[string]any:
			var exists bool
			value, exists = node[token]
			if !exists {
				return nil, pointerMissing(locator)
			}
		case []any:
			index, err := pointerArrayIndex(token, len(node), locator)
			if err != nil {
				return nil, err
			}
			value = node[index]
		default:
			return nil, pointerMissing(locator)
		}
	}
	return value, nil
}

func pointerArrayIndex(token string, length int, locator string) (int, error) {
	if token == "-" {
		return 0, pointerMissing(locator)
	}
	if token == "" || len(token) > 1 && token[0] == '0' {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "locator has an invalid array index")
	}
	for _, c := range token {
		if c < '0' || c > '9' {
			return 0, kernel.Fail(kernel.ErrUsageInvalid, "locator has an invalid array index")
		}
	}
	index, err := strconv.ParseUint(token, 10, 64)
	if err != nil || index >= uint64(length) {
		return 0, pointerMissing(locator)
	}
	return int(index), nil
}

func pointerMissing(locator string) error {
	return kernel.Fail(kernel.ErrObjectRefUnresolved, "locator %q does not select an existing node", locator)
}

// FormatPointer renders decoded tokens as a JSON Pointer; no tokens is root.
func FormatPointer(tokens []string) string {
	parts := make([]string, len(tokens))
	for i, token := range tokens {
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
	}
	if len(parts) == 0 {
		return ""
	}
	return "/" + strings.Join(parts, "/")
}
