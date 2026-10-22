package home

import (
	"path/filepath"

	"kc/home/config"
	"kc/internal/jsonfile"
)

const (
	datasetAccessVersion = 1
	datasetAccessFile    = "dataset-access.json"
)

var (
	// AuthenticatedDatasetActions is the closed consume-side action set.
	AuthenticatedDatasetActions = config.AuthenticatedDatasetActions
)

// datasetAccessRecord is the stateDir copy of the deployment input. Opening a
// deployment rewrites it from configuration, so authorization paths that only
// know the state directory evaluate the same declaration as the opened Home.
type datasetAccessRecord struct {
	Version  int             `json:"version"`
	Datasets []DatasetAccess `json:"datasets"`
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
	if err := config.ValidateDatasetAccess(record.Datasets); err != nil {
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
