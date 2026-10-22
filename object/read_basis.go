package object

import (
	"sort"
	"strings"

	"kc/kernel"
)

// ReadBasis records immutable local read coordinates. It grants no permission:
// the application must revalidate the Dataset publication or Repository scope
// and authorize every request before opening it. It contains no observed value
// and does not retain search resources.
type ReadBasis struct {
	Version   int    `json:"version"`
	Reference string `json:"reference"`
	Shape     string `json:"shape,omitempty"`
	Dataset   string `json:"dataset,omitempty"`
	Revision  int    `json:"revision,omitempty"`
	// Temporary marks an unpublished delivery definition. Its caller label
	// must never be used as a named Dataset grant or publication coordinate.
	Temporary              bool                    `json:"temporary,omitempty"`
	Candidates             []ReadSource            `json:"candidates"`
	Dependencies           []ReadSource            `json:"dependencies,omitempty"`
	UnresolvedDependencies []ReadDependencyFailure `json:"unresolvedDependencies,omitempty"`
	Paths                  []ReadPath              `json:"paths"`
}

// ReadDependencyFailure preserves an interpretation candidate whose version
// could not be frozen. Local state can still be complete, but such a basis
// cannot promise an exact interpretation replay.
type ReadDependencyFailure struct {
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit,omitempty"`
	Code       kernel.ErrorCode    `json:"code"`
}

// ReadSource includes every candidate, even one that contributed no unit.
// Binding identifies the authority instance, independently of its logical ID.
type ReadSource struct {
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
	Binding    kernel.Digest       `json:"binding"`
}

// ReadPath is a logical file selection, independent of the selected commit.
// An empty prefix selects the whole Repository; no paths selects no files.
type ReadPath struct {
	Repository kernel.RepositoryID `json:"repository"`
	Kind       string              `json:"kind"`
	Prefix     string              `json:"prefix,omitempty"`
	File       string              `json:"file,omitempty"`
}

func (b ReadBasis) Validate() error {
	ref, err := ParseReference(b.Reference)
	if err != nil {
		return err
	}
	if b.Version != 1 || len(b.Candidates) == 0 || b.Revision < 0 || (!b.Temporary && (b.Dataset == "") != (b.Revision == 0)) {
		return kernel.Fail(kernel.ErrUsageInvalid, "invalid object read basis version or scope")
	}
	if b.Shape != "" {
		if ref.Locator != "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "read basis cannot combine a locator and shape")
		}
		if _, err := ParseShape(b.Shape); err != nil {
			return err
		}
	}
	candidates, err := validateReadSources(b.Candidates)
	if err != nil {
		return err
	}
	if _, err := validateReadSources(b.Dependencies); err != nil {
		return err
	}
	bindings := map[kernel.RepositoryID]kernel.Digest{}
	for _, failure := range b.UnresolvedDependencies {
		if failure.Repository == "" || failure.Code == "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "invalid unresolved interpretation candidate")
		}
	}
	for _, source := range b.Candidates {
		bindings[source.Repository] = source.Binding
	}
	for _, source := range b.Dependencies {
		if binding, exists := bindings[source.Repository]; exists && binding != source.Binding {
			return kernel.Fail(kernel.ErrUsageInvalid, "read basis has inconsistent source bindings")
		}
	}
	for _, path := range b.Paths {
		if !candidates[path.Repository] || path.Kind != "prefix" && path.Kind != "file" ||
			path.Kind == "file" && (path.File == "" || path.Prefix != "") || path.Kind == "prefix" && path.File != "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "invalid file selection in object read basis")
		}
		for _, part := range strings.Split(path.Prefix+path.File, "/") {
			if part == "." || part == ".." {
				return kernel.Fail(kernel.ErrUsageInvalid, "read basis file selection contains a relative traversal")
			}
		}
	}
	return nil
}

func validateReadSources(sources []ReadSource) (map[kernel.RepositoryID]bool, error) {
	seen := make(map[kernel.RepositoryID]bool, len(sources))
	for _, source := range sources {
		if source.Repository == "" || source.Commit == "" || source.Binding == "" || seen[source.Repository] {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "read basis requires unique, bound, fixed sources")
		}
		seen[source.Repository] = true
	}
	return seen, nil
}

// ScopeDigest describes logical selection, not the returned unit set or a
// version. Equal digests alone do not establish local-state completeness.
func (b ReadBasis) ScopeDigest() (kernel.Digest, error) {
	if err := b.Validate(); err != nil {
		return "", err
	}
	type candidate struct {
		Repository kernel.RepositoryID
		Binding    kernel.Digest
	}
	sources := make([]candidate, 0, len(b.Candidates))
	for _, source := range b.Candidates {
		sources = append(sources, candidate{source.Repository, source.Binding})
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Repository < sources[j].Repository })
	paths := make([]ReadPath, 0, len(b.Paths))
	seen := map[ReadPath]bool{}
	for _, path := range b.Paths {
		path.Prefix, path.File = strings.Trim(path.Prefix, "/"), strings.Trim(path.File, "/")
		if !seen[path] {
			paths = append(paths, path)
			seen[path] = true
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		a, z := paths[i], paths[j]
		if a.Repository != z.Repository {
			return a.Repository < z.Repository
		}
		if a.Kind != z.Kind {
			return a.Kind < z.Kind
		}
		if a.Prefix != z.Prefix {
			return a.Prefix < z.Prefix
		}
		return a.File < z.File
	})
	ref, _ := ParseReference(b.Reference)
	reference, _ := FormatReference(ref)
	return kernel.CanonicalDigest(struct {
		Reference  string
		Shape      any
		Candidates []candidate
		Paths      []ReadPath
	}{reference, readShapeScope(b.Shape), sources, paths}), nil
}

func readShapeScope(raw string) any {
	if raw == "" {
		return nil
	}
	shape, _ := ParseShape(raw) // Validate already checked the syntax.
	aspects := map[string]any{}
	for _, aspect := range shape.Aspects {
		keys := append([]string(nil), aspect.Keys...)
		sort.Strings(keys)
		aspects[aspect.Name] = struct {
			Keys   []string
			Fields map[string]any
		}{keys, readShapeFieldScope(aspect.Fields)}
	}
	return aspects
}

func readShapeFieldScope(fields []ShapeField) map[string]any {
	if fields == nil {
		return nil
	}
	out := make(map[string]any, len(fields))
	for _, field := range fields {
		out[field.Name] = readShapeFieldScope(field.Children)
	}
	return out
}
