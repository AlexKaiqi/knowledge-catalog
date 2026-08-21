package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"kc/catalog"
	"kc/controlplane"
	"kc/gitea"
	"kc/index"
	"kc/internal/journal"
	"kc/internal/jsonfile"
	"kc/kernel"
	"kc/local"
	"kc/reader"
	"kc/repository"
	"kc/scale"
	"kc/writer"
)

type WorkspaceFile struct {
	Catalogs    []WorkspaceCatalog `json:"catalogs"`
	Repos       []WorkspaceRepo    `json:"repos"`
	IndexDriver string             `json:"indexDriver,omitempty"`
	IndexURL    string             `json:"indexURL,omitempty"`
}

type WorkspaceCatalog struct {
	ID  string `json:"id"`
	Dir string `json:"dir"`
}

type WorkspaceRepo struct {
	ID     string `json:"id"`
	Dir    string `json:"dir,omitempty"`
	Driver string `json:"driver,omitempty"`
	DSN    string `json:"dsn,omitempty"`
}

type OpenWorkspace struct {
	Home         string
	Store        *repository.Store
	Writer       *writer.Writer
	Reader       *reader.Reader
	Catalogs     map[string]*catalog.Catalog
	Registries   map[string]*catalog.Registry
	Catalog      *catalog.Catalog
	Registry     *catalog.Registry
	ControlPlane *controlplane.ControlPlane
	ControlStore *controlplane.FileControlState
	File         WorkspaceFile
	Control      controlplane.ControlState
	Controls     map[string]controlplane.ControlState
	controlID    string
	Journal      journal.Journal
	Index        *index.Index
	Stores       StoresFile
}

func workspacePath(home string) string {
	return filepath.Join(home, "workspace.json")
}

func missingHome(home string) error {
	return fmt.Errorf("no kc home at %s; run: kc init --home %s", home, home)
}

func homeReady(home string) bool {
	if _, err := os.Stat(layoutPath(home)); err == nil {
		return true
	}
	if _, err := os.Stat(storesPath(home)); err == nil {
		return true
	}
	_, err := os.Stat(workspacePath(home))
	return err == nil
}

func catalogDirOf(stores StoresFile, catalogID string) string {
	root := stores.Layout.Catalogs
	if root == "" {
		root = defaultCatalogsDir
	}
	return filepath.ToSlash(filepath.Join(root, EncodeRepoDir(catalogID)))
}

func homeRel(home, abs string) string {
	rel, err := filepath.Rel(home, abs)
	if err != nil {
		return filepath.ToSlash(abs)
	}
	return filepath.ToSlash(rel)
}

func isGitDir(abs string) bool {
	_, err := os.Stat(filepath.Join(abs, ".git"))
	return err == nil
}

var unsafeRepoChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func EncodeRepoDir(repositoryID string) string {
	return unsafeRepoChars.ReplaceAllString(repositoryID, "_")
}

// NormalizeCatalogID accepts kr://<org>/<name> or <org>/<name>.
func NormalizeCatalogID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("catalog id is required")
	}
	if strings.Contains(raw, "://") {
		if !strings.HasPrefix(raw, "kr://") {
			return "", fmt.Errorf("catalog id must use kr://, got %s", raw)
		}
		rest := strings.TrimPrefix(raw, "kr://")
		if rest == "" || strings.HasSuffix(rest, "/") || !strings.Contains(rest, "/") {
			return "", fmt.Errorf("catalog id must be kr://<org>/<name>, got %s", raw)
		}
		return raw, nil
	}
	if !strings.Contains(raw, "/") {
		return "", fmt.Errorf("catalog id must be kr://<org>/<name> or <org>/<name>, got %q", raw)
	}
	return "kr://" + raw, nil
}

