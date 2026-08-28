package snapshot

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"

	"kc/kernel"
)

// DirectoryCursor is transport state for one DirectoryReader generation.
// Position is provider-private; callers may only return the signed token.
type DirectoryCursor struct {
	Commit    kernel.CommitID `json:"commit"`
	Directory string          `json:"directory,omitempty"`
	Position  string          `json:"position,omitempty"`
}

var directoryCursorKey = func() [32]byte {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		panic("initialize directory continuation key: " + err.Error())
	}
	return key
}()

func EncodeDirectoryCursor(cursor DirectoryCursor) string {
	payload, _ := json.Marshal(cursor)
	mac := hmac.New(sha256.New, directoryCursorKey[:])
	_, _ = mac.Write(payload)
	signed := append(append([]byte(nil), payload...), mac.Sum(nil)...)
	return base64.RawURLEncoding.EncodeToString(signed)
}

func DecodeDirectoryCursor(token string, commit kernel.CommitID, directory string) (DirectoryCursor, error) {
	if token == "" {
		return DirectoryCursor{Commit: commit, Directory: directory}, nil
	}
	signed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(signed) <= sha256.Size {
		return DirectoryCursor{}, kernel.Fail(kernel.ErrPreconditionFailed, "directory continuation is invalid")
	}
	payload, signature := signed[:len(signed)-sha256.Size], signed[len(signed)-sha256.Size:]
	mac := hmac.New(sha256.New, directoryCursorKey[:])
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return DirectoryCursor{}, kernel.Fail(kernel.ErrPreconditionFailed, "directory continuation was modified")
	}
	var cursor DirectoryCursor
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.Commit != commit || cursor.Directory != directory {
		return DirectoryCursor{}, kernel.Fail(kernel.ErrPreconditionFailed, "directory continuation does not match the fixed basis")
	}
	return cursor, nil
}
