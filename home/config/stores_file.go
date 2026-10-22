package config

import (
	"bytes"
	"os"
	"path/filepath"

	"kc/declarativeindex/opensearch"
	"kc/internal/jsonfile"

	"gopkg.in/yaml.v3"
)

type storesDisk struct {
	Profile        string                `json:"profile,omitempty" yaml:"profile,omitempty"`
	Repository     string                `json:"repository,omitempty" yaml:"repository,omitempty"`
	Index          string                `json:"index,omitempty" yaml:"index,omitempty"`
	Layout         LayoutFile            `json:"layout,omitempty" yaml:"layout,omitempty"`
	OpenSearch     opensearch.Config     `json:"opensearch,omitempty" yaml:"opensearch,omitempty"`
	HydrationCache *HydrationCacheConfig `json:"hydrationCache,omitempty" yaml:"hydrationCache,omitempty"`
}

type storesWire struct {
	Profile        string                `yaml:"profile,omitempty"`
	Repository     string                `yaml:"repository"`
	Index          string                `yaml:"index"`
	OpenSearch     *opensearch.Config    `yaml:"opensearch,omitempty"`
	HydrationCache *HydrationCacheConfig `yaml:"hydrationCache,omitempty"`
}

// ReadStores loads layout.yaml + stores.yaml. Missing files yield local
// defaults; a retired stores.json is still honoured so an existing home opens.
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
	out.Layout = MergeLayout(layout, legacyLayout)
	if err := out.RejectSecrets(); err != nil {
		return StoresFile{}, err
	}
	out = out.WithDefaults()
	if err := out.ValidateProfile(); err != nil {
		return StoresFile{}, err
	}
	return out, nil
}

// WriteStores persists layout.yaml and stores.yaml without secrets.
func WriteStores(home string, file StoresFile) error {
	file = file.WithDefaults()
	if err := file.RejectSecrets(); err != nil {
		return err
	}
	if err := file.ValidateProfile(); err != nil {
		return err
	}
	file.OpenSearch.Password = ""
	file.OpenSearch.APIKey = ""
	if err := writeYAML(LayoutPath(home), file.Layout); err != nil {
		return err
	}
	if err := writeYAML(StoresPath(home), file.enginesWire()); err != nil {
		return err
	}
	_ = os.Remove(LegacyStoresPath(home))
	return nil
}

func readEngineFile(home string) (StoresFile, LayoutFile, error) {
	path := StoresPath(home)
	body, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return StoresFile{}, LayoutFile{}, err
		}
		legacy := LegacyStoresPath(home)
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
	body, err := os.ReadFile(LayoutPath(home))
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
	return MergeLayout(layout, fallback), nil
}

func (d storesDisk) engines() StoresFile {
	return StoresFile{Profile: d.Profile, Repository: d.Repository, Index: d.Index, OpenSearch: d.OpenSearch, HydrationCache: d.HydrationCache}
}

func (d storesDisk) legacyLayout() LayoutFile {
	return d.Layout
}

func (s StoresFile) enginesWire() storesWire {
	wire := storesWire{Profile: s.Profile, Repository: s.Repository, Index: s.Index, HydrationCache: s.HydrationCache}
	if s.OpenSearch.URL != "" {
		openSearch := s.OpenSearch
		wire.OpenSearch = &openSearch
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
