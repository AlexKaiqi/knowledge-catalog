package service

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot/access"
)

func nativeSourceChecks(r *http.Request, caller identity.HTTPIdentity, resources Resources, target *url.URL, proof http.Header, route nativeRoute, values map[string]string) error {
	name := values["repository"]
	if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
		return nil
	}
	if nativeUploadRequest(r) {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, MaxRequestBytes+1))
	if err != nil || len(raw) > MaxRequestBytes {
		return kernel.Fail(kernel.ErrUsageInvalid, "native JSON request exceeds limit")
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	if len(raw) == 0 {
		return nil
	}
	var body map[string]json.RawMessage
	if json.Unmarshal(raw, &body) != nil {
		return kernel.Fail(kernel.ErrUsageInvalid, "native request requires one JSON object")
	}
	if body == nil {
		return kernel.Fail(kernel.ErrUsageInvalid, "native request requires one JSON object")
	}
	if route.action == access.RepositoryImport {
		return nativePhysicalSourceChecks(r, caller, resources, target, proof, route, body)
	}
	if route.path == "/repositories/{repository}/branches/{branch}/objects/copy" {
		ref := values["branch"]
		if source, exists := body["src_ref"]; exists {
			if json.Unmarshal(source, &ref) != nil {
				return kernel.Fail(kernel.ErrUsageInvalid, "src_ref must be a string")
			}
			if ref == "" {
				ref = values["branch"]
			}
		}
		if err := authorizeNative(r.Context(), caller, resources, name, access.RepositoryFileRead, ref); err != nil {
			return err
		}
	}
	for _, field := range []string{"source", "ref", "source_ref", "src_ref", "source_branch", "destination_branch"} {
		value, ok := body[field]
		if !ok {
			continue
		}
		var ref string
		if json.Unmarshal(value, &ref) != nil {
			return kernel.Fail(kernel.ErrUsageInvalid, "native source reference must be a nonempty string")
		}
		if ref == "" {
			ref = values["branch"]
		}
		action := access.RepositoryHistoryRead
		if route.action == access.RepositoryFilesWrite {
			action = access.RepositoryFileRead
		}
		if err := authorizeNative(r.Context(), caller, resources, name, action, ref); err != nil {
			return err
		}
	}
	for _, field := range []string{"source_repository", "src_repository"} {
		value, ok := body[field]
		if !ok {
			continue
		}
		var source string
		if json.Unmarshal(value, &source) != nil || source == "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "native source repository must be a nonempty string")
		}
		if err := authorizeNative(r.Context(), caller, resources, source, access.RepositoryFileRead, ""); err != nil {
			return err
		}
	}
	return nil
}

func nativePullMergeChecks(r *http.Request, caller identity.HTTPIdentity, resources Resources, target *url.URL, proof http.Header, values map[string]string) error {
	name := values["repository"]
	if err := authorizeNative(r.Context(), caller, resources, name, access.RepositoryPullsRead, ""); err != nil {
		return err
	}
	var pull struct {
		Source      string `json:"source_branch"`
		Destination string `json:"destination_branch"`
	}
	if err := fetchNativeJSON(r.Context(), target, "/repositories/"+url.PathEscape(name)+"/pulls/"+url.PathEscape(values["pull"]), nil, proof, &pull); err != nil {
		return err
	}
	if pull.Source == "" || pull.Destination == "" {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "native PR has no source/destination coordinates")
	}
	if err := authorizeNative(r.Context(), caller, resources, name, access.RepositoryHistoryRead, pull.Source); err != nil {
		return err
	}
	return authorizeNative(r.Context(), caller, resources, name, access.RepositoryMerge, pull.Destination)
}
