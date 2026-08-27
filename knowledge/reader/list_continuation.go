package reader

import (
	"encoding/base64"
	"encoding/json"

	"kc/kernel"
)

type listContinuation struct {
	Version   int           `json:"version"`
	Scope     string        `json:"scope"`
	Basis     kernel.Digest `json:"basis"`
	Member    int           `json:"member,omitempty"`
	Position  string        `json:"position,omitempty"`
	Signature kernel.Digest `json:"signature"`
}

func encodeListContinuation(state listContinuation) string {
	state.Version = 1
	state.Signature = ""
	state.Signature = kernel.CanonicalDigest(state)
	raw, _ := json.Marshal(state)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeListContinuation(token, scope string, basis kernel.Digest) (listContinuation, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return listContinuation{}, invalidListContinuation()
	}
	state := listContinuation{}
	if err := json.Unmarshal(raw, &state); err != nil {
		return listContinuation{}, invalidListContinuation()
	}
	signature := state.Signature
	state.Signature = ""
	if state.Version != 1 || state.Scope != scope || state.Basis != basis || signature != kernel.CanonicalDigest(state) {
		return listContinuation{}, invalidListContinuation()
	}
	state.Signature = signature
	return state, nil
}

func invalidListContinuation() error {
	return kernel.Fail(kernel.ErrPreconditionFailed, "continuation does not match this immutable list basis")
}