func InitWorkspace(home, catalogID string) (WorkspaceFile, bool, error) {
	stores := DefaultStores()
	repoRoot, err := resolveStoreDir(home, stores.Layout.Repos, defaultReposDir)
	if err != nil {
		return WorkspaceFile{}, false, err
	}
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		return WorkspaceFile{}, false, err
	}
	catRoot, err := resolveStoreDir(home, stores.Layout.Catalogs, defaultCatalogsDir)
	if err != nil {
		return WorkspaceFile{}, false, err
	}
	if err := os.MkdirAll(catRoot, 0o755); err != nil {
		return WorkspaceFile{}, false, err
	}
	createdHome := false
	if !homeReady(home) {
		if err := WriteStores(home, stores); err != nil {
			return WorkspaceFile{}, false, err
		}
		createdHome = true
	}
	stores, err = ReadStores(home)
	if err != nil {
		return WorkspaceFile{}, false, err
	}
	file, err := ReadWorkspace(home)
	if err != nil {
		return WorkspaceFile{}, false, err
	}
	if catalogID != "" {
		catalogID, err = NormalizeCatalogID(catalogID)
		if err != nil {
			return WorkspaceFile{}, false, err
		}
	} else if createdHome || len(file.Catalogs) == 0 {
		catalogID = catalog.DefaultCatalogID
	} else {
		return file, false, nil
	}
	if hasCatalog(file, catalogID) {
		return file, false, nil
	}
	if !createdHome && len(file.Catalogs) > 0 {
		return WorkspaceFile{}, false, fmt.Errorf("this home already has catalog %s; requested %s. Use catalog-add, or a different --home", file.Catalogs[0].ID, catalogID)
	}
	dirRel := catalogDirOf(stores, catalogID)
	catalogAbs, err := resolveStoreDir(home, dirRel, dirRel)
	if err != nil {
		return WorkspaceFile{}, false, err
	}
	if err := recordCatalogCreated(catalogAbs, catalogID, journal.NewFile(systemPath(home))); err != nil {
		return WorkspaceFile{}, false, err
	}
	file, err = ReadWorkspace(home)
	return file, true, err
}

func hasCatalog(file WorkspaceFile, catalogID string) bool {
	for _, item := range file.Catalogs {
		if item.ID == catalogID {
			return true
		}
	}
	return false
}

func ReadWorkspace(home string) (WorkspaceFile, error) {
	if !homeReady(home) {
		return WorkspaceFile{}, missingHome(home)
	}
	stores, err := ReadStores(home)
	if err != nil {
		return WorkspaceFile{}, err
	}
	return discoverHome(home, stores)
}

func readLegacyWorkspaceFile(home string) (WorkspaceFile, error) {
	path := workspacePath(home)
	if _, err := os.Stat(path); err != nil {
		return WorkspaceFile{}, err
	}
	var raw WorkspaceFile
	if err := jsonfile.Read(path, &raw); err != nil {
		return WorkspaceFile{}, err
	}
	return raw, nil
}

func addCatalogDir(home, abs string, byID map[string]WorkspaceCatalog) {
	if !isGitDir(abs) {
		return
	}
	id, err := catalog.PeekID(abs)
	if err != nil || id == "" {
		id, _, err = local.ReadFileGitStamp(abs)
		if err != nil || id == "" {
			return
		}
	}
	if _, ok := byID[id]; ok {
		return
	}
	byID[id] = WorkspaceCatalog{ID: id, Dir: homeRel(home, abs)}
}

func scanCatalogRoot(home, rootAbs string, byID map[string]WorkspaceCatalog) {
	entries, err := os.ReadDir(rootAbs)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		addCatalogDir(home, filepath.Join(rootAbs, e.Name()), byID)
	}
}

func sortCatalogs(catalogs []WorkspaceCatalog) {
	sort.Slice(catalogs, func(i, j int) bool {
		return catalogs[i].ID < catalogs[j].ID
	})
}

