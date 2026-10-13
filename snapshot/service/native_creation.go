package service

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot"
)

func createNativeRepository(w http.ResponseWriter, r *http.Request, caller identity.HTTPIdentity, resources Resources, target *url.URL, proof http.Header) (any, error) {
	if resources.CreateNativeRepository == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "repository creation requires the configured allocation and registration lifecycle")
	}
	var input struct {
		Name             string `json:"name"`
		StorageNamespace string `json:"storage_namespace"`
		DefaultBranch    string `json:"default_branch"`
		SampleData       bool   `json:"sample_data"`
	}
	if !decodeRequest(w, r, &input) {
		return nil, nil
	}
	defaultBranch := strings.TrimPrefix(string(snapshot.DefaultRef), "refs/heads/")
	if input.StorageNamespace != "" || input.DefaultBranch != "" && input.DefaultBranch != defaultBranch || input.SampleData {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "managed creation uses the configured namespace and default branch; custom namespace, branch and sample data are not accepted")
	}
	id, name, err := resources.CreateNativeRepository(r.Context(), caller, input.Name)
	if err != nil {
		return nil, err
	}
	resources.NativeRepositories = map[string]kernel.RepositoryID{name: id}
	if err := authorizeNative(r.Context(), caller, resources, name, "repository.metadata.read", ""); err != nil {
		return nil, err
	}
	var result map[string]any
	if err := fetchNativeJSON(r.Context(), target, "/repositories/"+url.PathEscape(name), nil, proof, &result); err != nil {
		return nil, err
	}
	writeJSON(w, http.StatusCreated, result)
	return map[string]any{"repository": id, "nativeRepository": name}, nil
}

func (h *handler) decorateNativeConfig(response *http.Response) error {
	if !strings.HasSuffix(response.Request.URL.Path, "/api/v1/config") || response.StatusCode != 200 {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, MaxRequestBytes+1))
	_ = response.Body.Close()
	if err != nil || len(raw) > MaxRequestBytes {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "invalid native configuration response")
	}
	var config map[string]any
	if json.Unmarshal(raw, &config) != nil {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "invalid native configuration JSON")
	}
	decorate := func(storage map[string]any) {
		storage["pre_sign_support_ui"] = false
		storage["snapshot_managed_creation"] = h.config.NativeManagedCreation
	}
	if storage, ok := config["storage_config"].(map[string]any); ok {
		decorate(storage)
	}
	if storage, ok := config["storage_config_list"].([]any); ok {
		for _, item := range storage {
			if row, ok := item.(map[string]any); ok {
				decorate(row)
			}
		}
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return err
	}
	response.Body = io.NopCloser(bytes.NewReader(encoded))
	response.ContentLength = int64(len(encoded))
	response.Header.Set("Content-Length", strconv.Itoa(len(encoded)))
	return nil
}
