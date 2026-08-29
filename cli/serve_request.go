package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"kc/kernel"
)

const maxRequestBodyBytes = 8 << 20

func decodeJSONBody(r *http.Request) (map[string]any, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxRequestBodyBytes {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "request body exceeds %d bytes", maxRequestBodyBytes)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return map[string]any{}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("body must be a JSON object")
	}
	return raw, nil
}
