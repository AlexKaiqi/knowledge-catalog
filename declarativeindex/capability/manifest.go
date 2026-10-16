package capability

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"

	retrieval "kc/discovery"
	"kc/kernel"
	objectmodel "kc/object"
)

// ManifestName is the capability manifest name of the compiled OpenSearch
// interpreter. A deployment maps <name>@<version> to exactly one instance.
const ManifestName = "declarative-index/opensearch"

// ManifestVersion is the compiled manifest version.
const ManifestVersion = "2026-10"

// Manifest is the declarative index Affordance of one plugin instance: which
// builds it supports, each option's admissible values, whether a change
// rebuilds, and its limits. It is published per instance, never per plugin.
// It never exposes raw mappings, settings, scripts, pipelines or shards.
type Manifest retrieval.IndexManifest

type BuildCapability = retrieval.IndexBuildCapability
type BuildOption = retrieval.IndexBuildOption
type InputOption = retrieval.IndexInputOption
type ManifestLimits = retrieval.IndexManifestLimits

// ManifestCatalog is the <name>@<version> a declaration names.
func (m Manifest) ManifestCatalog() string { return m.Name + "@" + m.Version }

// DefaultManifest is the compiled OpenSearch instance manifest. models lists
// the embedding models (with versions) registered for this instance. Vector
// members are supported only for registered models and the implemented distance.
func DefaultManifest(models ...string) Manifest {
	registered := append([]string{}, models...)
	sort.Strings(registered)
	array := BuildOption{Name: "array", Type: "string", Values: []string{"flatten"}, Rebuild: true}
	template := BuildOption{Name: "template", Type: "string", Rebuild: true}
	vectorUnavailable := ""
	if len(registered) == 0 {
		vectorUnavailable = "this instance registers no embedding model"
	}
	m := Manifest{
		Name: ManifestName, Version: ManifestVersion,
		Builds: []BuildCapability{
			{Kind: "text", Category: "全文", Engine: "text", InputTypes: []string{"string"}, Uses: []string{objectmodel.IndexUseText}, MaxInputs: 8,
				Options: []BuildOption{{Name: "analyzer", Type: "string", Values: []string{"standard"}, Rebuild: true}, {Name: "synonyms", Type: "objectRef", Rebuild: true, UnavailableReason: "per-member synonym tables are unavailable in this instance"}, {Name: "prefix", Type: "boolean", Rebuild: true, UnavailableReason: "per-member prefix construction is unavailable in this instance"}, template, array}},
			{Kind: "keyword", Category: "精确", Engine: "keyword + normalizer", InputTypes: []string{"string", "object_ref", "object_ref_list", "number", "integer"}, Uses: []string{objectmodel.IndexUseFilter, objectmodel.IndexUseSort}, MaxInputs: 1,
				Options: []BuildOption{{Name: "normalizer", Type: "string", Values: []string{"lowercase"}, Rebuild: true}, array}},
			{Kind: "number", Category: "过滤与排序", Engine: "long, double", InputTypes: []string{"number", "integer"}, Uses: []string{objectmodel.IndexUseFilter, objectmodel.IndexUseSort}, MaxInputs: 1,
				Options: []BuildOption{array}},
			{Kind: "date", Category: "过滤与排序", Engine: "date", InputTypes: []string{"date", "datetime", "timestamp"}, Uses: []string{objectmodel.IndexUseFilter, objectmodel.IndexUseSort}, MaxInputs: 1,
				Options: []BuildOption{array}},
			{Kind: "boolean", Category: "过滤与排序", Engine: "boolean", InputTypes: []string{"boolean"}, Uses: []string{objectmodel.IndexUseFilter}, MaxInputs: 1},
			{Kind: "vector", Category: "向量", Engine: "knn_vector (hnsw)", InputTypes: []string{"string"}, Uses: []string{objectmodel.IndexUseSemantic}, MaxInputs: 8, Requires: []string{"model"}, UnavailableReason: vectorUnavailable,
				Options: []BuildOption{{Name: "model", Type: "string", Values: registered, Rebuild: true}, {Name: "distance", Type: "string", Values: []string{"cosine"}, Rebuild: true}, template}},
		},
		Inputs: []InputOption{{Name: "missing", Type: "string", Values: []string{objectmodel.IndexMissingSkip, objectmodel.IndexMissingExclude}, Rebuild: true}, {Name: "maxTokens", Type: "integer", Rebuild: true, UnavailableReason: "per-input truncation is unavailable in this instance"}, {Name: "boost", Type: "number", Rebuild: false, UnavailableReason: "per-input scoring weights are unavailable in this instance"}},
		Limits: ManifestLimits{MaxInputs: 8, MaxTokens: 8192},
		Closed: []string{"mapping", "settings", "script", "pipeline", "shards", "replicas"},
	}
	return m.index()
}

