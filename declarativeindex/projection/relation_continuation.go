package projection

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	ports "kc/discovery/projection"

	retrieval "kc/discovery"
	"kc/kernel"
)

type relationContinuation struct {
	Repository kernel.RepositoryID `json:"repository"`
	Basis      kernel.CommitID     `json:"basis"`
	Query      kernel.Digest       `json:"query"`
	Generation string              `json:"generation"`
	Position   string              `json:"position"`
}

var relationContinuationKey = func() [32]byte {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		panic("initialize relation continuation key: " + err.Error())
	}
	return key
}()

type legacyRelationSealer struct{}

func (legacyRelationSealer) SealContinuation(body []byte) string {
	mac := hmac.New(sha256.New, relationContinuationKey[:])
	_, _ = mac.Write(body)
	signed := make([]byte, 0, len(body)+sha256.Size)
	signed = append(signed, body...)
	signed = append(signed, mac.Sum(nil)...)
	return base64.RawURLEncoding.EncodeToString(signed)
}
func (legacyRelationSealer) OpenContinuation(token string) ([]byte, error) {
	signed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(signed) <= sha256.Size {
		return nil, invalidRelationContinuation()
	}
	body, signature := signed[:len(signed)-sha256.Size], signed[len(signed)-sha256.Size:]
	mac := hmac.New(sha256.New, relationContinuationKey[:])
	_, _ = mac.Write(body)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return nil, invalidRelationContinuation()
	}
	return body, nil
}

func relationSealer(ctx context.Context, owner any, existing bool) (ports.ContinuationSealer, error) {
	if store, ok := owner.(ports.ContinuationSealStore); ok {
		var sealer ports.ContinuationSealer
		var err error
		if existing {
			sealer, err = store.LoadContinuationSealer(ctx)
		} else {
			sealer, err = store.PrepareContinuationSealer(ctx)
		}
		if err != nil {
			return nil, err
		}
		if sealer == nil {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "provider returned no continuation sealer")
		}
		return sealer, nil
	}
	return legacyRelationSealer{}, nil
}

func openRelationContinuation(sealer ports.ContinuationSealer, token string) (relationContinuation, error) {
	body, err := sealer.OpenContinuation(token)
	if err != nil {
		return relationContinuation{}, err
	}
	var state relationContinuation
	if err := json.Unmarshal(body, &state); err != nil || state.Position == "" || state.Repository == "" || state.Basis == "" || state.Query == "" || state.Generation == "" {
		return relationContinuation{}, invalidRelationContinuation()
	}
	return state, nil
}

func encodeRelationContinuation(sealer ports.ContinuationSealer, state relationContinuation) string {
	body, _ := json.Marshal(state)
	return sealer.SealContinuation(body)
}
func decodeRelationContinuation(sealer ports.ContinuationSealer, token string, repository kernel.RepositoryID, basis kernel.CommitID, query retrieval.RelationQuery, generation string) (string, error) {
	state, err := openRelationContinuation(sealer, token)
	if err != nil {
		return "", err
	}
	if state.Repository != repository || state.Basis != basis || state.Query != retrieval.RelationQueryDigest(query) || state.Generation != generation {
		return "", invalidRelationContinuation()
	}
	return state.Position, nil
}

func invalidRelationContinuation() error {
	return kernel.Fail(kernel.ErrPreconditionFailed, "relation continuation does not match repository, fixed basis, query, or provider generation")
}
