package cli

import (
	"encoding/json"
	"fmt"
	"net/http"

	"kc/kernel"
)

func writeInvoke(w http.ResponseWriter, result RunResult) {
	code := http.StatusOK
	if result.Status != 0 {
		code = http.StatusBadRequest
		var payload map[string]any
		if json.Unmarshal([]byte(result.Stdout), &payload) == nil {
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
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(result.Stdout))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(jsonOut(value)))
}
