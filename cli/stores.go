package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"kc/gitea"
	"kc/internal/jsonfile"
	"kc/scale"

	"gopkg.in/yaml.v3"
)

const (
	defaultRepositoryDriver = "filegit"
	defaultIndexDriver      = "sqlite"
	defaultReposDir         = "repos"
	defaultCatalogsDir      = "catalogs"
	defaultProjectionsDir   = "projections"
	legacyCatalogDir        = "repos/_catalog"
	legacyCatalogsDir       = "repos/_catalogs"
)

// LayoutFile is .kc/layout.yaml: this machine's directories. Catalog registry
// is always FileGit here. Catalogs is the parent of per-id registry gits.
// Catalog is a legacy single-registry path (repos/_catalog); omit on new homes.
type LayoutFile struct {
	Repos       string `json:"repos,omitempty" yaml:"repos"`
	Catalog     string `json:"catalog,omitempty" yaml:"catalog,omitempty"`
	Catalogs    string `json:"catalogs,omitempty" yaml:"catalogs"`
	Projections string `json:"projections,omitempty" yaml:"projections"`
}

// StoresFile is the merged in-memory view of layout.yaml + stores.yaml.
// Passwords stay in the environment, never in either file.
type StoresFile struct {
	Layout        LayoutFile                `json:"layout,omitempty" yaml:"layout,omitempty"`
	Profile       string                    `json:"profile,omitempty" yaml:"profile,omitempty"`
	Repository    string                    `json:"repository,omitempty" yaml:"repository,omitempty"`
	Index         string                    `json:"index,omitempty" yaml:"index,omitempty"`
	Cache         string                    `json:"cache,omitempty" yaml:"cache,omitempty"`
	Redis         scale.RedisConfig         `json:"redis,omitempty" yaml:"redis,omitempty"`
	Elasticsearch scale.ElasticsearchConfig `json:"elasticsearch,omitempty" yaml:"elasticsearch,omitempty"`
	StarRocks     scale.StarRocksConfig     `json:"starrocks,omitempty" yaml:"starrocks,omitempty"`
}

type storesDisk struct {
	Profile       string                    `json:"profile,omitempty" yaml:"profile,omitempty"`
	Repository    string                    `json:"repository,omitempty" yaml:"repository,omitempty"`
	Index         string                    `json:"index,omitempty" yaml:"index,omitempty"`
	Cache         string                    `json:"cache,omitempty" yaml:"cache,omitempty"`
	Layout        LayoutFile                `json:"layout,omitempty" yaml:"layout,omitempty"`
	FileGit       fileGitLegacy             `json:"filegit,omitempty" yaml:"filegit,omitempty"`
	SQLite        sqliteLegacy              `json:"sqlite,omitempty" yaml:"sqlite,omitempty"`
	Redis         scale.RedisConfig         `json:"redis,omitempty" yaml:"redis,omitempty"`
	Elasticsearch scale.ElasticsearchConfig `json:"elasticsearch,omitempty" yaml:"elasticsearch,omitempty"`
	StarRocks     scale.StarRocksConfig     `json:"starrocks,omitempty" yaml:"starrocks,omitempty"`
}

type fileGitLegacy struct {
	Dir      string `json:"dir,omitempty" yaml:"dir,omitempty"`
	Catalog  string `json:"catalog,omitempty" yaml:"catalog,omitempty"`
	Catalogs string `json:"catalogs,omitempty" yaml:"catalogs,omitempty"`
}

type sqliteLegacy struct {
	Dir string `json:"dir,omitempty" yaml:"dir,omitempty"`
}

type storesWire struct {
	Profile       string                     `yaml:"profile,omitempty"`
	Repository    string                     `yaml:"repository"`
	Index         string                     `yaml:"index"`
	Cache         string                     `yaml:"cache,omitempty"`
	Redis         *scale.RedisConfig         `yaml:"redis,omitempty"`
	Elasticsearch *scale.ElasticsearchConfig `yaml:"elasticsearch,omitempty"`
	StarRocks     *scale.StarRocksConfig     `yaml:"starrocks,omitempty"`
}

