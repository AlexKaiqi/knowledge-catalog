package object

import (
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"

	"kc/kernel"
)

// IndexDeclarationSchemaV1 is the protocol Schema an index declaration adopts.
// The declarative index plugin claims units by this Schema, never by the
// aspect name: a business aspect that happens to be called `index` is an
// ordinary aspect.
const IndexDeclarationSchemaV1 ObjectID = "schema/core/index-declaration/v1"

// Index declaration uses. They name the role a retrieval field plays in the
// access primitives, not an engine feature.
const (
	IndexUseText     = "text"
	IndexUseFilter   = "filter"
	IndexUseSort     = "sort"
	IndexUseSemantic = "semantic"
)

// Missing-value handling of one declared input.
const (
	IndexMissingSkip    = "skip"
	IndexMissingExclude = "exclude"
)

// IndexDeclaration is one member of an `index` aspect: one retrieval field.
// The member key is the declaration's stable identity; attribution and usage
// records hang on it.
type IndexDeclaration struct {
	Member         string       `json:"member"`
	Catalog        string       `json:"catalog"`
	CatalogName    string       `json:"catalogName"`
	CatalogVersion string       `json:"catalogVersion"`
	Description    string       `json:"description,omitempty"`
	Inputs         []IndexInput `json:"inputs"`
	Build          IndexBuild   `json:"build"`
	Uses           []string     `json:"uses"`
}

// IndexInput points at one Schema field. It covers every object in scope that
// adopts that Schema, never one instance.
type IndexInput struct {
	From      string       `json:"from"`
	Locator   FieldLocator `json:"locator"`
	Missing   string       `json:"missing,omitempty"`
	Boost     float64      `json:"boost,omitempty"`
	MaxTokens int          `json:"maxTokens,omitempty"`
}

// IndexBuild is the build kind plus its options. Options are opaque to the
// Writer; a plugin instance's capability manifest decides what it supports.
type IndexBuild struct {
	Kind    string         `json:"kind"`
	Options map[string]any `json:"options,omitempty"`
}

// FieldLocator is `<schema-ref>#/fields/<path>`: an object-reference to a
// Schema plus the field it names. There is no wildcard syntax.
type FieldLocator struct {
	Schema ParsedSchemaRef `json:"schema"`
	Path   string          `json:"path"`
}

func (l FieldLocator) String() string {
	ref := string(l.Schema.Object)
	if l.Schema.Commit != "" {
		ref += "@" + string(l.Schema.Commit)
	}
	if l.Schema.Repository != "" {
		ref = "kc://" + strings.TrimPrefix(string(l.Schema.Repository), "kr://") + "/" + ref
	}
	token := strings.ReplaceAll(strings.ReplaceAll(l.Path, "~", "~0"), "/", "~1")
	return ref + "#" + (&url.URL{Fragment: "/fields/" + token}).EscapedFragment()
}

// ParseFieldLocator parses one input locator.
func ParseFieldLocator(raw string) (FieldLocator, error) {
	raw = strings.TrimSpace(raw)
	ref, fragment, ok := strings.Cut(raw, "#")
	if !ok {
		return FieldLocator{}, fmt.Errorf("locator %q must be <schema-ref>#/fields/<path>", raw)
	}
	parsed, valid := ParseSchemaRef(ref)
	if !valid {
		return FieldLocator{}, fmt.Errorf("locator %q does not name a schema/* object", raw)
	}
	pointer, err := url.PathUnescape(fragment)
	if err != nil || strings.Contains(fragment, "#") {
		return FieldLocator{}, fmt.Errorf("locator %q has an invalid URI fragment", raw)
	}
	tokens, err := PointerTokens(pointer)
	if err != nil || len(tokens) != 2 || tokens[0] != "fields" || tokens[1] == "" || strings.ContainsAny(tokens[1], "*?") {
		return FieldLocator{}, fmt.Errorf("locator %q must name one field as #/fields/<path>; wildcards are not supported", raw)
	}
	return FieldLocator{Schema: parsed, Path: tokens[1]}, nil
}

// IsIndexDeclarationRef reports whether a schema_ref adopts the index
// declaration protocol Schema.
func IsIndexDeclarationRef(ref string) bool {
	parsed, ok := ParseSchemaRef(ref)
	return ok && parsed.Object == IndexDeclarationSchemaV1
}

// Keys that belong to deployment, plugin instance or engine configuration.
// A declaration never carries them.
var indexDeploymentKeys = map[string]struct{}{
	"instance": {}, "engine": {}, "endpoint": {}, "url": {}, "plugin": {},
	"shards": {}, "replicas": {}, "capacity": {},
}

// Build options that would open the engine's raw surface.
var indexRawBuildKeys = map[string]struct{}{
	"mapping": {}, "mappings": {}, "settings": {}, "script": {}, "pipeline": {},
	"ingest": {}, "shards": {}, "replicas": {}, "engine": {}, "instance": {},
}

var indexDeclarationKeys = map[string]struct{}{
	"catalog": {}, "description": {}, "inputs": {}, "build": {}, "uses": {},
}

var indexInputKeys = map[string]struct{}{
	"from": {}, "missing": {}, "boost": {}, "maxTokens": {},
}

var indexUses = map[string]struct{}{
	IndexUseText: {}, IndexUseFilter: {}, IndexUseSort: {}, IndexUseSemantic: {},
}

