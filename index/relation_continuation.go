package index

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"

	"kc/kernel"
	"kc/retrieval"
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

func encodeRelationContinuation(state relationContinuation) string {
	body, _ := json.Marshal(state)
	mac := hmac.New(sha256.New, relationContinuationKey[:])
	_, _ = mac.Write(body)
	return base64.RawURLEncoding.EncodeToString(append(body, mac.Sum(nil)...))
}

func decodeRelationContinuation(token string, repository kernel.RepositoryID, basis kernel.CommitID, query retrieval.RelationQuery, generation string) (string, error) {
	signed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(signed) <= sha256.Size {
		return "", invalidRelationContinuation()
	}
	body, signature := signed[:len(signed)-sha256.Size], signed[len(signed)-sha256.Size:]
	mac := hmac.New(sha256.New, relationContinuationKey[:])
	_, _ = mac.Write(body)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return "", invalidRelationContinuation()
	}
	var state relationContinuation
	if err := json.Unmarshal(body, &state); err != nil || state.Position == "" ||
		state.Repository != repository || state.Basis != basis ||
		state.Query != retrieval.RelationQueryDigest(query) || state.Generation != generation {
		return "", invalidRelationContinuation()
	}
	return state.Position, nil
}

func invalidRelationContinuation() error {
	return kernel.Fail(kernel.ErrPreconditionFailed, "relation continuation does not match repository, fixed basis, query, or provider generation")
}
