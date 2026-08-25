package cli

import (
	"fmt"
	"strconv"
	"strings"

	"kc/retrieval/elasticsearch"
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
			driver = "elasticsearch"
		default:
			return fmt.Errorf("--dsn needs --driver elasticsearch or an http URL")
		}
	}
	switch normalizeRepoDriver(driver) {
	case "postgres":
		return errUnsupportedDriver("repository", driver)
	case "elasticsearch", "es":
		cfg := elasticsearch.Config{URL: dsn}
		if err := cfg.RejectSecrets(); err != nil {
			return err
		}
		file.Elasticsearch.URL = strings.TrimRight(dsn, "/")
	default:
		if driver == "elasticsearch" || driver == "es" {
			cfg := elasticsearch.Config{URL: dsn}
			if err := cfg.RejectSecrets(); err != nil {
				return err
			}
			file.Elasticsearch.URL = strings.TrimRight(dsn, "/")
			return nil
		}
		return fmt.Errorf("--dsn is not used for driver %s", driver)
	}
	return nil
}

func applyStoreFlags(file StoresFile, flags map[string]FlagValue) (StoresFile, error) {
	if v := FlagString(flags, "profile"); v != "" {
		n, err := normalizeProfile(v)
		if err != nil {
			return StoresFile{}, err
		}
		file.Profile = n
		if n == "local" {
			file.Repository, file.Index = defaultRepositoryDriver, defaultIndexDriver
		} else {
			file.Repository, file.Index = "dolt", "elasticsearch"
		}
	}
	if v := FlagString(flags, "repository"); v != "" {
		n := normalizeRepoDriver(v)
		if n == "postgres" {
			return StoresFile{}, errUnsupportedDriver("repository", n)
		}
		file.Repository = n
	}
	if v := FlagString(flags, "index"); v != "" {
		file.Index = normalizeIndexDriver(v)
	}
	if v := FlagString(flags, "repos-dir"); v != "" {
		file.Layout.Repos = v
	}
	if v := FlagString(flags, "catalogs-dir"); v != "" {
		file.Layout.Catalogs = v
	}
	if v := FlagString(flags, "projections-dir"); v != "" {
		file.Layout.Projections = v
	}
	if v := FlagString(flags, "checkouts-dir"); v != "" {
		file.Layout.Checkouts = v
	}
	driver := FlagString(flags, "driver")
	touchedLayout := FlagString(flags, "repos-dir") != "" || FlagString(flags, "catalogs-dir") != "" || FlagString(flags, "projections-dir") != "" || FlagString(flags, "checkouts-dir") != ""
	touchedEngine := FlagString(flags, "repository") != "" || FlagString(flags, "index") != "" || FlagString(flags, "profile") != ""
	if driver == "" {
		if (touchedLayout || touchedEngine) && FlagString(flags, "host") == "" && FlagString(flags, "url") == "" && FlagString(flags, "dsn") == "" && FlagString(flags, "dir") == "" {
			out := file.withDefaults()
			if err := out.validateProfile(); err != nil {
				return StoresFile{}, err
			}
			return out, nil
		}
		return StoresFile{}, fmt.Errorf("store-set requires --driver elasticsearch|starrocks|filegit|sqlite|dolt|gitea (or --profile / --repository / --index / layout dirs)")
	}
	if strings.EqualFold(strings.TrimSpace(driver), "mysql") {
		return StoresFile{}, errUnsupportedDriver("store", "mysql")
	}
	if dsn := FlagString(flags, "dsn"); dsn != "" {
		if err := applyDSN(&file, driver, dsn); err != nil {
			return StoresFile{}, err
		}
	}
	host, portRaw := FlagString(flags, "host"), FlagString(flags, "port")
	database, user := FlagString(flags, "database"), FlagString(flags, "user")
	esURL, dir := FlagString(flags, "url"), FlagString(flags, "dir")
	port := 0
	if portRaw != "" {
		n, err := strconv.Atoi(portRaw)
		if err != nil || n <= 0 {
			return StoresFile{}, fmt.Errorf("--port must be a positive number")
		}
		port = n
	}
	switch normalizeRepoDriver(driver) {
	case "filegit":
		if dir != "" {
			file.Layout.Repos = dir
		}
		if file.Repository == "" {
			file.Repository = "filegit"
		}
	case "sqlite":
		if dir != "" {
			file.Layout.Projections = dir
		}
		if file.Index == "" || file.Index == defaultIndexDriver {
			file.Index = "sqlite"
		}
	case "dolt":
		file.Repository = "dolt"
		if dir != "" {
			file.Layout.Repos = dir
		}
	case "gitea":
		file.Repository = "gitea"
	case "postgres":
		return StoresFile{}, errUnsupportedDriver("repository", driver)
	case "starrocks":
		if host != "" {
			file.StarRocks.Host = host
		}
		if port != 0 {
			file.StarRocks.Port = port
		}
		if user != "" {
			file.StarRocks.User = user
		}
		if database != "" {
			file.StarRocks.Database = database
		}
	default:
		if driver == "elasticsearch" || driver == "es" {
			if esURL != "" {
				cfg := elasticsearch.Config{URL: esURL, User: user}
				if err := cfg.RejectSecrets(); err != nil {
					return StoresFile{}, err
				}
				file.Elasticsearch.URL = strings.TrimRight(esURL, "/")
			}
			if user != "" {
				file.Elasticsearch.User = user
			}
			break
		}
		return StoresFile{}, fmt.Errorf("unknown store driver %s", driver)
	}
	out := file.withDefaults()
	if err := out.validateProfile(); err != nil {
		return StoresFile{}, err
	}
	return out, nil
}
