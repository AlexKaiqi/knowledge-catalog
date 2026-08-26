package cli

import (
	"bytes"
	"os"
	"path/filepath"

	"kc/internal/jsonfile"
	"kc/retrieval/opensearch"
	"kc/retrieval/starrocks"

	"gopkg.in/yaml.v3"
)

type storesDisk struct {
	Profile    string            `json:"profile,omitempty" yaml:"profile,omitempty"`
	Repository string            `json:"repository,omitempty" yaml:"repository,omitempty"`
	Index      string            `json:"index,omitempty" yaml:"index,omitempty"`
	Layout     LayoutFile        `json:"layout,omitempty" yaml:"layout,omitempty"`
	FileGit    fileGitLegacy     `json:"filegit,omitempty" yaml:"filegit,omitempty"`
	SQLite     sqliteLegacy      `json:"sqlite,omitempty" yaml:"sqlite,omitempty"`
	OpenSearch opensearch.Config `json:"opensearch,omitempty" yaml:"opensearch,omitempty"`
	StarRocks  starrocks.Config  `json:"starrocks,omitempty" yaml:"starrocks,omitempty"`
}

type fileGitLegacy struct {
	Dir      string `json:"dir,omitempty" yaml:"dir,omitempty"`
	Catalog  string `json:"catalog,omitempty" yaml:"catalog,omitempty"`
	Catalogs string `json:"catalogs,omitempty" yaml:"catalogs,omitempty"`
}

type sqliteLegacy struct {
	Dir string `json:"dir,omitempty" yaml:"dir,omitempty"`
}

type storesWire struct {
	Profile    string             `yaml:"profile,omitempty"`
	Repository string             `yaml:"repository"`
	Index      string             `yaml:"index"`
	OpenSearch *opensearch.Config `yaml:"opensearch,omitempty"`
	StarRocks  *starrocks.Config  `yaml:"starrocks,omitempty"`
}

// ReadStores loads layout.yaml + stores.yaml. Missing files yield local defaults.
func ReadStores(home string) (StoresFile, error) {
	engines, legacyLayout, err := readEngineFile(home)
	if err != nil {
		return StoresFile{}, err
	}
	layout, err := readLayoutFile(home, legacyLayout)
	if err != nil {
		return StoresFile{}, err
	}
	out := engines
	out.Layout = mergeLayout(layout, legacyLayout)
	if err := out.rejectSecrets(); err != nil {
		return StoresFile{}, err
	}
	return out.withDefaults(), nil
}

// WriteStores persists layout.yaml and stores.yaml without secrets.
func WriteStores(home string, file StoresFile) error {
	file = file.withDefaults()
	if err := file.rejectSecrets(); err != nil {
		return err
	}
	if err := file.validateProfile(); err != nil {
		return err
	}
	file.OpenSearch.Password = ""
	file.OpenSearch.APIKey = ""
	file.StarRocks.Password = ""
	if err := writeYAML(layoutPath(home), file.Layout); err != nil {
		return err
	}
	if err := writeYAML(storesPath(home), file.enginesWire()); err != nil {
		return err
	}
	_ = os.Remove(legacyStoresJSONPath(home))
	return nil
}

func readEngineFile(home string) (StoresFile, LayoutFile, error) {
	path := storesPath(home)
	body, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return StoresFile{}, LayoutFile{}, err
		}
		legacy := legacyStoresJSONPath(home)
		if _, statErr := os.Stat(legacy); statErr != nil {
			return StoresFile{}, LayoutFile{}, nil
		}
		var disk storesDisk
		if err := jsonfile.Read(legacy, &disk); err != nil {
			return StoresFile{}, LayoutFile{}, err
		}
		return disk.engines(), disk.legacyLayout(), nil
	}
	var disk storesDisk
	if err := yaml.Unmarshal(body, &disk); err != nil {
		return StoresFile{}, LayoutFile{}, err
	}
	return disk.engines(), disk.legacyLayout(), nil
}

func readLayoutFile(home string, fallback LayoutFile) (LayoutFile, error) {
	body, err := os.ReadFile(layoutPath(home))
	if err != nil {
		if os.IsNotExist(err) {
			return fallback, nil
		}
		return LayoutFile{}, err
	}
	var layout LayoutFile
	if err := yaml.Unmarshal(body, &layout); err != nil {
		return LayoutFile{}, err
	}
	return mergeLayout(layout, fallback), nil
}

func (d storesDisk) engines() StoresFile {
	return StoresFile{Profile: d.Profile, Repository: d.Repository, Index: d.Index, OpenSearch: d.OpenSearch, StarRocks: d.StarRocks}
}

func (d storesDisk) legacyLayout() LayoutFile {
	layout := d.Layout
	if layout.Repos == "" {
		layout.Repos = d.FileGit.Dir
	}
	if layout.Catalog == "" {
		layout.Catalog = d.FileGit.Catalog
	}
	if layout.Catalogs == "" {
		layout.Catalogs = d.FileGit.Catalogs
	}
	if layout.Projections == "" {
		layout.Projections = d.SQLite.Dir
	}
	return layout
}

func mergeLayout(primary, fallback LayoutFile) LayoutFile {
	if primary.Repos == "" {
		primary.Repos = fallback.Repos
	}
	if primary.Catalog == "" {
		primary.Catalog = fallback.Catalog
	}
	if primary.Catalogs == "" {
		primary.Catalogs = fallback.Catalogs
	}
	if primary.Projections == "" {
		primary.Projections = fallback.Projections
	}
	if primary.Checkouts == "" {
		primary.Checkouts = fallback.Checkouts
	}
	return primary
}

func (s StoresFile) enginesWire() storesWire {
	wire := storesWire{Profile: s.Profile, Repository: s.Repository, Index: s.Index}
	if s.OpenSearch.URL != "" {
		openSearch := s.OpenSearch
		wire.OpenSearch = &openSearch
	}
	if s.StarRocks.Host != "" {
		sr := s.StarRocks
		wire.StarRocks = &sr
	}
	return wire
}

func writeYAML(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(value); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
