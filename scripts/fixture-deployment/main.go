// fixture-deployment provisions empty authorities for acceptance harnesses.
// It is not part of the kc product command surface. Knowledge is published by
// the harness through the public Writer after deployment initialization.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	kchome "kc/home"
	"kc/kernel"
	knowledgedolt "kc/knowledge/dolt"
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
	catalogID := flag.String("catalog", "", "Catalog identity")
	principal := flag.String("principal", "", "explicit bootstrap principal")
	index := flag.String("opensearch", "", "optional existing OpenSearch URL")
	var dolt, gitea bindings
	flag.Var(&dolt, "repo", "empty Dolt source to provision: identity=absolute-directory; repeatable")
	flag.Var(&gitea, "gitea-repo", "existing Gitea source binding: identity=DSN; repeatable")
	flag.Parse()
	if *root == "" || *catalogID == "" || *principal == "" || flag.NArg() != 0 {
		return fmt.Errorf("require --root, --catalog and --principal")
	}
	abs, err := filepath.Abs(*root)
	if err != nil {
		return err
	}
	configPath := filepath.Join(abs, "deployment.json")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		return fmt.Errorf("fixture configuration already exists or is inaccessible: %s", configPath)
	}
	config := kchome.DeploymentConfig{Version: 1, StateDir: filepath.Join(abs, "durable"), CacheDir: filepath.Join(abs, "cache"), Auth: "local", BootstrapPrincipal: *principal,
		Catalogs: []kchome.CatalogBinding{{ID: *catalogID, Remote: filepath.Join(abs, "authority.git")}}}
	defaults := kchome.DefaultStores()
	config.Stores = kchome.StoresFile{Index: defaults.Index, OpenSearch: defaults.OpenSearch}
	if *index != "" {
		config.Stores.Index = "opensearch"
		config.Stores.OpenSearch.URL = *index
	}
	for _, sources := range []struct {
		values bindings
		driver string
	}{{dolt, "dolt"}, {gitea, "gitea"}} {
		for _, value := range sources.values {
			id, location, ok := strings.Cut(value, "=")
			if !ok || location == "" {
				return fmt.Errorf("source must be identity=location")
			}
			binding := kchome.RepositoryBinding{ID: id, Driver: sources.driver}
			if sources.driver == "dolt" {
				binding.Dir = location
			} else {
				binding.DSN = location
			}
			config.Repositories = append(config.Repositories, binding)
		}
	}
	if err := config.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0700); err != nil {
		return err
	}
	if raw, err := exec.Command("git", "init", "--bare", config.Catalogs[0].Remote).CombinedOutput(); err != nil {
		return fmt.Errorf("provision Catalog Git: %s: %w", raw, err)
	}
	for _, binding := range config.Repositories {
		if binding.Driver != "dolt" {
			continue
		}
		_, err := knowledgedolt.Open(binding.Dir, kernel.RepositoryID(binding.ID))
		if err != nil {
			return err
		}
	}
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