func (m Manifest) index() Manifest {
	m.Digest = ""
	m.Digest = kernel.CanonicalDigest(m)
	return m
}

// Describe returns an independent, digest-checked copy of the public manifest.
func (m Manifest) Describe() retrieval.IndexManifest {
	data, _ := json.Marshal(m)
	var copy Manifest
	_ = json.Unmarshal(data, &copy)
	return retrieval.IndexManifest(copy.index())
}

func (m Manifest) build(kind string) (BuildCapability, bool) {
	for _, build := range m.Builds {
		if build.Kind == kind {
			return build, true
		}
	}
	return BuildCapability{}, false
}

func buildOption(build BuildCapability, name string) (BuildOption, bool) {
	for _, option := range build.Options {
		if option.Name == name {
			return option, true
		}
	}
	return BuildOption{}, false
}

func (m Manifest) inputRebuilds(name string) bool {
	for _, option := range m.Inputs {
		if option.Name == name {
			return option.Rebuild
		}
	}
	return true
}

// Check judges one shape-valid declaration against this manifest. It returns
// "" when supported, otherwise the reason. It never substitutes a version or
// a value: a declaration naming another manifest version is unsupported.
func (m Manifest) Check(declaration objectmodel.IndexDeclaration, inputs []retrieval.DeclarationInput) string {
	if declaration.CatalogName != m.Name {
		return fmt.Sprintf("catalog %s is not interpreted by this instance (%s)", declaration.Catalog, m.ManifestCatalog())
	}
	if declaration.CatalogVersion != m.Version {
		return fmt.Sprintf("declaration names manifest version %s; this instance publishes %s", declaration.CatalogVersion, m.Version)
	}
	build, known := m.build(declaration.Build.Kind)
	if !known {
		return fmt.Sprintf("build kind %s is not in manifest %s", declaration.Build.Kind, m.ManifestCatalog())
	}
	if build.UnavailableReason != "" {
		return build.UnavailableReason
	}
	if len(declaration.Inputs) > build.MaxInputs || len(declaration.Inputs) > m.Limits.MaxInputs {
		return fmt.Sprintf("build kind %s combines at most %d inputs", build.Kind, build.MaxInputs)
	}
	for _, use := range declaration.Uses {
		if !contains(build.Uses, use) {
			return fmt.Sprintf("build kind %s does not serve use %s", build.Kind, use)
		}
	}
	for _, name := range sortedOptionNames(declaration.Build.Options) {
		option, listed := buildOption(build, name)
		if !listed {
			return fmt.Sprintf("build option %s is not in manifest %s for kind %s", name, m.ManifestCatalog(), build.Kind)
		}
		if reason := checkOption(option, declaration.Build.Options[name]); reason != "" {
			return reason
		}
		if name == "template" {
			template, ok := declaration.Build.Options[name].(string)
			if !ok {
				return "build option template must be a string"
			}
			if reason := retrieval.CheckIndexTemplate(template, declaration.Inputs); reason != "" {
				return reason
			}
		}
	}
	for _, required := range build.Requires {
		if _, present := declaration.Build.Options[required]; !present {
			return fmt.Sprintf("build kind %s requires option %s", build.Kind, required)
		}
	}
	for i, input := range declaration.Inputs {
		for _, setting := range []struct {
			name    string
			present bool
			value   any
		}{{"missing", input.Missing != "", input.Missing}, {"maxTokens", input.MaxTokens != 0, input.MaxTokens}, {"boost", input.Boost != 0, input.Boost}} {
			if setting.present {
				if reason := m.checkInputOption(setting.name, setting.value); reason != "" {
					return reason
				}
			}
		}
		if input.MaxTokens > m.Limits.MaxTokens {
			return fmt.Sprintf("input %s maxTokens exceeds the manifest limit %d", input.From, m.Limits.MaxTokens)
		}
		if i < len(inputs) && inputs[i].Type != "" && !contains(build.InputTypes, inputs[i].Type) {
			return fmt.Sprintf("input %s has type %q; build kind %s accepts %s", input.From, inputs[i].Type, build.Kind, strings.Join(build.InputTypes, ", "))
		}
	}
	return ""
}

