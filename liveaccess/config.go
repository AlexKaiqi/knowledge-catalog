package liveaccess

// RuntimeConfig supplies credential target policy to the compiled HTTP entry.
// Repo metadata owns adoption independently of these deployment resources.
type RuntimeConfig struct {
	CredentialOrigins []string `json:"credentialOrigins,omitempty" yaml:"credentialOrigins,omitempty"`
}

func (config RuntimeConfig) Validate() error {
	for _, origin := range config.CredentialOrigins {
		if _, err := credentialOrigin(origin); err != nil {
			return err
		}
	}
	return nil
}
