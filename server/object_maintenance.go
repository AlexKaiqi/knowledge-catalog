package server

import (
	"encoding/json"
	"net/http"

	inv "kc/invocation"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
)

// The transport preserves the reference as body data. Object's maintenance
// service resolves its scope after the shared current authorization boundary.
type objectMaintenanceRequest struct {
	Reference   string                          `json:"reference"`
	CommandID   string                          `json:"commandId"`
	Ref         string                          `json:"ref,omitempty"`
	Base        string                          `json:"base,omitempty"`
	Expected    string                          `json:"expected,omitempty"`
	Value       json.RawMessage                 `json:"value"`
	Kind        string                          `json:"kind,omitempty"`
	SchemaRef   string                          `json:"schemaRef,omitempty"`
	PathHint    string                          `json:"pathHint,omitempty"`
	ValueSource *objectmodel.ValueSource        `json:"valueSource,omitempty"`
	IfAbsent    bool                            `json:"ifAbsent,omitempty"`
	IfDigest    string                          `json:"ifDigest,omitempty"`
	Message     string                          `json:"message,omitempty"`
	Provenance  *objectmodel.ProvenanceEnvelope `json:"provenance,omitempty"`
}

func (request objectMaintenanceRequest) flags() map[string]FlagValue {
	flags := compactFlags(map[string]FlagValue{
		"reference": request.Reference, "command-id": request.CommandID,
		"ref": request.Ref, "base": request.Base, "expected": request.Expected,
		"kind": request.Kind, "schema-ref": request.SchemaRef, "path-hint": request.PathHint,
		"if-digest": request.IfDigest, "message": request.Message,
	})
	if len(request.Value) > 0 {
		flags["value"] = string(request.Value)
	}
	if request.IfAbsent {
		flags["if-absent"] = true
	}
	if request.ValueSource != nil {
		raw, _ := json.Marshal(request.ValueSource)
		flags["value-source"] = string(raw)
	}
	if request.Provenance != nil {
		flags["_object-provenance"] = request.Provenance
	}
	return flags
}

func requireObjectReference(w http.ResponseWriter, raw string) bool {
	if _, err := objectmodel.ParseReference(raw); err != nil {
		invoke.WriteInvoke(w, inv.ErrorResult(err))
		return false
	}
	return true
}

func (f *httpFacade) knowledgePut(w http.ResponseWriter, r *http.Request) {
	var request objectMaintenanceRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	if !requireObjectReference(w, request.Reference) {
		return
	}
	if len(request.Value) == 0 {
		invoke.WriteInvoke(w, inv.ErrorResult(kernel.Fail(kernel.ErrUsageInvalid, "PUT requires value; JSON null is a valid value")))
		return
	}
	f.executeTyped(w, r, "writer-put", "writer.commit", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbPut}, request.flags())
}

func (f *httpFacade) knowledgeRemove(w http.ResponseWriter, r *http.Request) {
	var request objectMaintenanceRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	if !requireObjectReference(w, request.Reference) {
		return
	}
	f.executeTyped(w, r, "writer-remove", "writer.commit", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbRemove}, request.flags())
}
