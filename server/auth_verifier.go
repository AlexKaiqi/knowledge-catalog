package server

import (
	"io"
	"kc/identity"
)

func verifierOrigin(origin, provider string) (string, error) {
	return identity.VerifierOrigin(origin, provider)
}
func decodeVerifierUser(body io.Reader, target any, exact bool) error {
	return identity.DecodeVerifierUser(body, target, exact)
}
