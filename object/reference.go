package object

import (
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"kc/kernel"
)

const objectURNPrefix = "urn:kc:object:"

// Reference identifies an object by its namespaced object_id and selects
// content with a decoded JSON Pointer. The URN never names a repository: the
// request scope (one Repository, a Dataset publication, or every Repository the
// caller may read) decides where the object's aspects are collected. Repository
// is that resolved scope binding, never part of the URN text.
type Reference struct {
	Repository kernel.RepositoryID `json:"repository,omitempty"`
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

// FormatReference renders urn:kc:object:<namespace>/<name>[#pointer]. '/'
// stays literal inside the object_id; every other delimiter is percent encoded.
// The bound Repository is scope, so it is not rendered.
func FormatReference(ref Reference) (string, error) {
	if err := ValidateObjectID(ref.Object); err != nil {
		return "", err
	}
	if _, err := PointerTokens(ref.Locator); err != nil {
		return "", err
	}
	segments := strings.Split(string(ref.Object), "/")
	for i, segment := range segments {
		segments[i] = strings.ReplaceAll(url.QueryEscape(segment), "+", "%20")
	}
	raw := objectURNPrefix + strings.Join(segments, "/")
	if ref.Locator != "" {
		raw += "#" + (&url.URL{Fragment: ref.Locator}).EscapedFragment()
	}
	return raw, nil
}

// ParseReference accepts only the namespaced form. The retired
// urn:kc:object:<repo>:<object> form is rejected rather than reinterpreted.
func ParseReference(raw string) (Reference, error) {
	identity, fragment, _ := strings.Cut(raw, "#")
	if len(identity) < len(objectURNPrefix) || !strings.EqualFold(identity[:7], "urn:kc:") || identity[7:len(objectURNPrefix)] != "object:" || strings.Contains(fragment, "#") {
		return Reference{}, invalidReference("expected urn:kc:object:<namespace>/<name>#<pointer>")
	}
	encoded := identity[len(objectURNPrefix):]
	if strings.Contains(encoded, ":") {
		return Reference{}, invalidReference("repository-qualified URNs are retired; use urn:kc:object:<namespace>/<name> and select the Repository with the request scope")
	}
	segments := strings.Split(encoded, "/")
	for i, segment := range segments {
		decoded, err := decodeIdentityComponent(segment)
		if err != nil {
			return Reference{}, err
		}
		if strings.Contains(decoded, "/") {
			return Reference{}, invalidReference("object_id segments separate with a literal '/'")
		}
		segments[i] = decoded
	}
	id := ObjectID(strings.Join(segments, "/"))
	if err := ValidateObjectID(id); err != nil {
		return Reference{}, err
	}
	locator, err := url.PathUnescape(fragment)
	if err != nil {
		return Reference{}, invalidReference("invalid fragment percent encoding")
	}
	if _, err := PointerTokens(locator); err != nil {
		return Reference{}, err
	}
	return Reference{Object: id, Locator: locator}, nil
}

// ValidateObjectID enforces <namespace>/<name>: a nonempty namespace segment,
// a nonempty name, no empty segments and no control characters.
func ValidateObjectID(id ObjectID) error {
	raw := string(id)
	if strings.TrimSpace(raw) == "" || !utf8.ValidString(raw) {
		return invalidReference("object_id must be nonempty UTF-8")
	}
	for _, c := range raw {
		if unicode.IsControl(c) {
			return invalidReference("object_id cannot contain control characters")
		}
	}
	namespace, name, ok := strings.Cut(raw, "/")
	if !ok || strings.TrimSpace(namespace) == "" || strings.TrimSpace(name) == "" {
		return invalidReference("object_id %q must be <namespace>/<name>", raw)
	}
	for _, segment := range strings.Split(raw, "/") {
		if segment == "" {
			return invalidReference("object_id %q has an empty segment", raw)
		}
	}
	return nil
}

// Namespace returns the semantic owner segment of a valid object_id.
func Namespace(id ObjectID) string {
	namespace, _, _ := strings.Cut(string(id), "/")
	return namespace
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

func invalidReference(format string, args ...any) error {
	return kernel.Fail(kernel.ErrUsageInvalid, "object reference: "+format, args...)
}