func discoverHome(home string, stores StoresFile) (WorkspaceFile, error) {
	byID := map[string]WorkspaceCatalog{}
	seenRoots := map[string]bool{}
	scanNamed := func(dir, fallback string) {
		abs, err := resolveStoreDir(home, dir, fallback)
		if err != nil || seenRoots[abs] {
			return
		}
		seenRoots[abs] = true
		scanCatalogRoot(home, abs, byID)
	}
	scanNamed(stores.Layout.Catalogs, defaultCatalogsDir)
	scanNamed(defaultCatalogsDir, defaultCatalogsDir)
	scanNamed(legacyCatalogsDir, legacyCatalogsDir)
	if stores.Layout.Catalog != "" {
		if abs, err := resolveStoreDir(home, stores.Layout.Catalog, stores.Layout.Catalog); err == nil {
			addCatalogDir(home, abs, byID)
		}
	}
	if abs, err := resolveStoreDir(home, legacyCatalogDir, legacyCatalogDir); err == nil {
		addCatalogDir(home, abs, byID)
	}

	catalogAbs := map[string]bool{}
	for _, c := range byID {
		if abs, err := resolveStoreDir(home, c.Dir, c.Dir); err == nil {
			catalogAbs[abs] = true
		}
	}

	reposByID := map[string]WorkspaceRepo{}
	seenRepoRoots := map[string]bool{}
	scanRepos := func(dir, fallback string) {
		repoRoot, err := resolveStoreDir(home, dir, fallback)
		if err != nil || seenRepoRoots[repoRoot] {
			return
		}
		seenRepoRoots[repoRoot] = true
		entries, _ := os.ReadDir(repoRoot)
		for _, e := range entries {
			if !e.IsDir() || e.Name() == "_catalog" || e.Name() == "_catalogs" {
				continue
			}
			abs := filepath.Join(repoRoot, e.Name())
			if catalogAbs[abs] {
				continue
			}
			if !isGitDir(abs) {
				id, dsn, err := gitea.ReadStamp(abs)
				if err != nil || id == "" {
					continue
				}
				if _, ok := reposByID[id]; ok {
					continue
				}
				reposByID[id] = WorkspaceRepo{ID: id, Dir: homeRel(home, abs), Driver: "gitea", DSN: dsn}
				continue
			}
			if _, err := catalog.PeekID(abs); err == nil {
				continue
			}
			id, driver, err := local.ReadFileGitStamp(abs)
			if err != nil || id == "" {
				continue
			}
			if _, ok := reposByID[id]; ok {
				continue
			}
			reposByID[id] = WorkspaceRepo{ID: id, Dir: homeRel(home, abs), Driver: driver}
		}
	}
	scanRepos(stores.Layout.Repos, defaultReposDir)
	scanRepos(defaultReposDir, defaultReposDir)

	legacy, legacyErr := readLegacyWorkspaceFile(home)
	if legacyErr == nil {
		for _, c := range legacy.Catalogs {
			if c.ID == "" {
				continue
			}
			if _, ok := byID[c.ID]; !ok {
				byID[c.ID] = c
			}
		}
		for _, r := range legacy.Repos {
			if r.ID == "" {
				continue
			}
			if _, ok := reposByID[r.ID]; !ok {
				reposByID[r.ID] = r
			}
		}
	}

	catalogs := make([]WorkspaceCatalog, 0, len(byID))
	for _, c := range byID {
		catalogs = append(catalogs, c)
	}
	sortCatalogs(catalogs)
	repos := make([]WorkspaceRepo, 0, len(reposByID))
	for _, r := range reposByID {
		repos = append(repos, r)
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].ID < repos[j].ID })

	out := WorkspaceFile{
		Catalogs: catalogs,
		Repos:    repos,
	}
	if legacyErr == nil {
		out.IndexDriver = legacy.IndexDriver
		out.IndexURL = legacy.IndexURL
	}
	return out, nil
}

