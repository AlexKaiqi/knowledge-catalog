package identity

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"kc/internal/jsonfile"
	"kc/kernel"
)

// VerifiedUser is constructed only after validating a provider credential.
// Issuer identifies the configured trust domain; Subject is that issuer's
// immutable account identifier. None of these fields are credentials.
type VerifiedUser struct {
	Username string `json:"username"`
	Provider string `json:"provider"`
	Issuer   string `json:"issuer"`
	Subject  string `json:"subject"`
}

const Filename = "identities.json"

type bindingFile struct {
	Version       int                     `json:"version"`
	Bindings      []VerifiedUser          `json:"bindings"`
	LegacyAliases map[string]VerifiedUser `json:"legacyAliases,omitempty"`
}

var fileLocks sync.Map

func withFile(path string, action func(string) error) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	lock, _ := fileLocks.LoadOrStore(absolute, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	return action(absolute)
}

func validateUser(user VerifiedUser) error {
	if _, err := CanonicalUsername(user.Username); err != nil {
		return kernel.Fail(kernel.ErrUnauthenticated, "invalid verified username: %v", err)
	}
	for _, value := range []string{user.Provider, user.Issuer, user.Subject} {
		if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\x00\r\n") {
			return kernel.Fail(kernel.ErrUnauthenticated, "verified user requires provider, issuer and stable subject")
		}
	}
	return nil
}

func sameSubject(a, b VerifiedUser) bool {
	return a.Provider == b.Provider && a.Issuer == b.Issuer && a.Subject == b.Subject
}

func readFile(path string) (bindingFile, error) {
	var file bindingFile
	if err := jsonfile.Read(path, &file); err != nil {
		return file, kernel.Fail(kernel.ErrPreconditionFailed, "durable identity bindings are unavailable; initialize explicitly or restore the state volume")
	}
	if file.Version != 1 || file.Bindings == nil {
		return file, kernel.Fail(kernel.ErrPreconditionFailed, "durable identity bindings are invalid")
	}
	for i, user := range file.Bindings {
		if err := validateUser(user); err != nil {
			return file, kernel.Fail(kernel.ErrPreconditionFailed, "durable identity binding is invalid")
		}
		for _, previous := range file.Bindings[:i] {
			if user.Username == previous.Username || sameSubject(user, previous) {
				return file, kernel.Fail(kernel.ErrPreconditionFailed, "durable identity bindings contain a duplicate identity")
			}
		}
	}
	for legacy, target := range file.LegacyAliases {
		if err := validateLegacyAlias(legacy, target); err != nil {
			return file, kernel.Fail(kernel.ErrPreconditionFailed, "durable legacy identity alias is invalid")
		}
		found := false
		for _, user := range file.Bindings {
			found = found || user == target
		}
		if !found {
			return file, kernel.Fail(kernel.ErrPreconditionFailed, "durable legacy alias has no matching verified binding")
		}
	}
	return file, nil
}

// Initialize is an explicit deployment operation. An existing file is validated
// and never overwritten, including when it is damaged.
func Initialize(path string) error {
	return withFile(path, func(path string) error {
		if _, err := os.Stat(path); err == nil {
			_, err = readFile(path)
			return err
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return jsonfile.Write(path, bindingFile{Version: 1, Bindings: []VerifiedUser{}})
	})
}

func Validate(path string) error {
	return withFile(path, func(path string) error { _, err := readFile(path); return err })
}

// Bind reserves a username on the first verified login, then requires the exact
// same trust domain and subject forever. Account renames, recycling and IdP
// changes require explicit administrative migration; they never inherit grants.
// Bind does not create a missing file or grant any capability.
func Bind(path string, user VerifiedUser) error {
	if err := validateUser(user); err != nil {
		return err
	}
	return withFile(path, func(path string) error {
		file, err := readFile(path)
		if err != nil {
			return err
		}
		for _, previous := range file.Bindings {
			if sameSubject(user, previous) {
				if previous.Username != user.Username {
					return kernel.Fail(kernel.ErrForbidden, "verified account username changed; explicit identity migration is required")
				}
				return nil
			}
			if previous.Username == user.Username {
				return kernel.Fail(kernel.ErrForbidden, "username is already bound to another verified account; explicit identity migration is required")
			}
		}
		file.Bindings = append(file.Bindings, user)
		sort.Slice(file.Bindings, func(i, j int) bool { return file.Bindings[i].Username < file.Bindings[j].Username })
		if err := jsonfile.Write(path, file); err != nil {
			return fmt.Errorf("persist identity binding: %w", err)
		}
		return nil
	})
}
