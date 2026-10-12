package home

import (
	"os"
	"path/filepath"

	"kc/kernel"
)

func validateConnectionState(c DeploymentConfig, marker deploymentState, allowUpgrade bool) error {
	if !marker.ConnectionStoreInitialized {
		if _, err := os.Stat(filepath.Join(c.StateDir, connectionLedgerFile)); err == nil {
			records, err := loadConnectionRecords(c.StateDir)
			if err != nil || len(records) > 0 {
				return kernel.Fail(kernel.ErrPreconditionFailed, "connection initialization receipt is unavailable; restore durable state")
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if c.Connections != nil && !allowUpgrade {
			return kernel.Fail(kernel.ErrPreconditionFailed, "connections require explicit deployment init to initialize their private ledger")
		}
		return nil
	}
	records, err := loadConnectionRecords(c.StateDir)
	if err != nil {
		return err
	}
	bindings := c
	bindings.Repositories = append([]RepositoryBinding(nil), c.Repositories...)
	if marker.ManagedStoreInitialized {
		managed, err := loadManagedRecords(c.StateDir)
		if err != nil {
			return err
		}
		for _, record := range managed {
			bindings.Repositories = append(bindings.Repositories, record.Binding)
		}
	}
	for _, record := range records {
		if record.legacyAuthority != "" && record.legacyAuthority != c.Registry.ID {
			return kernel.Fail(kernel.ErrPreconditionFailed, "connection belongs to a different registry authority")
		}
		bindings.Repositories = append(bindings.Repositories, record.Binding)

	}
	return bindings.Validate()
}
