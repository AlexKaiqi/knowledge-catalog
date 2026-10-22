package server

import (
	"net/http"

	inv "kc/invocation"
	objectmodel "kc/object"
)

type objectDiffRequest struct {
	Reference string                     `json:"reference"`
	Shape     string                     `json:"shape,omitempty"`
	From      objectmodel.ComparisonView `json:"from"`
	To        objectmodel.ComparisonView `json:"to"`
}

func (f *httpFacade) objectDiff(w http.ResponseWriter, r *http.Request) {
	var request objectDiffRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.executeTyped(w, r, "object-diff", "object.read", inv.Command{}, map[string]FlagValue{"reference": request.Reference, "shape": request.Shape, "from": request.From, "to": request.To})
}
