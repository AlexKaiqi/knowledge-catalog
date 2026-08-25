package catalog

import (
	"fmt"
	"path/filepath"

	"kc/internal/gitdir"
)

// CatalogsPath is the parent directory of Catalog registry gits.
func CatalogsPath(home string) string {
	return filepath.Join(home, "catalogs")
}

const DefaultCatalogID = "kr://local/catalog"

// PeekID reads catalog.yaml at HEAD without opening the full Catalog.
func PeekID(rootDir string) (string, error) {
	body, err := gitdir.At(rootDir).Show("HEAD", CatalogFile())
	if err != nil {
		return "", err
	}
	var meta catalogMeta
	if err := decodeYAML([]byte(body), &meta); err != nil {
		return "", err
	}
	if meta.ID == "" {
		return "", fmt.Errorf("%s missing id in %s", CatalogFile(), rootDir)
	}
	return meta.ID, nil
}

// PeekStamp handles a registry directory before its first catalog.yaml commit.
func PeekStamp(rootDir string) (string, error) {
	id, err := gitdir.At(rootDir).Config(cfgCatalogID)
	if err != nil || id == "" {
		return "", fmt.Errorf("no %s in %s", cfgCatalogID, rootDir)
	}
	return id, nil
}
