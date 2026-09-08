package identity

import (
	"strconv"
	"strings"

	"kc/internal/jsonfile"
	"kc/kernel"
)

func validateLegacyAlias(legacy string, user VerifiedUser) error {
	if err := validateUser(user); err != nil {
		return err
	}
	expected := ""
	switch user.Provider {
	case "gitea":
		n, err := strconv.ParseInt(user.Subject, 10, 64)
		if err != nil || n <= 0 || strconv.FormatInt(n, 10) != user.Subject {
			return kernel.Fail(kernel.ErrUsageInvalid, "legacy Gitea alias requires a positive stable numeric subject")
		}
		expected = "gitea:" + user.Subject
	case "taihu":
		expected = "taihu:" + user.Username
	default:
		return kernel.Fail(kernel.ErrUsageInvalid, "legacy alias supports only Gitea and Taihu human users")
	}
	if legacy != expected {
		return kernel.Fail(kernel.ErrUsageInvalid, "legacy alias does not match the explicitly verified identity")
	}
	return nil
}

// MigrateLegacyAlias is an explicit operator operation. Login never calls it.
// It binds the trusted user, then durably reserves an immutable old principal
// alias for control-plane ownership without rewriting any historical request.
// Neither operation grants access; the application separately migrates only
// current authorization rules and persists its original migration decision.
func MigrateLegacyAlias(path, legacy string, user VerifiedUser) error {
	if err := validateLegacyAlias(legacy, user); err != nil {
		return err
	}
	if err := Bind(path, user); err != nil {
		return err
	}
	return withFile(path, func(path string) error {
		file, err := readFile(path)
		if err != nil {
			return err
		}
		if previous, found := file.LegacyAliases[legacy]; found {
			if previous != user {
				return kernel.Fail(kernel.ErrIdempotencyConflict, "legacy owner alias is already bound to another verified identity")
			}
			return nil
		}
		if file.LegacyAliases == nil {
			file.LegacyAliases = map[string]VerifiedUser{}
		}
		file.LegacyAliases[legacy] = user
		return jsonfile.Write(path, file)
	})
}

// ResolvePrincipal resolves only an explicitly migrated legacy human owner.
// It is an ownership lookup, never an authorization check or IdP inference.
// Ordinary canonical users and non-human names keep their exact identifier.
func ResolvePrincipal(path, principal string) (string, error) {
	if !strings.HasPrefix(principal, "gitea:") && !strings.HasPrefix(principal, "taihu:") {
		return principal, nil
	}
	result := principal
	err := withFile(path, func(path string) error {
		file, err := readFile(path)
		if err != nil {
			return err
		}
		if alias, found := file.LegacyAliases[principal]; found {
			result = alias.Username
		}
		return nil
	})
	return result, err
}
