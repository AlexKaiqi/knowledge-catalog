package retrieval

import (
	"encoding/base64"
	"encoding/json"

	"kc/kernel"
)

type ContinuationState struct {
	Scope      string        `json:"scope"`
	Query      kernel.Digest `json:"query"`
	SearchView kernel.Digest `json:"searchView"`
	Projection kernel.Digest `json:"projection,omitempty"`
	Position   string        `json:"position,omitempty"`
	Member     int           `json:"member,omitempty"`
	Check      kernel.Digest `json:"check"`
}

func SearchQueryDigest(req SearchRequest) kernel.Digest {
	req.Continuation = ""
	req.Limit = 0 // page size may change without changing query identity
	return kernel.CanonicalDigest(req)
}

func SearchViewDigest(searchView SearchView) kernel.Digest { return kernel.CanonicalDigest(searchView) }

func EncodeContinuation(state ContinuationState) string {
	state.Check = ""
	state.Check = kernel.CanonicalDigest(state)
	raw, _ := json.Marshal(state)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func DecodeContinuation(token string) (ContinuationState, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return ContinuationState{}, invalidContinuation()
	}
	var state ContinuationState
	if err := json.Unmarshal(raw, &state); err != nil {
		return ContinuationState{}, invalidContinuation()
	}
	want := state.Check
	state.Check = ""
	if want == "" || kernel.CanonicalDigest(state) != want {
		return ContinuationState{}, invalidContinuation()
	}
	state.Check = want
	return state, nil
}

func invalidContinuation() error {
	return kernel.Fail(kernel.ErrPreconditionFailed, "continuation does not match this SearchView")
}
