// fixture-deployment verifies declared authorities for acceptance harnesses.
// It is not part of the kc product command surface. Object is published by
// the harness through the public Writer after deployment initialization.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	kchome "kc/home"
)

type bindings []string

func (b *bindings) String() string         { return strings.Join(*b, ",") }
func (b *bindings) Set(value string) error { *b = append(*b, value); return nil }

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	root := flag.String("root", "", "fresh fixture deployment root")
	registryID := flag.String("registry-id", "", "registration Snapshot Repository identity")
	registryDriver := flag.String("registry-driver", "lakefs", "registration Snapshot driver: lakefs")
	registryDSN := flag.String("registry-dsn", "", "registration Snapshot DSN when driver is lakefs")
	principal := flag.String("principal", "", "explicit bootstrap principal")
	index := flag.String("opensearch", "", "optional existing OpenSearch URL")
	var lakefs bindings
	flag.Var(&lakefs, "lakefs-repo", "existing LakeFS source binding: identity=DSN; repeatable")
	managedLakefsDSN := flag.String("managed-lakefs-dsn", "", "LakeFS origin for managedStores (http(s)://host, no repository path)")
	managedLakefsNamespace := flag.String("managed-lakefs-namespace", "", "s3:// prefix for managed Graveler storage namespaces")
	managedPublicURL := flag.String("managed-public-url", "", "public KC URL returned as managementURL")
	liveLookupTimeout := flag.String("live-lookup-timeout", "", "optional liveaccess.lookupTimeout (Go duration)")
	liveReadTimeout := flag.String("live-read-timeout", "", "optional liveaccess.readTimeout (Go duration)")
	flag.Parse()
	if *root == "" || *registryID == "" || *principal == "" || flag.NArg() != 0 {
		return fmt.Errorf("require --root, --registry-id and --principal")
	}
	abs, err := filepath.Abs(*root)
	if err != nil {
		return err
	}
	configPath := filepath.Join(abs, "deployment.json")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		return fmt.Errorf("fixture configuration already exists or is inaccessible: %s", configPath)
	}
	registry, err := registryBinding(*registryID, *registryDriver, *registryDSN)
	if err != nil {
		return err
	}
	config := kchome.DeploymentConfig{Version: 1, StateDir: filepath.Join(abs, "durable"), CacheDir: filepath.Join(abs, "cache"), Auth: "local", BootstrapPrincipal: *principal,
		Registry:         registry,
		RepositoryAccess: []kchome.RepositoryAccess{kchome.SystemRepositoryAccess()}}
	defaults := kchome.DefaultStores()
	config.Stores = kchome.StoresFile{Index: defaults.Index, OpenSearch: defaults.OpenSearch}
	if *index != "" {
		config.Stores.Index = "opensearch"
		config.Stores.OpenSearch.URL = *index
	}
	for _, sources := range []struct {
		values bindings
		driver string
	}{{lakefs, "lakefs"}} {
		for _, value := range sources.values {
			id, location, ok := strings.Cut(value, "=")
			if !ok || location == "" {
				return fmt.Errorf("source must be identity=location")
			}
			binding := kchome.RepositoryBinding{ID: id, Driver: sources.driver, DSN: location}
			config.Repositories = append(config.Repositories, binding)
		}
	}
	if *managedLakefsDSN != "" || *managedLakefsNamespace != "" || *managedPublicURL != "" {
		if *managedLakefsDSN == "" || *managedLakefsNamespace == "" || *managedPublicURL == "" {
			return fmt.Errorf("managed lakeFS requires --managed-lakefs-dsn, --managed-lakefs-namespace and --managed-public-url")
		}
		// Fixture configuration uses the public deployment document, not
		// provisioning-runtime types from the Home composition root.
		managed, err := json.Marshal(map[string]any{"managedStores": map[string]any{
			"lakefs": map[string]any{
				"driver":    "lakefs",
				"dsn":       *managedLakefsDSN,
				"root":      *managedLakefsNamespace,
				"publicURL": *managedPublicURL,
				"creatorActions": []string{
					"writer.preview", "writer.commit", "writer.receipt.read",
					"object.read", "object.schema.read", "repository.metadata.read",
				},
			},
		}})
		if err != nil {
			return err
		}
		if err := json.Unmarshal(managed, &config); err != nil {
			return err
		}
	}
	config.LiveAccess.LookupTimeout = *liveLookupTimeout
	config.LiveAccess.ReadTimeout = *liveReadTimeout
	if err := config.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0700); err != nil {
		return err
	}
	if err := kchome.PrepareRegistryAuthority(config.Registry); err != nil {
		return fmt.Errorf("verify registration Snapshot: %w", err)
	}
	// Repository fixture bindings name authorities provisioned elsewhere
	// (the local lakeFS stack); provisioning used to be a
	// local-authority concern and is gone with that driver.
	raw, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(configPath, append(raw, '\n'), 0600); err != nil {
		return err
	}
	fmt.Println(configPath)
	return nil
}

func registryBinding(id, driver, dsn string) (kchome.RegistryBinding, error) {
	driver = strings.TrimSpace(driver)
	if driver == "" {
		driver = "lakefs"
	}
	switch driver {
	case "lakefs":
		if strings.TrimSpace(dsn) == "" {
			return kchome.RegistryBinding{}, fmt.Errorf("%s registry requires --registry-dsn http(s)://host/repository", driver)
		}
		return kchome.RegistryBinding{ID: id, Driver: driver, DSN: dsn}, nil
	default:
		return kchome.RegistryBinding{}, fmt.Errorf("registry-driver must be lakefs")
	}
}
