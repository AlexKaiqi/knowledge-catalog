package home

import (
	"path/filepath"

	"kc/identity"
)

func initializeIdentityBindings(stateDir string) error {
	return identity.Initialize(filepath.Join(stateDir, identity.Filename))
}

func validateIdentityBindings(stateDir string) error {
	return identity.Validate(filepath.Join(stateDir, identity.Filename))
}