func checkOption(option BuildOption, raw any) string {
	if option.UnavailableReason != "" {
		return option.UnavailableReason
	}
	switch option.Type {
	case "boolean":
		if _, ok := raw.(bool); !ok {
			return fmt.Sprintf("build option %s must be a boolean", option.Name)
		}
		return ""
	case "number", "integer":
		return checkNumericOption(option, raw)
	}
	value, ok := raw.(string)
	if !ok || strings.TrimSpace(value) == "" {
		return fmt.Sprintf("build option %s must be a non-empty string", option.Name)
	}
	if option.Values != nil && !contains(option.Values, value) {
		if len(option.Values) == 0 {
			return fmt.Sprintf("build option %s value %s is not in the manifest; this instance registers none", option.Name, value)
		}
		return fmt.Sprintf("build option %s value %s is not in the manifest (%s)", option.Name, value, strings.Join(option.Values, ", "))
	}
	return ""
}

func checkNumericOption(option BuildOption, raw any) string {
	value, valid := numericOptionJSON(raw)
	if !valid || option.Type == "integer" && !integralJSONNumber(value) {
		article := "a"
		if option.Type == "integer" {
			article = "an"
		}
		return fmt.Sprintf("build option %s must be %s %s", option.Name, article, option.Type)
	}
	if option.Values != nil {
		for _, listed := range option.Values {
			candidate, valid := numericOptionJSON(json.Number(listed))
			if valid && kernel.CanonicalDigest(json.Number(candidate)) == kernel.CanonicalDigest(json.Number(value)) {
				return ""
			}
		}
		return fmt.Sprintf("build option %s value %s is not in the manifest (%s)", option.Name, value, strings.Join(option.Values, ", "))
	}
	return ""
}

func numericOptionJSON(raw any) (string, bool) {
	switch raw.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number:
		encoded, err := json.Marshal(raw)
		return string(encoded), err == nil
	}
	return "", false
}

// integralJSONNumber checks an already valid JSON number without rounding its
// coefficient or expanding a possibly arbitrary power of ten.
func integralJSONNumber(value string) bool {
	mantissa, power, scientific := strings.Cut(strings.ToLower(value), "e")
	exponent := new(big.Int)
	if scientific {
		exponent.SetString(power, 10)
	}
	mantissa = strings.TrimPrefix(mantissa, "-")
	point := strings.IndexByte(mantissa, '.')
	if point < 0 {
		point = len(mantissa)
	}
	coefficient := strings.ReplaceAll(mantissa, ".", "")
	last := strings.LastIndexAny(coefficient, "123456789")
	if last < 0 {
		return true
	}
	return exponent.Cmp(big.NewInt(int64(last+1-point))) >= 0
}

func (m Manifest) checkInputOption(name string, raw any) string {
	for _, option := range m.Inputs {
		if option.Name == name {
			return checkOption(BuildOption{Name: "input." + name, Type: option.Type, Values: option.Values, UnavailableReason: option.UnavailableReason}, raw)
		}
	}
	return fmt.Sprintf("input option %s is not in manifest %s", name, m.ManifestCatalog())
}

// Rebuilds reports whether replacing previous with next for the same member
// rebuilds its retrieval field under this manifest. Only options marked
// non-rebuilding (scoring weights) may differ without a rebuild.
func (m Manifest) Rebuilds(previous, next objectmodel.IndexDeclaration) bool {
	if previous.Catalog != next.Catalog || previous.Build.Kind != next.Build.Kind || len(previous.Inputs) != len(next.Inputs) || strings.Join(previous.Uses, ",") != strings.Join(next.Uses, ",") {
		return true
	}
	build, known := m.build(next.Build.Kind)
	if !known {
		return true
	}
	names := map[string]bool{}
	for name := range previous.Build.Options {
		names[name] = true
	}
	for name := range next.Build.Options {
		names[name] = true
	}
	for name := range names {
		if kernel.CanonicalDigest(previous.Build.Options[name]) == kernel.CanonicalDigest(next.Build.Options[name]) {
			continue
		}
		option, listed := buildOption(build, name)
		if !listed || option.Rebuild {
			return true
		}
	}
	for i := range previous.Inputs {
		a, b := previous.Inputs[i], next.Inputs[i]
		if a.From != b.From || (m.inputRebuilds("missing") && a.Missing != b.Missing) || (m.inputRebuilds("maxTokens") && a.MaxTokens != b.MaxTokens) {
			return true
		}
		if m.inputRebuilds("boost") && a.Boost != b.Boost {
			return true
		}
	}
	return false
}

func sortedOptionNames(options map[string]any) []string {
	names := make([]string, 0, len(options))
	for name := range options {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
