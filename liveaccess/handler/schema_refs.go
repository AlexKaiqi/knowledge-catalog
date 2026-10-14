package handler

import (
	"fmt"
	"strings"
)

func interfaceSchemaRefs(raw any) ([]string, error) {
	var refs []string
	switch values := raw.(type) {
	case nil:
		return []string{}, nil
	case []string:
		refs = append(refs, values...)
	case []any:
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("schema reference must be a string")
			}
			refs = append(refs, text)
		}
	default:
		return nil, fmt.Errorf("schema references must be a list")
	}
	for i, ref := range refs {
		if strings.TrimSpace(ref) == "" {
			return nil, fmt.Errorf("schema reference must not be empty")
		}
		refs[i] = strings.ToLower(strings.TrimSpace(ref))
	}
	return refs, nil
}