func storesPath(home string) string {
	return filepath.Join(home, "stores.yaml")
}

func layoutPath(home string) string {
	return filepath.Join(home, "layout.yaml")
}

func legacyStoresJSONPath(home string) string {
	return filepath.Join(home, "stores.json")
}

// DefaultLayout returns on-disk directories relative to --home.
func DefaultLayout() LayoutFile {
	return LayoutFile{
		Repos:       defaultReposDir,
		Catalogs:    defaultCatalogsDir,
		Projections: defaultProjectionsDir,
	}
}

// DefaultStores returns local FileGit + SQLite engines and the default layout.
// It does not invent remote hosts.
func DefaultStores() StoresFile {
	return StoresFile{
		Layout:     DefaultLayout(),
		Profile:    "local",
		Repository: defaultRepositoryDriver,
		Index:      defaultIndexDriver,
	}.withDefaults()
}

// ReadStores loads layout.yaml + stores.yaml. Missing files yield local defaults.
// Secrets in either file fail.
func ReadStores(home string) (StoresFile, error) {
	engines, legacyLayout, err := readEngineFile(home)
	if err != nil {
		return StoresFile{}, err
	}
	layout, err := readLayoutFile(home, legacyLayout)
	if err != nil {
		return StoresFile{}, err
	}
	out := engines
	out.Layout = mergeLayout(layout, legacyLayout)
	if err := out.rejectSecrets(); err != nil {
		return StoresFile{}, err
	}
	return out.withDefaults(), nil
}

// WriteStores persists layout.yaml (dirs) and stores.yaml (engines + hosts).
func WriteStores(home string, file StoresFile) error {
	file = file.withDefaults()
	if err := file.rejectSecrets(); err != nil {
		return err
	}
	if err := file.validateProfile(); err != nil {
		return err
	}
	file.Redis.Password = ""
	file.Elasticsearch.Password = ""
	file.Elasticsearch.APIKey = ""
	file.StarRocks.Password = ""
	if err := writeYAML(layoutPath(home), file.Layout); err != nil {
		return err
	}
	if err := writeYAML(storesPath(home), file.enginesWire()); err != nil {
		return err
	}
	_ = os.Remove(legacyStoresJSONPath(home))
	return nil
}

func readEngineFile(home string) (StoresFile, LayoutFile, error) {
	path := storesPath(home)
	body, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return StoresFile{}, LayoutFile{}, err
		}
		legacy := legacyStoresJSONPath(home)
		if _, statErr := os.Stat(legacy); statErr != nil {
			return StoresFile{}, LayoutFile{}, nil
		}
		var disk storesDisk
		if err := jsonfile.Read(legacy, &disk); err != nil {
			return StoresFile{}, LayoutFile{}, err
		}
		return disk.engines(), disk.legacyLayout(), nil
	}
	var disk storesDisk
	if err := yaml.Unmarshal(body, &disk); err != nil {
		return StoresFile{}, LayoutFile{}, err
	}
	return disk.engines(), disk.legacyLayout(), nil
}

func readLayoutFile(home string, fallback LayoutFile) (LayoutFile, error) {
	path := layoutPath(home)
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fallback, nil
		}
		return LayoutFile{}, err
	}
	var layout LayoutFile
	if err := yaml.Unmarshal(body, &layout); err != nil {
		return LayoutFile{}, err
	}
	return mergeLayout(layout, fallback), nil
}

func (d storesDisk) engines() StoresFile {
	return StoresFile{
		Profile:       d.Profile,
		Repository:    d.Repository,
		Index:         d.Index,
		Cache:         d.Cache,
		Redis:         d.Redis,
		Elasticsearch: d.Elasticsearch,
		StarRocks:     d.StarRocks,
	}
}

