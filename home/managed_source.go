package home

import (
	"kc/kernel"
	"kc/snapshot"
)

// restoreManagedSource preserves the driver's exact optional capabilities.
func restoreManagedSource(record managedRecord) (snapshot.Store, error) {
	return openVerifiedManagedSource(record)
}

func openVerifiedManagedSource(record managedRecord) (snapshot.Store, error) {
	driver, err := authorityFor(record.Binding.Driver)
	if err != nil {
		return nil, err
	}
	source, err := driver.managedOpen(record.Binding, record.AllocationID, record.BackendID)
	if err != nil {
		return nil, err
	}
	if !source.HasCommit(record.Head) {
		closeManagedSource(source)
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "managed initial published commit is unavailable")
	}
	return source, nil
}
