package liveaccess

import (
	"time"

	"kc/kernel"
)

// RuntimeConfig supplies deployment policy to the compiled HTTP entry: the
// credential audience and the external lookup time limits. Repo metadata owns
// adoption independently of these deployment resources.
type RuntimeConfig struct {
	CredentialOrigins []string `json:"credentialOrigins,omitempty" yaml:"credentialOrigins,omitempty"`
	// LookupTimeout is each external unit's own limit, e.g. "2s".
	LookupTimeout string `json:"lookupTimeout,omitempty" yaml:"lookupTimeout,omitempty"`
	// ReadTimeout is the time all external units of one read share, e.g. "5s".
	ReadTimeout string `json:"readTimeout,omitempty" yaml:"readTimeout,omitempty"`
}

func (config RuntimeConfig) Validate() error {
	for _, origin := range config.CredentialOrigins {
		if _, err := credentialOrigin(origin); err != nil {
			return err
		}
	}
	_, err := config.Limits()
	return err
}

// Limits parses the configured time limits; unset limits stay zero.
func (config RuntimeConfig) Limits() (ReadLimits, error) {
	lookup, err := positiveDuration("lookupTimeout", config.LookupTimeout)
	if err != nil {
		return ReadLimits{}, err
	}
	read, err := positiveDuration("readTimeout", config.ReadTimeout)
	if err != nil {
		return ReadLimits{}, err
	}
	return ReadLimits{Lookup: lookup, Read: read}, nil
}

func positiveDuration(name, raw string) (time.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "liveaccess.%s requires a positive duration such as 2s", name)
	}
	return value, nil
}