func (d storesDisk) legacyLayout() LayoutFile {
	layout := d.Layout
	if layout.Repos == "" {
		layout.Repos = d.FileGit.Dir
	}
	if layout.Catalog == "" {
		layout.Catalog = d.FileGit.Catalog
	}
	if layout.Catalogs == "" {
		layout.Catalogs = d.FileGit.Catalogs
	}
	if layout.Projections == "" {
		layout.Projections = d.SQLite.Dir
	}
	return layout
}

func mergeLayout(primary, fallback LayoutFile) LayoutFile {
	if primary.Repos == "" {
		primary.Repos = fallback.Repos
	}
	if primary.Catalog == "" {
		primary.Catalog = fallback.Catalog
	}
	if primary.Catalogs == "" {
		primary.Catalogs = fallback.Catalogs
	}
	if primary.Projections == "" {
		primary.Projections = fallback.Projections
	}
	return primary
}

func (s StoresFile) enginesWire() storesWire {
	wire := storesWire{Profile: s.Profile, Repository: s.Repository, Index: s.Index, Cache: s.Cache}
	if s.Redis.Host != "" {
		rd := s.Redis
		wire.Redis = &rd
	}
	if s.Elasticsearch.URL != "" {
		es := s.Elasticsearch
		wire.Elasticsearch = &es
	}
	if s.StarRocks.Host != "" {
		sr := s.StarRocks
		wire.StarRocks = &sr
	}
	return wire
}

func writeYAML(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(value); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s StoresFile) withDefaults() StoresFile {
	if s.Profile == "" {
		s.Profile = "local"
	}
	if s.Repository == "" {
		if s.Profile == "scale" {
			s.Repository = "dolt"
		} else {
			s.Repository = defaultRepositoryDriver
		}
	}
	s.Repository = normalizeRepoDriver(s.Repository)
	if s.Index == "" {
		if s.Profile == "scale" {
			s.Index = "elasticsearch"
		} else {
			s.Index = defaultIndexDriver
		}
	}
	s.Index = normalizeIndexDriver(s.Index)
	if s.Profile == "scale" && s.Cache == "" {
		s.Cache = "redis"
	}
	if s.Layout.Repos == "" {
		s.Layout.Repos = defaultReposDir
	}
	if s.Layout.Catalogs == "" {
		s.Layout.Catalogs = defaultCatalogsDir
	}
	if s.Layout.Projections == "" {
		s.Layout.Projections = defaultProjectionsDir
	}
	return s
}

func (s StoresFile) validateProfile() error {
	switch s.Profile {
	case "", "local", "scale":
	default:
		return fmt.Errorf("unknown store profile %s (want local or scale)", s.Profile)
	}
	if s.Repository == "postgres" {
		return errPostgresRemoved()
	}
	if s.Repository == "stream" {
		return errStreamNotRepository()
	}
	if s.Profile != "scale" && (s.Index == "redis" || s.Cache == "redis") {
		return errRedisNotLocal()
	}
	return nil
}

func normalizeRepoDriver(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "filegit", "git", "local":
		return "filegit"
	case "dolt":
		return "dolt"
	case "gitea":
		return "gitea"
	case "stream", "append":
		return "stream"
	case "postgres", "postgresql", "pg":
		return "postgres"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func normalizeIndexDriver(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "sqlite", "fts":
		return "sqlite"
	case "elasticsearch", "es":
		return "elasticsearch"
	case "redis":
		return "redis"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func normalizeProfile(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "local":
		return "local", nil
	case "scale":
		return "scale", nil
	default:
		return "", fmt.Errorf("unknown store profile %s (want local or scale)", raw)
	}
}

func errRedisNotRepository() error {
	return fmt.Errorf("redis is hot-tail cache (scale profile cache: redis), not a repository")
}

func errStreamNotRepository() error {
	return fmt.Errorf("append stream is not a snapshot repository; hang git, do not repo-add --driver stream")
}

func errRedisNotLocal() error {
	return fmt.Errorf("local profile does not use redis as index or cache; scale profile may set cache: redis")
}

func errPostgresRemoved() error {
	return fmt.Errorf("unknown repository driver postgres: not a warehouse; use filegit (local) or dolt (scale)")
}

func errMySQLNotWarehouse(kind string) error {
	return fmt.Errorf("unknown %s driver mysql: not a warehouse; StarRocks is column index (scene-side), not a step through MySQL", kind)
}

// resolveStoreDir joins a layout directory with --home unless it is absolute.
func resolveStoreDir(home, dir, fallback string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		dir = fallback
	}
	if filepath.IsAbs(dir) {
		return dir, nil
	}
	clean := filepath.Clean(dir)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("store dir %q must stay under --home", dir)
	}
	return filepath.Join(home, clean), nil
}

