package testkit

import "strings"

// MarkdownBody extracts the markdown `body` field from a unit value or from
// an assembled object whose readme Aspect carries it.
func MarkdownBody(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, strings.TrimSpace(typed) != ""
	case map[string]any:
		if raw, ok := typed["body"].(string); ok && strings.TrimSpace(raw) != "" {
			return raw, true
		}
		if nested, ok := typed["readme"]; ok {
			return MarkdownBody(nested)
		}
	}
	return "", false
}