// ParseIndexDeclaration checks shape only. It never checks whether a plugin
// instance supports the build; that is the capability manifest's job, so a
// well-shaped declaration outside the manifest is still writable.
func ParseIndexDeclaration(address Address, value any) (IndexDeclaration, error) {
	if address.Kind != KindMember || address.MemberKey == "" {
		return IndexDeclaration{}, indexInvalid(address, "", "an index declaration is one Member of an index aspect")
	}
	body, ok := value.(map[string]any)
	if !ok {
		return IndexDeclaration{}, indexInvalid(address, "", "value must be an object")
	}
	for _, key := range sortedKeys(body) {
		if _, deployment := indexDeploymentKeys[key]; deployment {
			return IndexDeclaration{}, indexInvalid(address, key,
				"declarations carry no plugin instance, engine address, shard or capacity; those belong to deployment configuration")
		}
		if _, known := indexDeclarationKeys[key]; !known {
			return IndexDeclaration{}, indexInvalid(address, key, "undeclared field")
		}
	}
	out := IndexDeclaration{Member: address.MemberKey}
	catalog, _ := body["catalog"].(string)
	name, version, found := strings.Cut(strings.TrimSpace(catalog), "@")
	if !found || strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
		return IndexDeclaration{}, indexInvalid(address, "catalog", "must be <manifest>@<version>")
	}
	out.Catalog, out.CatalogName, out.CatalogVersion = strings.TrimSpace(catalog), name, version
	if raw, exists := body["description"]; exists {
		text, ok := raw.(string)
		if !ok {
			return IndexDeclaration{}, indexInvalid(address, "description", "must be a string")
		}
		out.Description = text
	}
	inputs, ok := body["inputs"].([]any)
	if !ok || len(inputs) == 0 {
		return IndexDeclaration{}, indexInvalid(address, "inputs", "must list at least one input")
	}
	for i, raw := range inputs {
		input, err := parseIndexInput(address, i, raw)
		if err != nil {
			return IndexDeclaration{}, err
		}
		out.Inputs = append(out.Inputs, input)
	}
	build, err := parseIndexBuild(address, body["build"])
	if err != nil {
		return IndexDeclaration{}, err
	}
	out.Build = build
	uses, ok := body["uses"].([]any)
	if !ok || len(uses) == 0 {
		return IndexDeclaration{}, indexInvalid(address, "uses", "must list at least one use")
	}
	seen := map[string]struct{}{}
	for i, raw := range uses {
		use, ok := raw.(string)
		if _, known := indexUses[use]; !ok || !known {
			return IndexDeclaration{}, indexInvalid(address, fmt.Sprintf("uses[%d]", i), "expected text, filter, sort or semantic")
		}
		if _, dup := seen[use]; dup {
			continue
		}
		seen[use] = struct{}{}
		out.Uses = append(out.Uses, use)
	}
	sort.Strings(out.Uses)
	return out, nil
}

func parseIndexInput(address Address, i int, raw any) (IndexInput, error) {
	at := fmt.Sprintf("inputs[%d]", i)
	body, ok := raw.(map[string]any)
	if !ok {
		return IndexInput{}, indexInvalid(address, at, "must be an object")
	}
	for _, key := range sortedKeys(body) {
		if _, known := indexInputKeys[key]; !known {
			return IndexInput{}, indexInvalid(address, at+"."+key, "undeclared field")
		}
	}
	from, _ := body["from"].(string)
	locator, err := ParseFieldLocator(from)
	if err != nil {
		return IndexInput{}, indexInvalid(address, at+".from", "%v", err)
	}
	input := IndexInput{From: strings.TrimSpace(from), Locator: locator}
	if raw, exists := body["missing"]; exists {
		missing, _ := raw.(string)
		if missing != IndexMissingSkip && missing != IndexMissingExclude {
			return IndexInput{}, indexInvalid(address, at+".missing", "expected skip or exclude")
		}
		input.Missing = missing
	}
	if raw, exists := body["boost"]; exists {
		boost, ok := indexNumber(raw)
		if !ok || boost <= 0 {
			return IndexInput{}, indexInvalid(address, at+".boost", "must be a positive number")
		}
		input.Boost = boost
	}
	if raw, exists := body["maxTokens"]; exists {
		tokens, ok := indexNumber(raw)
		if !ok || tokens <= 0 || tokens != math.Trunc(tokens) {
			return IndexInput{}, indexInvalid(address, at+".maxTokens", "must be a positive integer")
		}
		input.MaxTokens = int(tokens)
	}
	return input, nil
}

func parseIndexBuild(address Address, raw any) (IndexBuild, error) {
	body, ok := raw.(map[string]any)
	if !ok {
		return IndexBuild{}, indexInvalid(address, "build", "must be an object")
	}
	kind, _ := body["kind"].(string)
	if strings.TrimSpace(kind) == "" {
		return IndexBuild{}, indexInvalid(address, "build.kind", "is required")
	}
	out := IndexBuild{Kind: strings.TrimSpace(kind)}
	for _, key := range sortedKeys(body) {
		if key == "kind" {
			continue
		}
		if _, rawKey := indexRawBuildKeys[key]; rawKey {
			return IndexBuild{}, indexInvalid(address, "build."+key,
				"raw mapping, settings, scripts, pipelines and shards are not declarable; choose a build from the capability manifest")
		}
		if out.Options == nil {
			out.Options = map[string]any{}
		}
		out.Options[key] = body[key]
	}
	return out, nil
}

func indexNumber(raw any) (float64, bool) {
	switch value := raw.(type) {
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case float64:
		return value, true
	case uint64:
		return float64(value), true
	}
	return 0, false
}

func indexInvalid(address Address, at, format string, args ...any) error {
	where := AddressLabel(address)
	if at != "" {
		where += " " + at
	}
	return kernel.Fail(kernel.ErrUsageInvalid, "index declaration %s: %s", where, fmt.Sprintf(format, args...))
}