func Open(home string) (*OpenWorkspace, error) {
	if err := migrateLegacyJSONCatalog(home); err != nil {
		return nil, err
	}
	file, err := ReadWorkspace(home)
	if err != nil {
		return nil, err
	}
	if len(file.Catalogs) == 0 {
		return nil, fmt.Errorf("no catalog at %s; run: kc init --home %s", home, home)
	}
	stores, err := ReadStores(home)
	if err != nil {
		return nil, err
	}
	if err := stores.validateProfile(); err != nil {
		return nil, err
	}
	store := repository.NewStore()
	for _, repo := range file.Repos {
		if repo.DSN != "" {
			if err := repository.RejectConfiguredSecret(repo.Driver, repo.DSN, "KC_GITEA_TOKEN"); err != nil {
				return nil, fmt.Errorf("repository %s: %w", repo.ID, err)
			}
		}
		opened, err := openMountedRepo(home, repo, stores)
		if err != nil {
			return nil, err
		}
		if err := store.Add(opened); err != nil {
			return nil, err
		}
		if err := bindCollocatedStream(store, opened); err != nil {
			return nil, err
		}
	}
	catalogs := map[string]*catalog.Catalog{}
	registries := map[string]*catalog.Registry{}
	for _, item := range file.Catalogs {
		registry, err := catalog.NewRegistry(filepath.Join(home, item.Dir), item.ID)
		if err != nil {
			return nil, err
		}
		cat, err := catalog.NewCatalog(store, registry)
		if err != nil {
			return nil, err
		}
		catalogs[item.ID] = cat
		registries[item.ID] = registry
	}
	defaultID := file.Catalogs[0].ID
	w, err := writer.NewWriter(store, writer.NewFileIdempotencyStore(filepath.Join(home, "writer.json")))
	if err != nil {
		return nil, err
	}
	controlStore := controlplane.NewFileControlState(filepath.Join(home, "control.json"))
	bundle, err := controlStore.LoadBundle()
	if err != nil {
		return nil, err
	}
	if legacy, ok := bundle[""]; ok {
		delete(bundle, "")
		bundle[defaultID] = legacy
	}
	if _, ok := bundle[defaultID]; !ok {
		bundle[defaultID] = controlplane.EmptyControlState
	}
	sys := journal.NewFile(systemPath(home))
	w.SetJournal(sys)
	rd := reader.NewReader(store)
	rd.SetJournal(sys)
	for _, cat := range catalogs {
		cat.SetJournal(sys)
	}
	plane := controlplane.New(store, w, catalogs[defaultID])
	plane.SetJournal(sys)
	idxDir, err := resolveStoreDir(home, stores.Layout.Projections, defaultProjectionsDir)
	if err != nil {
		return nil, err
	}
	idx := index.NewIndexEngine(idxDir, indexOpener(file, stores))
	ws := &OpenWorkspace{
		Home:         home,
		Store:        store,
		Writer:       w,
		Reader:       rd,
		Catalogs:     catalogs,
		Registries:   registries,
		Catalog:      catalogs[defaultID],
		Registry:     registries[defaultID],
		ControlPlane: plane,
		ControlStore: controlStore,
		File:         file,
		Control:      bundle[defaultID],
		Controls:     bundle,
		controlID:    defaultID,
		Journal:      sys,
		Index:        idx,
		Stores:       stores,
	}
	ws.wireSidecars()
	return ws, nil
}

