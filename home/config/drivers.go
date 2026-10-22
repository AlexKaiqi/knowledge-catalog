// Package config owns the production deployment configuration: its shapes, its
// single-document parsing, its static validation and its pure path derivation.
//
// Reading a configuration here contacts no source, opens no engine, writes no
// deployment state and starts no maintenance. Opening authorities, restoring
// resources and assembling the object graph stay in the composition root.
package config

import (
	"fmt"
	"net/url"
	"strings"

	"kc/kernel"
	"kc/snapshot"
)

// DriverLakeFS is the only Snapshot authority driver this implementation ships.
const DriverLakeFS = "lakefs"

// Authority is the pure half of one repository driver: the name it answers to
// and the checks that need no connection. The composition root keeps the other
// half — how to open, create, stamp and discover — beside the code that may
// actually reach a provider.
type Authority struct {
	Name string
	// Validate refuses a configured binding that cannot describe its authority.
	Validate func(Binding) error
	// ManagedValidate refuses a provisioning pool that cannot allocate.
	ManagedValidate func(ManagedRepositoryConfig) error
}

// Binding is the driver-neutral view of one configured authority location.
type Binding struct {
	ID     string
	Driver string
	DSN    string
	Dir    string
}

var authorities = []Authority{
	{
		Name: DriverLakeFS,
		Validate: func(item Binding) error {
			if strings.TrimSpace(item.DSN) == "" {
				return fmt.Errorf("lakefs repository %s is missing dsn", item.ID)
			}
			_, err := snapshot.ParseLakeFSDSN(item.DSN)
			return err
		},
		ManagedValidate: func(pool ManagedRepositoryConfig) error {
			if strings.TrimSpace(pool.DSN) == "" || strings.ContainsAny(pool.DSN, "?#") {
				return kernel.Fail(kernel.ErrUsageInvalid, "managed LakeFS requires an origin dsn")
			}
			if _, err := snapshot.ParseLakeFSDSN(strings.TrimRight(pool.DSN, "/") + "/name-probe"); err != nil {
				return err
			}
			namespace, err := url.Parse(strings.TrimSpace(pool.Root))
			if err != nil || namespace.Scheme != "s3" || namespace.Host == "" || namespace.User != nil || namespace.RawQuery != "" || namespace.Fragment != "" {
				return kernel.Fail(kernel.ErrUsageInvalid, "managed LakeFS requires an s3:// storage namespace prefix in root")
			}
			return nil
		},
	},
}

// Authorities returns every driver name in a stable order. The composition
// root asserts its own driver table against this list so the two cannot drift.
func Authorities() []Authority {
	return append([]Authority(nil), authorities...)
}

// LookupAuthority resolves one configured driver name.
func LookupAuthority(name string) (Authority, error) {
	name = NormalizeRepositoryDriver(name)
	for _, authority := range authorities {
		if authority.Name == name {
			return authority, nil
		}
	}
	return Authority{}, kernel.Fail(kernel.ErrUsageInvalid, "unknown repository driver %s", name)
}

// ValidateBinding applies the driver's pure checks to one configured location.
func ValidateBinding(item Binding) error {
	authority, err := LookupAuthority(item.Driver)
	if err != nil {
		return err
	}
	if authority.Validate == nil {
		return nil
	}
	return authority.Validate(item)
}

// ValidateManagedPool applies the driver's pure checks to one provisioning pool.
func ValidateManagedPool(pool ManagedRepositoryConfig) error {
	authority, err := LookupAuthority(pool.Driver)
	if err != nil {
		return err
	}
	if authority.ManagedValidate == nil {
		return kernel.Fail(kernel.ErrUsageInvalid, "managed repositories require an explicit supported driver")
	}
	return authority.ManagedValidate(pool)
}

// ValidExternalAuthorityName reports whether a bare name can address a
// provider repository without colliding with a platform-owned one.
func ValidExternalAuthorityName(name string) bool {
	return snapshot.ValidLakeFSName(name) && !snapshot.PlatformLakeFSName(name)
}
