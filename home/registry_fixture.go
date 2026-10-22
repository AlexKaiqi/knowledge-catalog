package home

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
	"kc/kernel"
)

const fixtureRegistryPointer = "registry-authority.yaml"

// Component homes keep an explicit remote pointer, never a local Git authority.
func fixtureRegistryBinding(binding RegistryBinding) (RegistryBinding, error) {
	id, err := NormalizeRepositoryID(binding.ID)
	if err != nil || id != binding.ID || binding.Driver != "lakefs" || binding.DSN == "" || binding.Dir != "" {
		return RegistryBinding{}, kernel.Fail(kernel.ErrUsageInvalid, "component registry initialization requires an explicit lakeFS binding")
	}
	driver, err := authorityFor(binding.Driver)
	if err != nil {
		return RegistryBinding{}, err
	}
	if err := driver.validate(registryHomeRepo(binding)); err != nil {
		return RegistryBinding{}, err
	}
	return binding, nil
}

func writeFixtureRegistryBinding(dir string, binding RegistryBinding) error {
	raw, err := yaml.Marshal(binding)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, fixtureRegistryPointer), raw, 0644)
}

func readFixtureRegistryBinding(dir string) (RegistryBinding, error) {
	raw, err := os.ReadFile(filepath.Join(dir, fixtureRegistryPointer))
	if err != nil {
		return RegistryBinding{}, err
	}
	var binding RegistryBinding
	if err := yaml.Unmarshal(raw, &binding); err != nil {
		return binding, err
	}
	id, err := NormalizeRepositoryID(binding.ID)
	if err != nil || id != binding.ID {
		return binding, kernel.Fail(kernel.ErrUsageInvalid, "component registry pointer has an invalid identity")
	}
	return fixtureRegistryBinding(binding)
}