func (s StoresFile) rejectSecrets() error {
	if err := s.Redis.RejectSecrets(); err != nil {
		return err
	}
	if err := s.Elasticsearch.RejectSecrets(); err != nil {
		return err
	}
	if err := s.StarRocks.RejectSecrets(); err != nil {
		return err
	}
	return nil
}

// PublicStores returns file-backed endpoints for status/store-ls. It never includes env secrets.
func PublicStores(file StoresFile) map[string]any {
	file = file.withDefaults()
	layout := map[string]any{
		"repos":       file.Layout.Repos,
		"catalogs":    file.Layout.Catalogs,
		"projections": file.Layout.Projections,
	}
	if file.Layout.Catalog != "" {
		layout["catalog"] = file.Layout.Catalog
	}
	out := map[string]any{
		"layout":     layout,
		"profile":    file.Profile,
		"repository": file.Repository,
		"index":      file.Index,
		"secrets": map[string]string{
			"redis":         scale.EnvRedisPassword,
			"elasticsearch": scale.EnvElasticsearchPassword + " or " + scale.EnvElasticsearchAPIKey,
			"starrocks":     scale.EnvStarRocksPassword,
			"gitea":         gitea.EnvToken,
		},
	}
	if file.Cache != "" {
		out["cache"] = file.Cache
	}
	if file.Redis.Host != "" {
		out["redis"] = map[string]any{
			"host": file.Redis.Host,
			"port": file.Redis.Port,
			"db":   file.Redis.DB,
		}
	}
	if file.Elasticsearch.URL != "" {
		es := map[string]any{"url": file.Elasticsearch.URL}
		if file.Elasticsearch.User != "" {
			es["user"] = file.Elasticsearch.User
		}
		out["elasticsearch"] = es
	}
	if file.StarRocks.Host != "" {
		out["starrocks"] = map[string]any{
			"host":     file.StarRocks.Host,
			"port":     file.StarRocks.Port,
			"user":     file.StarRocks.User,
			"database": file.StarRocks.Database,
		}
	}
	return out
}

