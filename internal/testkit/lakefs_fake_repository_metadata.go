package testkit

import (
	"encoding/json"
	"net/http"
)

func serveLakeFSRepositoryMetadata(w http.ResponseWriter, r *http.Request, repo *lakefsFakeRepo) {
	switch r.Method {
	case http.MethodGet:
		writeLakeFSJSON(w, http.StatusOK, repo.metadata)
	case http.MethodPost:
		var request struct {
			Metadata map[string]json.RawMessage `json:"metadata"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil || request.Metadata == nil {
			writeLakeFSJSON(w, http.StatusBadRequest, map[string]any{"message": "metadata property map required"})
			return
		}
		updates := make(map[string]string, len(request.Metadata))
		for key, raw := range request.Metadata {
			var value string
			if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &value) != nil {
				writeLakeFSJSON(w, http.StatusBadRequest, map[string]any{"message": "metadata properties must be strings"})
				return
			}
			updates[key] = value
		}
		for key, value := range updates {
			repo.metadata[key] = value
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		var request struct {
			Keys []string `json:"keys"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil || request.Keys == nil {
			writeLakeFSJSON(w, http.StatusBadRequest, map[string]any{"message": "metadata keys required"})
			return
		}
		for _, key := range request.Keys {
			delete(repo.metadata, key)
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
