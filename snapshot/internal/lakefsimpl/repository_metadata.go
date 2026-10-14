package lakefsimpl

import (
	"encoding/json"
	"net/http"

	"kc/kernel"
)

// ReadRepositoryMetadata reads the repository's current native properties.
// These attributes are independent of immutable Snapshot commit metadata.
func (r *Repository) ReadRepositoryMetadata() (map[string]string, error) {
	var raw map[string]json.RawMessage
	status, _, err := r.client.doJSON(http.MethodGet, r.client.repositoryPath("metadata"), nil, &raw)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs repository metadata returned HTTP %d", status)
	}
	if raw == nil {
		return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs repository metadata omitted its property map")
	}
	metadata := make(map[string]string, len(raw))
	for key, value := range raw {
		var text string
		if len(value) == 0 || value[0] != '"' || json.Unmarshal(value, &text) != nil {
			return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs repository metadata contains a non-string property")
		}
		metadata[key] = text
	}
	return metadata, nil
}

// WriteRepositoryMetadata adds or updates the supplied native property keys.
// It neither replaces unrelated keys nor creates a Snapshot commit.
func (r *Repository) WriteRepositoryMetadata(metadata map[string]string) error {
	if metadata == nil {
		metadata = map[string]string{}
	}
	request := struct {
		Metadata map[string]string `json:"metadata"`
	}{Metadata: metadata}
	status, _, err := r.client.doJSON(http.MethodPost, r.client.repositoryPath("metadata"), request, nil)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs repository metadata update returned HTTP %d", status)
	}
	return nil
}
