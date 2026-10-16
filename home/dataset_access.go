package home

import (
	"path/filepath"
	"strings"

	"kc/internal/jsonfile"
	"kc/kernel"
)

const (
	datasetAccessVersion = 1
	datasetAccessFile    = "dataset-access.json"
)

// DatasetAccess is a deployment-declared default for authenticated principals
// on one published Dataset: every verified caller may consume that delivery.
// It never admits the source Repositories, writes, grants or Dataset
// management, and it is not a grant to a synthetic principal.
type DatasetAccess struct {
	ID                   string   `json:"id" yaml:"id"`
	AuthenticatedActions []string `json:"authenticatedActions,omitempty" yaml:"authenticatedActions,omitempty"`
}

// datasetAccessRecord is the stateDir copy of the deployment input. Opening a
// deployment rewrites it from configuration, so authorization paths that only
// know the state directory evaluate the same declaration as the opened Home.
type datasetAccessRecord struct {
	Version  int             `json:"version"`
	Datasets []DatasetAccess `json:"datasets"`
}

// AuthenticatedDatasetActions is the closed consume-side set a Dataset may
// grant to every authenticated principal.
func AuthenticatedDatasetActions() []string {
	return []string{"dataset.resolve", "file.read"}
}

// ReadDatasetAccess returns the declared authenticated Dataset defaults. A
// missing record declares none; an unreadable record is an error so callers
// fail closed.
func ReadDatasetAccess(dir string) ([]DatasetAccess, error) {
	path := filepath.Join(dir, datasetAccessFile)
	if !fileExists(path) {
		return nil, nil
	}
	var record datasetAccessRecord
	if err := jsonfile.Read(path, &record); err != nil {
		return nil, err
	}
	if err := validateDatasetAccess(record.Datasets); err != nil {
		return nil, err
	}
	return record.Datasets, nil
}

func writeDatasetAccess(dir string, items []DatasetAccess) error {
	return jsonfile.Write(filepath.Join(dir, datasetAccessFile), datasetAccessRecord{
		Version:  datasetAccessVersion,
		Datasets: append([]DatasetAccess{}, items...),
	})
}

func validateDatasetAccess(items []DatasetAccess) error {
	allowed := map[string]bool{}
	for _, action := range AuthenticatedDatasetActions() {
		allowed[action] = true
	}
	seen := map[string]bool{}
	for _, item := range items {
		if item.ID == "" || strings.TrimSpace(item.ID) != item.ID || strings.ContainsAny(item.ID, " \t\n/") {
			return kernel.Fail(kernel.ErrUsageInvalid, "datasetAccess has invalid Dataset id %q", item.ID)
		}
		if seen[item.ID] {
			return kernel.Fail(kernel.ErrUsageInvalid, "datasetAccess has duplicate Dataset id %q", item.ID)
		}
		seen[item.ID] = true
		if len(item.AuthenticatedActions) == 0 {
			return kernel.Fail(kernel.ErrUsageInvalid, "datasetAccess %s requires authenticatedActions", item.ID)
		}
		for _, action := range item.AuthenticatedActions {
			if !allowed[action] {
				return kernel.Fail(kernel.ErrUsageInvalid, "datasetAccess authenticatedActions cannot include %q", action)
			}
		}
	}
	return nil
}
