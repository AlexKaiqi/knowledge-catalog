package object

import (
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"kc/kernel"
)

const objectURNPrefix = "urn:kc:object:"

// Reference identifies a source object and selects content with a decoded JSON
// Pointer. Dataset scope and the fixed version are separate request coordinates.
type Reference struct {
	Repository kernel.RepositoryID `json:"repository"`
	Object     ObjectID            `json:"object"`
	Locator    string              `json:"locator,omitempty"`
}

// ReferenceReader reads the selected maintenance units at one fixed version.
// An implementation must plan from coordinates before reading unit bodies.
type ReferenceReader interface {
	ReadReference(Reference, kernel.CommitID) (ObjectValue, error)
}

// ReferencePlanner exposes only maintenance coordinates from immutable
// metadata. It must not read unit bodies to derive authorization scope.
type ReferencePlanner interface {
	PlanReference(Reference, kernel.CommitID) ([]Address, error)
}

// ReferenceUnitsReader returns complete maintenance units selected by a
// reference before value projection. Logical consumers may apply observations
// before selecting a leaf that is absent from the fixed declaration value.
type ReferenceUnitsReader interface {
	ReadReferenceUnits(Reference, kernel.CommitID) ([]Unit, error)
}

// FormatReference encodes each identity component separately; ':' separates
// them, while only the fragment carries the URI representation of the pointer.
func FormatReference(ref Reference) (string, error) {
	if err := validateReferenceIdentity(string(ref.Repository), string(ref.Object)); err != nil {
		return "", err
	}
	if _, err := PointerTokens(ref.Locator); err != nil {
		return "", err
	}
	encode := func(value string) string {
		return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
	}
	raw := objectURNPrefix + encode(string(ref.Repository)) + ":" + encode(string(ref.Object))
	if ref.Locator != "" {
		raw += "#" + (&url.URL{Fragment: ref.Locator}).EscapedFragment()
	}
	return raw, nil
}

func ParseReference(raw string) (Reference, error) {
	identity, fragment, _ := strings.Cut(raw, "#")
	if len(identity) < len(objectURNPrefix) || !strings.EqualFold(identity[:7], "urn:kc:") || identity[7:len(objectURNPrefix)] != "object:" || strings.Contains(fragment, "#") {
		return Reference{}, invalidReference("expected urn:kc:object:<encoded-repo>:<encoded-object>#<pointer>")
	}
	parts := strings.Split(identity[len(objectURNPrefix):], ":")
	if len(parts) != 2 {
		return Reference{}, invalidReference("identity requires exactly two encoded components")
	}
	repository, err := decodeIdentityComponent(parts[0])
	if err != nil {
		return Reference{}, err
	}
	id, err := decodeIdentityComponent(parts[1])
	if err != nil {
		return Reference{}, err
	}
	if err := validateReferenceIdentity(repository, id); err != nil {
		return Reference{}, err
	}
	locator, err := url.PathUnescape(fragment)
	if err != nil {
		return Reference{}, invalidReference("invalid fragment percent encoding")
	}
	if _, err := PointerTokens(locator); err != nil {
		return Reference{}, err
	}
	return Reference{Repository: kernel.RepositoryID(repository), Object: ObjectID(id), Locator: locator}, nil
}

func decodeIdentityComponent(raw string) (string, error) {
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return "", invalidReference("invalid identity percent encoding")
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] == '%' {
			i += 2 // PathUnescape has validated the complete escape.
			continue
		}
		if !identityUnreserved(raw[i]) {
			return "", invalidReference("identity delimiters and Unicode must be percent encoded")
		}
	}
	return decoded, nil
}

func identityUnreserved(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-._~", rune(c))
}

func validateReferenceIdentity(repository, id string) error {
	for _, component := range []string{repository, id} {
		if strings.TrimSpace(component) == "" || !utf8.ValidString(component) {
			return invalidReference("repository and object identity must be nonempty UTF-8")
		}
		for _, c := range component {
			if unicode.IsControl(c) {
				return invalidReference("identity cannot contain control characters")
			}
		}
	}
	return nil
}

func invalidReference(message string) error {
	return kernel.Fail(kernel.ErrUsageInvalid, "object reference: %s", message)
}