func applyDSN(file *StoresFile, driver, dsn string) error {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil
	}
	if driver == "" {
		switch {
		case strings.HasPrefix(dsn, "postgres://"), strings.HasPrefix(dsn, "postgresql://"):
			return errPostgresRemoved()
		case strings.HasPrefix(dsn, "redis://"), strings.HasPrefix(dsn, "rediss://"):
			driver = "redis"
		case strings.HasPrefix(dsn, "http://"), strings.HasPrefix(dsn, "https://"):
			driver = "elasticsearch"
		default:
			return fmt.Errorf("--dsn needs --driver redis|elasticsearch, or a redis:// / http URL")
		}
	}
	switch normalizeRepoDriver(driver) {
	case "postgres":
		return errPostgresRemoved()
	case "redis":
		cfg, err := scale.ParseRedisAddr(dsn)
		if err != nil {
			return err
		}
		file.Redis = mergeRedis(file.Redis, cfg)
	case "elasticsearch", "es":
		cfg := scale.ElasticsearchConfig{URL: dsn}
		if err := cfg.RejectSecrets(); err != nil {
			return err
		}
		file.Elasticsearch.URL = strings.TrimRight(dsn, "/")
	default:
		if driver == "elasticsearch" || driver == "es" {
			cfg := scale.ElasticsearchConfig{URL: dsn}
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

func mergeRedis(base, overlay scale.RedisConfig) scale.RedisConfig {
	if overlay.Host != "" {
		base.Host = overlay.Host
	}
	if overlay.Port != 0 {
		base.Port = overlay.Port
	}
	if overlay.DB != 0 {
		base.DB = overlay.DB
	}
	return base
}

func applyStoreFlags(file StoresFile, flags map[string]FlagValue) (StoresFile, error) {
	if v := FlagString(flags, "profile"); v != "" {
		n, err := normalizeProfile(v)
		if err != nil {
			return StoresFile{}, err
		}
		file.Profile = n
		if n == "local" {
			file.Repository = defaultRepositoryDriver
			file.Index = defaultIndexDriver
			file.Cache = ""
		} else {
			file.Repository = "dolt"
			file.Index = "elasticsearch"
			file.Cache = "redis"
		}
	}
	if v := FlagString(flags, "repository"); v != "" {
		n := normalizeRepoDriver(v)
		if n == "postgres" {
			return StoresFile{}, errPostgresRemoved()
		}
		if n == "redis" {
			return StoresFile{}, errRedisNotRepository()
		}
		if n == "stream" {
			return StoresFile{}, errStreamNotRepository()
		}
		file.Repository = n
	}
	if v := FlagString(flags, "index"); v != "" {
		file.Index = normalizeIndexDriver(v)
	}
	if v := FlagString(flags, "cache"); v != "" {
		file.Cache = strings.ToLower(strings.TrimSpace(v))
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
	driver := FlagString(flags, "driver")
	touchedLayout := FlagString(flags, "repos-dir") != "" || FlagString(flags, "catalogs-dir") != "" || FlagString(flags, "projections-dir") != ""
	touchedEngine := FlagString(flags, "repository") != "" || FlagString(flags, "index") != "" || FlagString(flags, "profile") != "" || FlagString(flags, "cache") != ""
	if driver == "" {
		if (touchedLayout || touchedEngine) && FlagString(flags, "host") == "" && FlagString(flags, "url") == "" && FlagString(flags, "dsn") == "" && FlagString(flags, "dir") == "" {
			out := file.withDefaults()
			if err := out.validateProfile(); err != nil {
				return StoresFile{}, err
			}
			return out, nil
		}
		return StoresFile{}, fmt.Errorf("store-set requires --driver redis|elasticsearch|starrocks|filegit|sqlite|dolt|gitea (or --profile / --repository / --index / layout dirs)")
	}
	if strings.EqualFold(strings.TrimSpace(driver), "mysql") {
		return StoresFile{}, errMySQLNotWarehouse("store")
	}
	if dsn := FlagString(flags, "dsn"); dsn != "" {
		if err := applyDSN(&file, driver, dsn); err != nil {
			return StoresFile{}, err
		}
	}
	host := FlagString(flags, "host")
	portRaw := FlagString(flags, "port")
	database := FlagString(flags, "database")
	user := FlagString(flags, "user")
	esURL := FlagString(flags, "url")
	dir := FlagString(flags, "dir")
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
	case "stream":
		return StoresFile{}, errStreamNotRepository()
	case "postgres":
		return StoresFile{}, errPostgresRemoved()
	case "redis":
		if file.Profile != "scale" {
			return StoresFile{}, errRedisNotLocal()
		}
		file.Cache = "redis"
		if host != "" {
			file.Redis.Host = host
		}
		if port != 0 {
			file.Redis.Port = port
		}
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
				cfg := scale.ElasticsearchConfig{URL: esURL, User: user}
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