// Close releases projection engines and member store adapters opened for this process.
func (ws *OpenWorkspace) Close() error {
	if ws == nil {
		return nil
	}
	var first error
	if ws.Index != nil {
		if err := ws.Index.Close(); err != nil {
			first = err
		}
	}
	if ws.Store != nil {
		if err := ws.Store.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func PersistControl(ws *OpenWorkspace) error {
	if ws.Controls == nil {
		ws.Controls = map[string]controlplane.ControlState{}
	}
	ws.Controls[ws.controlID] = ws.Control
	return ws.ControlStore.SaveBundle(ws.Controls)
}

func (ws *OpenWorkspace) bindControl(catalogID string) {
	if catalogID == "" {
		catalogID = ws.File.Catalogs[0].ID
	}
	st, ok := ws.Controls[catalogID]
	if !ok {
		st = controlplane.EmptyControlState
	}
	if st.Proposals == nil {
		st.Proposals = map[string]controlplane.Proposal{}
	}
	if st.Previews == nil {
		st.Previews = map[string]controlplane.PreviewGeneration{}
	}
	if st.Validations == nil {
		st.Validations = map[string]controlplane.ValidationReport{}
	}
	ws.Controls[catalogID] = st
	ws.Control = st
	ws.controlID = catalogID
}

func (ws *OpenWorkspace) UseCatalog(catalogID string) (*catalog.Catalog, *catalog.Registry, error) {
	if catalogID == "" {
		return ws.Catalog, ws.Registry, nil
	}
	cat, ok := ws.Catalogs[catalogID]
	if !ok {
		return nil, nil, fmt.Errorf("unknown catalog %s", catalogID)
	}
	return cat, ws.Registries[catalogID], nil
}

func AddCatalog(ws *OpenWorkspace, catalogID string) error {
	if catalogID == "" {
		return fmt.Errorf("catalog id is required")
	}
	for _, item := range ws.File.Catalogs {
		if item.ID == catalogID {
			return fmt.Errorf("catalog already exists: %s", catalogID)
		}
	}
	for _, r := range ws.File.Repos {
		if r.ID == catalogID {
			return fmt.Errorf("%s is already a member repository", catalogID)
		}
	}
	dir := catalogDirOf(ws.Stores, catalogID)
	registry, err := catalog.NewRegistry(filepath.Join(ws.Home, dir), catalogID)
	if err != nil {
		return err
	}
	cat, err := catalog.NewCatalog(ws.Store, registry)
	if err != nil {
		return err
	}
	cat.SetJournal(ws.Journal)
	cat.SetStamp(ws.Writer.Stamp())
	ws.attachIndex(cat)
	if err := cat.RecordCreated(); err != nil {
		return err
	}
	ws.File.Catalogs = append(ws.File.Catalogs, WorkspaceCatalog{ID: catalogID, Dir: dir})
	sortCatalogs(ws.File.Catalogs)
	ws.Catalogs[catalogID] = cat
	ws.Registries[catalogID] = registry
	return nil
}

func (ws *OpenWorkspace) mountRepository(repositoryID, driver, dsn string) (repository.Repository, error) {
	for _, item := range ws.File.Catalogs {
		if repositoryID == item.ID {
			return nil, fmt.Errorf("%s is reserved for a Catalog registry", repositoryID)
		}
	}
	for _, r := range ws.File.Repos {
		if r.ID == repositoryID {
			return nil, fmt.Errorf("repository already registered: %s", repositoryID)
		}
	}
	if driver == "" {
		driver = ws.Stores.Repository
	}
	driver = normalizeRepoDriver(driver)
	if driver == "redis" {
		return nil, errRedisNotRepository()
	}
	if driver == "stream" {
		return nil, errStreamNotRepository()
	}
	if driver == "mysql" {
		return nil, errMySQLNotWarehouse("repository")
	}
	if driver == "postgres" {
		return nil, errPostgresRemoved()
	}
	if driver != "filegit" && driver != "dolt" && driver != "gitea" {
		return nil, fmt.Errorf("unknown repository driver %s", driver)
	}
	item := WorkspaceRepo{ID: repositoryID, Driver: driver}
	if driver == "filegit" || driver == "dolt" {
		root := ws.Stores.Layout.Repos
		if root == "" {
			root = defaultReposDir
		}
		item.Dir = filepath.ToSlash(filepath.Join(root, EncodeRepoDir(repositoryID)))
	}
	if driver == "gitea" {
		if strings.TrimSpace(dsn) == "" {
			return nil, fmt.Errorf("gitea repo-add requires --dsn http(s)://host/owner/name")
		}
		if err := repository.RejectConfiguredSecret("gitea", dsn, gitea.EnvToken); err != nil {
			return nil, err
		}
		root := ws.Stores.Layout.Repos
		if root == "" {
			root = defaultReposDir
		}
		item.Dir = filepath.ToSlash(filepath.Join(root, EncodeRepoDir(repositoryID)))
		item.DSN = dsn
	}
	repo, err := openMountedRepo(ws.Home, item, ws.Stores)
	if err != nil {
		return nil, err
	}
	if err := ws.Store.Add(repo); err != nil {
		return nil, err
	}
	if err := bindCollocatedStream(ws.Store, repo); err != nil {
		return nil, err
	}
	if driver == "gitea" {
		abs, err := resolveStoreDir(ws.Home, item.Dir, item.Dir)
		if err != nil {
			return nil, err
		}
		if err := gitea.WriteStamp(abs, repositoryID, dsn); err != nil {
			return nil, err
		}
	}
	ws.File.Repos = append(ws.File.Repos, item)
	return repo, nil
}

func AddRepository(ws *OpenWorkspace, repositoryID, driver, dsn string) (kernel.CommitID, error) {
	if normalizeRepoDriver(driver) == "redis" || strings.HasPrefix(strings.TrimSpace(dsn), "redis://") || strings.HasPrefix(strings.TrimSpace(dsn), "rediss://") {
		return "", errRedisNotRepository()
	}
	if strings.TrimSpace(dsn) != "" && normalizeRepoDriver(driver) != "gitea" {
		if err := applyDSN(&ws.Stores, driver, dsn); err != nil {
			return "", err
		}
		if err := WriteStores(ws.Home, ws.Stores); err != nil {
			return "", err
		}
	}
	repo, err := ws.mountRepository(repositoryID, driver, dsn)
	if err != nil {
		return "", err
	}
	if ws.Catalog != nil {
		if err := ws.Catalog.RegisterRepository(kernel.RepositoryID(repositoryID)); err != nil {
			return "", err
		}
	}
	return repo.Head("refs/heads/main")
}

func openMountedRepo(home string, repo WorkspaceRepo, stores StoresFile) (repository.Repository, error) {
	id := kernel.RepositoryID(repo.ID)
	dir := repo.Dir
	if dir == "" {
		root := stores.Layout.Repos
		if root == "" {
			root = defaultReposDir
		}
		dir = filepath.ToSlash(filepath.Join(root, EncodeRepoDir(repo.ID)))
	}
	abs, err := resolveStoreDir(home, dir, dir)
	if err != nil {
		return nil, err
	}
	switch normalizeRepoDriver(repo.Driver) {
	case "filegit":
		return local.NewFileGit(abs, id)
	case "dolt":
		return scale.OpenDolt(abs, id)
	case "gitea":
		if strings.TrimSpace(repo.DSN) == "" {
			return nil, fmt.Errorf("gitea repository %s is missing dsn", repo.ID)
		}
		return gitea.Open(id, repo.DSN, os.Getenv(gitea.EnvToken))
	case "stream":
		return nil, errStreamNotRepository()
	case "postgres":
		return nil, errPostgresRemoved()
	case "redis":
		return nil, errRedisNotRepository()
	default:
		return nil, fmt.Errorf("unknown repository driver %s", repo.Driver)
	}
}

func bindCollocatedStream(store *repository.Store, opened repository.Repository) error {
	rooted, ok := opened.(interface{ RootDir() string })
	if !ok {
		return nil
	}
	return store.AddStream(opened.ID(), local.NewJSONLStream(rooted.RootDir(), opened.ID()))
}

func indexOpener(file WorkspaceFile, stores StoresFile) index.EngineOpener {
	driver := stores.Index
	if driver == "" {
		driver = file.IndexDriver
	}
	if driver == "" {
		driver = os.Getenv("KC_INDEX_DRIVER")
	}
	driver = normalizeIndexDriver(driver)
	profile := stores.Profile
	if profile == "" {
		profile = "local"
	}
	if profile != "scale" && driver == "redis" {
		return func(string, kernel.RepositoryID) (index.Engine, error) {
			return nil, errRedisNotLocal()
		}
	}
	switch driver {
	case "sqlite":
		return local.OpenSQLite
	case "elasticsearch":
		return scale.OpenElasticsearch(stores.Elasticsearch)
	case "redis":
		return scale.OpenRedis(stores.Redis)
	default:
		return func(string, kernel.RepositoryID) (index.Engine, error) {
			return nil, fmt.Errorf("unknown index driver %s", driver)
		}
	}
}

func recordCatalogCreated(dir, catalogID string, j journal.Journal) error {
	registry, err := catalog.NewRegistry(dir, catalogID)
	if err != nil {
		return err
	}
	cat, err := catalog.NewCatalog(repository.NewStore(), registry)
	if err != nil {
		return err
	}
	cat.SetJournal(j)
	return cat.RecordCreated()
}

func migrateLegacyJSONCatalog(home string) error {
	jsonPath := filepath.Join(home, "catalog.json")
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		return nil
	}
	body, err := os.ReadFile(jsonPath)
	if err != nil {
		return err
	}
	var raw catalog.CatalogState
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	if raw.Views == nil {
		raw.Views = []catalog.ViewDefinition{}
	}
	if len(raw.Views) == 0 {
		return nil
	}
	catalogID := catalog.DefaultCatalogID
	if _, err := os.Stat(workspacePath(home)); err == nil {
		ws, err := ReadWorkspace(home)
		if err != nil {
			return err
		}
		if len(ws.Catalogs) > 0 {
			catalogID = ws.Catalogs[0].ID
		}
	}
	stores, err := ReadStores(home)
	if err != nil {
		return err
	}
	dir := stores.Layout.Catalog
	if dir == "" {
		dir = catalogDirOf(stores, catalogID)
	}
	catalogAbs, err := resolveStoreDir(home, dir, dir)
	if err != nil {
		return err
	}
	git, err := catalog.NewRegistry(catalogAbs, catalogID)
	if err != nil {
		return err
	}
	head, err := git.Repo().Head("")
	if err != nil {
		return err
	}
	listed, err := git.Repo().List(head)
	if err != nil {
		return err
	}
	if len(listed) == 0 {
		return git.Save(raw, "catalog: import catalog.json", "", "", "")
	}
	return nil
}
