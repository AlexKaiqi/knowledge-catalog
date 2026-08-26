package cli

import (
	"fmt"
	"strconv"
	"strings"

	"kc/retrieval/opensearch"
)

func applyDSN(file *StoresFile, driver, dsn string) error {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil
	}
	if driver == "" {
		switch {
		case strings.HasPrefix(dsn, "postgres://"), strings.HasPrefix(dsn, "postgresql://"):
			return errUnsupportedDriver("repository", "postgres")
		case strings.HasPrefix(dsn, "http://"), strings.HasPrefix(dsn, "https://"):
			driver = "opensearch"
		default:
			return fmt.Errorf("--dsn needs --driver opensearch or an http URL")
		}
	}
	if normalizeIndexDriver(driver) == "opensearch" {
		cfg := opensearch.Config{URL: dsn}
		if err := cfg.RejectSecrets(); err != nil {
			return err
		}
		file.OpenSearch.URL = strings.TrimRight(dsn, "/")
		return nil
	}
	switch normalizeRepoDriver(driver) {
	case "postgres":
		return errUnsupportedDriver("repository", driver)
	default:
		return fmt.Errorf("--dsn is not used for driver %s", driver)
	}
}

func applyStoreFlags(file StoresFile, flags map[string]FlagValue) (StoresFile, error) {
	touched, err := applyStoreSelections(&file, flags)
	if err != nil {
		return StoresFile{}, err
	}
	driver := FlagString(flags, "driver")
	if driver == "" {
		if touched && !storeEndpointTouched(flags) {
			return validateStores(file)
		}
		return StoresFile{}, fmt.Errorf("store-set requires --driver opensearch|filegit|dolt|gitea (or --profile / --repository / --index none|opensearch / layout dirs)")
	}
	if strings.EqualFold(strings.TrimSpace(driver), "mysql") {
		return StoresFile{}, errUnsupportedDriver("store", "mysql")
	}
	if dsn := FlagString(flags, "dsn"); dsn != "" {
		if err := applyDSN(&file, driver, dsn); err != nil {
			return StoresFile{}, err
		}
	}
	endpoint, err := storeEndpointFromFlags(flags)
	if err != nil {
		return StoresFile{}, err
	}
	if err := applyStoreDriver(&file, driver, endpoint); err != nil {
		return StoresFile{}, err
	}
	return validateStores(file)
}

// applyStoreSelections owns the engine/profile and local layout axes. Concrete
// endpoint flags are interpreted only after a --driver has been selected.
func applyStoreSelections(file *StoresFile, flags map[string]FlagValue) (bool, error) {
	touched := false
	if v := FlagString(flags, "profile"); v != "" {
		n, err := normalizeProfile(v)
		if err != nil {
			return false, err
		}
		file.Profile = n
		if n == "local" {
			file.Repository, file.Index = defaultRepositoryDriver, defaultIndexDriver
		} else {
			file.Repository, file.Index = "dolt", "opensearch"
		}
		touched = true
	}
	if v := FlagString(flags, "repository"); v != "" {
		n := normalizeRepoDriver(v)
		if n == "postgres" {
			return false, errUnsupportedDriver("repository", n)
		}
		file.Repository = n
		touched = true
	}
	if v := FlagString(flags, "index"); v != "" {
		file.Index = normalizeIndexDriver(v)
		touched = true
	}
	if v := FlagString(flags, "repos-dir"); v != "" {
		file.Layout.Repos = v
		touched = true
	}
	if v := FlagString(flags, "catalogs-dir"); v != "" {
		file.Layout.Catalogs = v
		touched = true
	}
	if v := FlagString(flags, "projections-dir"); v != "" {
		file.Layout.Projections = v
		touched = true
	}
	if v := FlagString(flags, "checkouts-dir"); v != "" {
		file.Layout.Checkouts = v
		touched = true
	}
	return touched, nil
}

func storeEndpointTouched(flags map[string]FlagValue) bool {
	for _, name := range []string{"host", "url", "dsn", "dir", "port", "database", "user"} {
		if FlagString(flags, name) != "" {
			return true
		}
	}
	return false
}

type storeEndpoint struct {
	host     string
	port     int
	database string
	user     string
	url      string
	dir      string
}

func storeEndpointFromFlags(flags map[string]FlagValue) (storeEndpoint, error) {
	endpoint := storeEndpoint{
		host: FlagString(flags, "host"), database: FlagString(flags, "database"), user: FlagString(flags, "user"),
		url: FlagString(flags, "url"), dir: FlagString(flags, "dir"),
	}
	portRaw := FlagString(flags, "port")
	port := 0
	if portRaw != "" {
		n, err := strconv.Atoi(portRaw)
		if err != nil || n <= 0 {
			return storeEndpoint{}, fmt.Errorf("--port must be a positive number")
		}
		port = n
	}
	endpoint.port = port
	return endpoint, nil
}

func applyStoreDriver(file *StoresFile, driver string, endpoint storeEndpoint) error {
	switch normalizeRepoDriver(driver) {
	case "filegit":
		if endpoint.dir != "" {
			file.Layout.Repos = endpoint.dir
		}
		if file.Repository == "" {
			file.Repository = "filegit"
		}
	case "dolt":
		file.Repository = "dolt"
		if endpoint.dir != "" {
			file.Layout.Repos = endpoint.dir
		}
	case "gitea":
		file.Repository = "gitea"
	case "postgres":
		return errUnsupportedDriver("repository", driver)
	default:
		if normalizeIndexDriver(driver) == "opensearch" {
			if endpoint.url != "" {
				cfg := opensearch.Config{URL: endpoint.url, User: endpoint.user}
				if err := cfg.RejectSecrets(); err != nil {
					return err
				}
				file.OpenSearch.URL = strings.TrimRight(endpoint.url, "/")
			}
			if endpoint.user != "" {
				file.OpenSearch.User = endpoint.user
			}
			break
		}
		return fmt.Errorf("unknown store driver %s", driver)
	}
	return nil
}

func validateStores(file StoresFile) (StoresFile, error) {
	out := file.withDefaults()
	if err := out.validateProfile(); err != nil {
		return StoresFile{}, err
	}
	return out, nil
}
