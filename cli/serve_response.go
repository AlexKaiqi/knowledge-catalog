package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"kc/kernel"
)

func writeInvoke(w http.ResponseWriter, result RunResult) {
	code := http.StatusOK
	if result.Status != 0 {
		code = http.StatusBadRequest
		var payload map[string]any
		if json.Unmarshal([]byte(result.Stdout), &payload) == nil {
			if result.Status == 2 && strings.EqualFold(fmt.Sprint(payload["outcome"]), "partial") {
				code = http.StatusMultiStatus
			}
			if errObj, ok := payload["error"].(map[string]any); ok {
				switch kernel.ErrorCode(fmt.Sprint(errObj["code"])) {
				case kernel.ErrUnauthenticated:
					code = http.StatusUnauthorized
				case kernel.ErrForbidden:
					code = http.StatusForbidden
				case kernel.ErrKnowledgeRefUnresolved, kernel.ErrVersionUnresolved:
					code = http.StatusNotFound
				case kernel.ErrTemporaryUnavailable:
					code = http.StatusServiceUnavailable
				case kernel.ErrNonFastForward, kernel.ErrIdempotencyConflict, kernel.ErrPreconditionFailed,
					kernel.ErrEventIDConflict, kernel.ErrCandidateMoved,
					kernel.ErrGateUnsatisfied, kernel.ErrCatalogArchived, kernel.ErrRepositoryArchived:
					code = http.StatusConflict
				}
			}
		}
	}
	// The HTTP facade promises JSON for every verb. CLI-oriented commands such
	// as help intentionally return plain text from Invoke; encode that text as a
	// JSON string instead of sending an invalid body with application/json.
	if code == http.StatusOK && !json.Valid([]byte(result.Stdout)) {
		writeJSON(w, code, strings.TrimSuffix(result.Stdout, "\n"))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(result.Stdout))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(jsonOut(value)))
}
