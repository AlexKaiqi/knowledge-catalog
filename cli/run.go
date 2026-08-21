package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"kc/catalog"
	"kc/controlplane"
	"kc/internal/journal"
	"kc/kernel"
	"kc/reader"
	"kc/repository"
	"kc/writer"
)

const Help = `kc — Knowledge Catalog CLI (protocol verbs)

Workspace
  kc init --home <dir> [--catalog <id>]       first Catalog. id is kr://<org>/<name> or <org>/<name>
                                              (omit → kr://local/catalog). Output: {catalog}.
                                              看它：kc read --catalog / kc audit
  kc catalog-add --home <dir> --catalog <id>  another Catalog (own registry git under catalogs/)
  kc register --home <dir> [--catalog <id>] --repo <id>
                                              admit a mounted repo into a Catalog
  kc status --home <dir>                      catalogs + mounted repos + views + local stores
  kc read --catalog [<id>]                    this Catalog's current combination space
                                              Output: CatalogState {catalogId, repositories, views}
                                              Not knowledge. History is kc audit.
  kc serve --home <dir> [--listen 127.0.0.1:7380]
                                              HTTP facade: same verbs as this CLI. UI at GET /
                                              POST /v1/<verb> JSON flags (no --home). X-Kc-As → --as; X-Kc-Request-Id → --request-id
  kc audit --home <dir> [--catalog <id>] [--cmd <verb>] [--limit N]
                                              Catalog 登记表 git 历史（define-view / register / retire-view）
  kc audit --layer kc|system                  本机过程账：audit.jsonl / system.jsonl

Repository (authority store; Catalogs combine these, do not own them)
  kc repo-add --home <dir> --repo <kr://...> [--driver filegit|dolt|gitea] [--dsn <url>]
                                              default --driver is stores.yaml repository (filegit on local profile).
                                              gitea --dsn is http(s)://host/owner/name; token is KC_GITEA_TOKEN.
                                              stream is not a snapshot repo.
                                              --driver mysql is refused.
                                              --dsn is non-secret. Passwords/tokens are env.
  kc store-set --home <dir> [--profile local|scale]
                    [--repository filegit|dolt|gitea] [--index sqlite|elasticsearch] [--cache redis]
                    [--repos-dir --catalogs-dir --projections-dir]
                    [--driver redis|elasticsearch|starrocks|filegit|sqlite|dolt|gitea] [--host --port --database --user --url --dsn]
                                              engines in stores.yaml; dirs in layout.yaml.
                                              --catalogs-dir is the parent of per-id registry gits.
                                              local: FileGit + JSONL + SQLite, no Redis.
                                              scale: Dolt + stream + ES + StarRocks + Redis cache (Dolt/stream/SR stubbed).
  kc store-ls       --home <dir>              layout.yaml + stores.yaml (never prints secrets)
  content write: put / commit / append take --repo
  content consume: read / list / search / log take --view (no --repo/--commit/--ref)

Writer (mutates one Repository; Catalog does not store knowledge)
  kc put            --home <dir> --command-id <id> --repo <id> --object <id>
                    [--aspect <name>] [--member <key>] [--file <json>|--value <json>]
                    [--ref refs/heads/main] [--expected <commit>] [--schema-ref <ref>]
                    [--if-absent | --if-digest <digest>]
                    [--origin-kind SOURCE] [--source-ref <s>] [--actor-ref <s>]
                    [--input-vrv <ref> --algorithm-spec|--algorithm-model|--algorithm-hash]
                    PUT one Address, then COMMIT. Output: CommitReceipt
  kc remove         same targeting flags without value. Output: CommitReceipt
  kc commit         --home <dir> --command-id <id> --changeset <file.json>
                    ChangeSet or ingest preview. Omits base/expected → current Ref. Output: CommitReceipt
  kc ingest         --home <dir> --repo <id> --dir <path> [--out <changeset.json>]
                    Preview only (frontmatter object_id wins). Then kc commit --changeset
  kc receipt        --home <dir> --command-id <id>
                    Lookup Writer idempotency log. Output: IdempotencyEntry
  kc append         --home <dir> --command-id <id> --repo <id> --stream <name>
                    --event-id <id> [--payload <json>|--file <json>] [--cursor <n>]
                    [--schema-ref <ref>]
                    Empty --cursor is filled from the current stream (retry reuses it)
                    Output: AppendReceipt

Consumer (View serving: --view; do not pass --repo / --commit / --ref)
  kc read           --home <dir> [--catalog <id>] --view <id> --object <id>
                    optional --aspect / --include / --exclude
                    Output: FederatedValue[]  (selectors resolved at open; union, no override)
  kc list           --home <dir> [--catalog <id>] --view <id>
                    Output: FederatedValue[]
  kc search         --home <dir> [--catalog <id>] --view <id>
                    [--query <text>] [--match path=text]
                    [--eq|--neq|--gt|--gte|--lt|--lte path=value]
                    [--in path=v1,v2] [--exists path] [--sort path[:asc|:desc]]
                    Output: KnowledgeValue[]  (member indexes at the resolved commits; hydrate Canonical)
  kc provenance     --home <dir> [--catalog <id>] --view <id> --object <id>
                    Output: ProvenanceTrace[]
  kc log            --home <dir> [--catalog <id>] --view <id> --object <id>
                    Output: ObjectLog[]  (object history at the resolved commits; not Catalog git)
  kc describe-schema --home <dir> [--catalog <id>] --view <id> [--object]
                    Output: SchemaReport[]
  kc resolve        --home <dir> [--catalog <id>] --view <id> --object <id>
                    Output: Resolution[]

Reader (maintainer: must name --repo and --commit or --ref)
  kc resolve        --home <dir> --repo <id> --object <id> (--commit <id>|--ref <ref>)
                    Output: Resolution (no body)
  kc read           same + optional --aspect / --include / --exclude
                    Output: KnowledgeValue
  kc provenance     same as resolve
                    Output: ProvenanceTrace { repository, commit, objectId, chain }
  kc stream         --home <dir> --repo <id> --stream <name>
                    [--from-cursor <c>] [--limit N]   continue (resume; cursor is opaque)
                    [--event-id <id>]                 lookup
                    Output: StreamPage
  kc list           --home <dir> --repo <id> (--commit <id>|--ref <ref>)
                    Output: KnowledgeValue[]
  kc describe-schema --home <dir> --repo <id> (--commit <id>|--ref <ref>) [--object]
                    Output: SchemaReport (schema/* AccessHints; --object follows schema_ref)
  kc search         --home <dir> --repo <id>
                    [--query <text>] [--match path=text]
                    [--eq|--neq|--gt|--gte|--lt|--lte path=value]
                    [--in path=v1,v2] [--exists path] [--sort path[:asc|:desc]]
                    [--commit <id>|--ref <ref>]  (defaults to index basis; Ensure if named)
                    Output: KnowledgeValue[]  (atomic clauses, implicit AND; hydrate Canonical)
  kc describe-index --home <dir> --repo <id>
                    Output: IndexDescriptor (basis / lag / compiled AccessHints)
  kc index-sync     --home <dir> --repo <id> (--commit <id>|--ref <ref>)
                    Output: IndexSync (incremental if possible, else rebuild)
  kc log            --home <dir> --repo <id> --object <id> (--commit <id>|--ref <ref>)
                    Output: ObjectRevision[]  (collapsed history)
  kc diff           --home <dir> --repo <id> --object <id> --from <commit> --to <commit>
                    Output: ObjectDiff

Catalog (combination space; --catalog selects which; omit when only one)
  kc define-view    --home <dir> [--catalog <id>] --view <id> --revision <n>
                    --source <repo>=<selector>   (repeatable; repo must be mounted)
                    Output: ViewDefinition (Catalog registry)
  kc index-plan     --home <dir> [--catalog <id>] --view <id>
                    Output: IndexPlan (derived recipe at current selectors). Working index compiles on put/commit/merge.
  kc retire-view    --home <dir> [--catalog <id>] --view <id>
  kc archive-catalog --home <dir> [--catalog <id>]
  kc archive-repo   --home <dir> --repo <id>

Access (empty allow.json = workspace owner; --as must match a rule)
  kc allow          --home <dir> --principal <who> --cmd <verbs>
                    (--repo <id>|--catalog <id>) [optional --ref --object --aspect --stream --release]
  kc revoke         --home <dir> --id <rule-id>
  kc allowed        --home <dir> [--principal|--as] [--cmd ...]
  kc whoami         --home <dir> [--as]
  verbs also take     --as <principal> [--request-id <token>]
                    Catalog / 仓库的 git commit 记下 as / request-id / 命中的 ruleId。

Hook (outbound; .kc/hooks.json. pre = --run fail closed; post = pointers, must not roll back)
  kc hook-add       --home <dir> --on <cmd> --phase pre|post
                    [--repo <id>|--catalog <id>] (--run <path>|--url <url>)
  kc hook-ls        --home <dir> [--on] [--repo|--catalog]
  kc hook-rm        --home <dir> --id <hook-id>

Gate (inbound checklist on merge; .kc/gates.json. not a hook)
  kc gate-add       --home <dir> --on merge --require validate,suite:<name>
                    --repo <id>
  kc gate-ls        --home <dir> [--on] [--repo|--catalog]
  kc gate-rm        --home <dir> --id <gate-id>

Control Plane (content still goes through Writer)
  kc propose        --home <dir> --proposal-id <id> --repo <id>
                    --target <ref> --candidate <ref>
                    PUT flags (--object --value/--file [--aspect]) or --changeset
                    Output: Proposal  (writes candidate Ref only)
  kc preview        --home <dir> [--catalog <id>] --proposal <id> --view <id>
                    Output: PreviewGeneration (View + overlay; stored in ControlState)
  kc validate       --home <dir> [--catalog <id>] --preview <id>
                    Structural check (mounted repos + commits exist), then records outcome
  kc record-validation --home <dir> [--catalog <id>] --preview <id>
                    --suite <rev> --outcome PASSED|FAILED
                    Records an external suite outcome; does not run tests
  kc merge          --home <dir> --proposal <id> --preview <id> [--validation <id>]
                    Fast-forwards target Ref. Next read --view follows the published branch.
                    --validation required unless a matching merge gate is configured.

Default --home is ./.kc
Connection: .kc/layout.yaml (this machine's dirs) + .kc/stores.yaml (engines + hosts).
Two store stacks, same public interfaces (repository.Repository, index.Engine):
  local  — FileGit + JSONL authority, SQLite index. No Redis.
  scale  — Dolt snapshot + ordered-stream APPEND (stubs), ES full-text, StarRocks columns (stub), Redis cache only.
Catalog registry is always FileGit under layout.catalogs/<encoded-id>. Redis is discardable hot tail, not a warehouse, not GT.
profile: local rejects redis as index or cache. scale may set cache: redis.
Managed hosts (redis/elasticsearch/starrocks) are optional stores.yaml sections; secrets are KC_REDIS_PASSWORD,
KC_ELASTICSEARCH_PASSWORD or KC_ELASTICSEARCH_API_KEY, KC_STARROCKS_PASSWORD. --dsn / stores.yaml must not contain passwords.
kc store-set writes both files; repo-add --dsn merges non-secret URL fields into stores.yaml.
Index: "index: sqlite" (local FTS+fields) | "index: elasticsearch" (scale full-text). Redis is not a repository; miss must read the store.
kc serve pins that home; POST /v1/<verb> JSON uses CLI flag names without the leading --.
X-Kc-As is --as. X-Kc-Request-Id is --request-id. This is a local owner facade, not a production gateway.
Writes require --command-id (retry = same id + same body; content change = new id).
First Catalog is layout.catalogs/<encoded-id> (default <home>/catalogs/…);
more catalogs are siblings under the same parent. --catalog selects which; omit it when there is only one.
FileGit repositories are layout.repos/<encoded-id>. SQLite projections are layout.projections.
Writer log: <home>/writer.json
kc log: <home>/audit.jsonl          本机 facade（kc audit --layer kc）
system log: <home>/system.jsonl     本机协议过程账（kc audit --layer system）
Catalog 当前态: kc read --catalog. 历史: 登记表 git（kc audit）
`

type RunResult struct {
	Status int
	Stdout string
}

func jsonOut(value any) string {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("{\n  \"error\": {\n    \"message\": %q\n  }\n}\n", err.Error())
	}
	return string(b) + "\n"
}

func decodeChangeSet(body []byte, label string) (repository.CommitChangeSet, error) {
	var raw repository.CommitChangeSet
	if err := json.Unmarshal(body, &raw); err != nil {
		return repository.CommitChangeSet{}, fmt.Errorf("%s is not valid JSON", label)
	}
	if raw.TargetRepository != "" && raw.Operations != nil {
		return raw, nil
	}
	var wrapped struct {
		ChangeSet repository.CommitChangeSet `json:"changeSet"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil || wrapped.ChangeSet.TargetRepository == "" || wrapped.ChangeSet.Operations == nil {
		return repository.CommitChangeSet{}, fmt.Errorf("changeset must include targetRepository and operations")
	}
	return wrapped.ChangeSet, nil
}

func parseJSON(text, label string) (any, error) {
	var value any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON", label)
	}
	return value, nil
}

func loadJSONFlag(flags map[string]FlagValue, label string) (any, bool, error) {
	file := FlagString(flags, "file")
	raw := FlagString(flags, "value")
	if raw == "" {
		raw = FlagString(flags, "payload")
	}
	if file != "" && raw != "" {
		return nil, false, fmt.Errorf("use only one of --file or --value/--payload")
	}
	if file != "" {
		body, err := os.ReadFile(file)
		if err != nil {
			return nil, false, err
		}
		value, err := parseJSON(string(body), file)
		return value, true, err
	}
	if raw != "" {
		value, err := parseJSON(raw, label)
		return value, true, err
	}
	return nil, false, nil
}

func addressFrom(flags map[string]FlagValue) (kernel.Address, error) {
	objectID, err := RequireFlag(flags, "object")
	if err != nil {
		return kernel.Address{}, err
	}
	aspect := FlagString(flags, "aspect")
	member := FlagString(flags, "member")
	if member != "" {
		if aspect == "" {
			return kernel.Address{}, fmt.Errorf("Member address requires --aspect and --member")
		}
		return kernel.Address{Kind: kernel.KindMember, ObjectID: kernel.ObjectID(objectID), AspectName: aspect, MemberKey: member}, nil
	}
	if aspect != "" {
		return kernel.Address{Kind: kernel.KindAspect, ObjectID: kernel.ObjectID(objectID), AspectName: aspect}, nil
	}
	kind := kernel.AddressKind(FlagString(flags, "kind"))
	if kind == "" {
		kind = kernel.KindEntity
	}
	return kernel.Address{Kind: kind, ObjectID: kernel.ObjectID(objectID)}, nil
}

func pinCommit(ws *OpenWorkspace, flags map[string]FlagValue) (kernel.RepositoryID, kernel.CommitID, error) {
	repositoryID, err := RequireFlag(flags, "repo")
	if err != nil {
		return "", "", err
	}
	repo, ok := ws.Store.Get(kernel.RepositoryID(repositoryID))
	if !ok {
		return "", "", fmt.Errorf("unknown repository %s", repositoryID)
	}
	if commit := FlagString(flags, "commit"); commit != "" {
		return kernel.RepositoryID(repositoryID), kernel.CommitID(commit), nil
	}
	ref := FlagString(flags, "ref")
	if ref == "" {
		ref = "refs/heads/main"
	}
	commitID, ok := repo.GetRef(ref)
	if !ok {
		return "", "", fmt.Errorf("unresolved ref %s in %s", ref, repositoryID)
	}
	return kernel.RepositoryID(repositoryID), commitID, nil
}

func originFrom(flags map[string]FlagValue) *kernel.ProvenanceEnvelope {
	originKind := FlagString(flags, "origin-kind")
	sourceRefs := FlagStrings(flags, "source-ref")
	evidenceRefs := FlagStrings(flags, "evidence-ref")
	actor := FlagString(flags, "actor-ref")
	activity := FlagString(flags, "activity-ref")
	inputVRV := FlagString(flags, "input-vrv")
	spec := FlagString(flags, "algorithm-spec")
	model := FlagString(flags, "algorithm-model")
	hash := FlagString(flags, "algorithm-hash")
	produced := FlagString(flags, "produced-at")
	if originKind == "" && len(sourceRefs) == 0 && len(evidenceRefs) == 0 && actor == "" && activity == "" && inputVRV == "" && spec == "" && model == "" && hash == "" && produced == "" {
		return nil
	}
	kind := kernel.OriginKind(originKind)
	if kind == "" {
		kind = kernel.OriginSource
	}
	env := &kernel.ProvenanceEnvelope{
		OriginKind:              kind,
		ActorRef:                actor,
		ActivityRef:             activity,
		SourceRefs:              sourceRefs,
		EvidenceRefs:            evidenceRefs,
		InputViewReadVersionRef: inputVRV,
		ProducedAt:              produced,
	}
	if spec != "" || model != "" || hash != "" {
		env.Algorithm = &kernel.AlgorithmRef{
			DerivationSpecRef: spec,
			ModelRef:          model,
			CodeHash:          hash,
		}
	}
	return env
}

func preconditionFrom(flags map[string]FlagValue) (*repository.Precondition, error) {
	ifAbsent := FlagBool(flags, "if-absent")
	digest := FlagString(flags, "if-digest")
	if ifAbsent && digest != "" {
		return nil, fmt.Errorf("use only one of --if-absent or --if-digest")
	}
	if ifAbsent {
		return &repository.Precondition{Type: repository.IfAbsent}, nil
	}
	if digest != "" {
		return &repository.Precondition{Type: repository.IfDigestEquals, Digest: kernel.Digest(digest)}, nil
	}
	return nil, nil
}

func writeOperation(flags map[string]FlagValue, op repository.OpKind, value any) (repository.Operation, error) {
	address, err := addressFrom(flags)
	if err != nil {
		return repository.Operation{}, err
	}
	pre, err := preconditionFrom(flags)
	if err != nil {
		return repository.Operation{}, err
	}
	return repository.Operation{
		Op:           op,
		Address:      address,
		Value:        value,
		PathHint:     FlagString(flags, "path-hint"),
		SchemaRef:    FlagString(flags, "schema-ref"),
		Precondition: pre,
	}, nil
}

func proposeOperations(flags map[string]FlagValue) ([]repository.Operation, error) {
	file := FlagString(flags, "changeset")
	if file != "" {
		body, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		var asOps []repository.Operation
		if json.Unmarshal(body, &asOps) == nil && len(asOps) > 0 {
			return asOps, nil
		}
		var wrapped struct {
			Operations []repository.Operation `json:"operations"`
		}
		if err := json.Unmarshal(body, &wrapped); err != nil || len(wrapped.Operations) == 0 {
			return nil, fmt.Errorf("changeset must include operations")
		}
		return wrapped.Operations, nil
	}
	value, ok, err := loadJSONFlag(flags, "--value")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("propose requires --file/--value or --changeset")
	}
	op, err := writeOperation(flags, repository.OpPut, value)
	if err != nil {
		return nil, err
	}
	return []repository.Operation{op}, nil
}

func commitOne(ws *OpenWorkspace, flags map[string]FlagValue, operations []repository.Operation) (any, error) {
	repositoryID, err := RequireFlag(flags, "repo")
	if err != nil {
		return nil, err
	}
	if _, ok := ws.Store.Get(kernel.RepositoryID(repositoryID)); !ok {
		return nil, fmt.Errorf("unknown repository %s", repositoryID)
	}
	targetRef := FlagString(flags, "ref")
	if targetRef == "" {
		targetRef = "refs/heads/main"
	}
	commandID, err := RequireFlag(flags, "command-id")
	if err != nil {
		return nil, err
	}
	return ws.Writer.CommitIntent(commandID, writer.CommitIntent{
		TargetRepository:     kernel.RepositoryID(repositoryID),
		TargetRef:            targetRef,
		BaseCommit:           kernel.CommitID(FlagString(flags, "base")),
		ExpectedTargetCommit: kernel.CommitID(FlagString(flags, "expected")),
		Operations:           operations,
		Message:              FlagString(flags, "message"),
		Provenance:           originFrom(flags),
	})
}

func resolveHome(flags map[string]FlagValue) (string, error) {
	home := FlagString(flags, "home")
	if home == "" {
		home = ".kc"
	}
	return filepath.Abs(home)
}

func handle(command string, flags map[string]FlagValue) (any, error) {
	home, err := resolveHome(flags)
	if err != nil {
		return nil, err
	}
	if _, err := requestIDFrom(flags); err != nil {
		return nil, err
	}

	if command == "help" || command == "--help" || command == "-h" || FlagBool(flags, "help") {
		return Help, nil
	}
	if err := rejectRetiredCommand(command); err != nil {
		return nil, err
	}
	if err := rejectRetiredFlags(flags); err != nil {
		return nil, err
	}

	if command == "init" {
		if FlagString(flags, "namespace") != "" {
			return nil, fmt.Errorf("init takes --catalog <id> (kr://acme/catalog or acme/catalog), not --namespace")
		}
		catalogID := FlagString(flags, "catalog")
		file, _, err := InitWorkspace(home, catalogID)
		if err != nil {
			return nil, err
		}
		id := catalogID
		if id != "" {
			id, err = NormalizeCatalogID(id)
			if err != nil {
				return nil, err
			}
		} else if len(file.Catalogs) > 0 {
			id = file.Catalogs[0].ID
		}
		return map[string]any{"catalog": id}, nil
	}

	if command == "catalog-add" {
		ws, err := Open(home)
		if err != nil {
			return nil, err
		}
		defer ws.Close()
		ws.observe(command, flags)
		catalogID, err := RequireFlag(flags, "catalog")
		if err != nil {
			return nil, err
		}
		if err := AddCatalog(ws, catalogID); err != nil {
			return nil, err
		}
		return map[string]any{"catalog": catalogID}, nil
	}

	if command == "store-set" {
		ws, err := Open(home)
		if err != nil {
			return nil, err
		}
		defer ws.Close()
		ws.observe(command, flags)
		updated, err := applyStoreFlags(ws.Stores, flags)
		if err != nil {
			return nil, err
		}
		if err := WriteStores(home, updated); err != nil {
			return nil, err
		}
		return PublicStores(updated), nil
	}

	if command == "store-ls" {
		if _, err := ReadWorkspace(home); err != nil {
			return nil, err
		}
		file, err := ReadStores(home)
		if err != nil {
			return nil, err
		}
		return PublicStores(file), nil
	}

	if command == "repo-add" {
		ws, err := Open(home)
		if err != nil {
			return nil, err
		}
		defer ws.Close()
		ws.observe(command, flags)
		repositoryID, err := RequireFlag(flags, "repo")
		if err != nil {
			return nil, err
		}
		head, err := AddRepository(ws, repositoryID, FlagString(flags, "driver"), FlagString(flags, "dsn"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"repositoryId": repositoryID, "head": head}, nil
	}

	if command == "audit" {
		if err := authorize(home, "audit", flags); err != nil {
			return nil, err
		}
		limit := 50
		if raw := FlagString(flags, "limit"); raw != "" {
			n, convErr := strconv.Atoi(raw)
			if convErr != nil || n < 0 {
				return nil, fmt.Errorf("--limit must be a non-negative number")
			}
			limit = n
			if limit == 0 {
				limit = 1 << 30
			}
		}
		layer := FlagString(flags, "layer")
		cmdFilter := FlagString(flags, "cmd")
		if layer == "" {
			ws, err := Open(home)
			if err != nil {
				entries, trailErr := readTrail(home, "", cmdFilter, limit)
				if trailErr != nil {
					return nil, err
				}
				return map[string]any{"source": "local", "entries": entries}, nil
			}
			defer ws.Close()
			cat, _, err := ws.UseCatalog(FlagString(flags, "catalog"))
			if err != nil {
				return nil, err
			}
			hist := cat.Log(catalog.CatalogLogQuery{
				Limit:    limit,
				View:     FlagString(flags, "view"),
				ObjectID: FlagString(flags, "object"),
			})
			entries := catalogLogEntries(hist.Commits, cmdFilter, limit)
			return map[string]any{
				"source":    "catalog",
				"catalogId": hist.RepositoryID,
				"entries":   entries,
			}, nil
		}
		if layer != "kc" && layer != "system" {
			return nil, fmt.Errorf("--layer must be kc or system")
		}
		entries, err := readTrail(home, layer, cmdFilter, limit)
		if err != nil {
			return nil, err
		}
		return map[string]any{"source": "local", "layer": layer, "entries": entries}, nil
	}

	if command == "status" {
		ws, err := Open(home)
		if err != nil {
			return nil, err
		}
		defer ws.Close()
		cat, reg, err := ws.UseCatalog(FlagString(flags, "catalog"))
		if err != nil {
			return nil, err
		}
		state := cat.DumpState()
		repos := make([]map[string]any, 0, len(ws.File.Repos))
		for _, r := range ws.File.Repos {
			item := map[string]any{"id": r.ID, "dir": r.Dir}
			if r.Driver != "" {
				item["driver"] = r.Driver
			}
			if r.DSN != "" {
				item["dsn"] = r.DSN
			}
			if repo, ok := ws.Store.Get(kernel.RepositoryID(r.ID)); ok {
				if head, err := repo.Head("refs/heads/main"); err == nil {
					item["head"] = head
				}
				item["archived"] = repo.Archived()
			}
			repos = append(repos, item)
		}
		catalogs := make([]map[string]any, 0, len(ws.File.Catalogs))
		for _, item := range ws.File.Catalogs {
			row := map[string]any{"id": item.ID, "dir": item.Dir}
			if r := ws.Registries[item.ID]; r != nil {
				if head, err := r.Repo().Head("refs/heads/main"); err == nil {
					row["head"] = head
				}
			}
			catalogs = append(catalogs, row)
		}
		catalogHead, err := reg.Repo().Head("refs/heads/main")
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"repos":    repos,
			"stores":   PublicStores(ws.Stores),
			"catalogs": catalogs,
			"catalog": map[string]any{
				"repositoryId": reg.Repo().ID(),
				"head":         catalogHead,
			},
			"views":        state.Views,
			"repositories": state.Repositories,
			"archived":     state.Archived,
		}, nil
	}

	if command == "allow" || command == "revoke" || command == "allowed" || command == "whoami" {
		return handleAllow(home, command, flags)
	}
	if command == "hook-add" || command == "hook-ls" || command == "hook-rm" {
		return handleHook(home, command, flags)
	}
	if command == "gate-add" || command == "gate-ls" || command == "gate-rm" {
		return handleGate(home, command, flags)
	}

	ws, err := Open(home)
	if err != nil {
		return nil, err
	}
	defer ws.Close()
	if err := authorize(home, consumerAllowCmd(command, flags), flags); err != nil {
		return nil, err
	}
	ws.observe(command, flags)
	ws.bindControl(FlagString(flags, "catalog"))

	return withHooks(ws, home, command, flags, func() (any, error) {
		return handleCommand(ws, home, command, flags)
	})
}

func handleCommand(ws *OpenWorkspace, home, command string, flags map[string]FlagValue) (any, error) {
	switch command {
	case "put":
		value, ok, err := loadJSONFlag(flags, "--value")
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("put requires --file or --value")
		}
		op, err := writeOperation(flags, repository.OpPut, value)
		if err != nil {
			return nil, err
		}
		return commitOne(ws, flags, []repository.Operation{op})
	case "remove":
		op, err := writeOperation(flags, repository.OpRemove, nil)
		if err != nil {
			return nil, err
		}
		return commitOne(ws, flags, []repository.Operation{op})
	case "ingest":
		dir, err := RequireFlag(flags, "dir")
		if err != nil {
			return nil, err
		}
		repoID, err := RequireFlag(flags, "repo")
		if err != nil {
			return nil, err
		}
		repo, ok := ws.Store.Get(kernel.RepositoryID(repoID))
		if !ok {
			return nil, fmt.Errorf("unknown repository %s", repoID)
		}
		targetRef := FlagString(flags, "ref")
		if targetRef == "" {
			targetRef = "refs/heads/main"
		}
		head, err := repo.Head(targetRef)
		if err != nil {
			return nil, err
		}
		preview, err := writer.Ingest(dir, kernel.RepositoryID(repoID), head)
		if err != nil {
			return nil, err
		}
		preview.ChangeSet.TargetRef = targetRef
		if out := FlagString(flags, "out"); out != "" {
			b, err := json.MarshalIndent(preview.ChangeSet, "", "  ")
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
				return nil, err
			}
		}
		return preview, nil
	case "receipt":
		commandID, err := RequireFlag(flags, "command-id")
		if err != nil {
			return nil, err
		}
		entry, ok := ws.Writer.Lookup(commandID)
		if !ok {
			return nil, fmt.Errorf("unknown command-id %s", commandID)
		}
		return entry, nil
	case "commit":
		file, err := RequireFlag(flags, "changeset")
		if err != nil {
			return nil, err
		}
		body, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		raw, err := decodeChangeSet(body, file)
		if err != nil {
			return nil, err
		}
		if _, ok := ws.Store.Get(raw.TargetRepository); !ok {
			return nil, fmt.Errorf("unknown repository %s", raw.TargetRepository)
		}
		targetRef := raw.TargetRef
		if targetRef == "" {
			targetRef = "refs/heads/main"
		}
		commandID, err := RequireFlag(flags, "command-id")
		if err != nil {
			return nil, err
		}
		return ws.Writer.CommitIntent(commandID, writer.CommitIntent{
			TargetRepository:     raw.TargetRepository,
			TargetRef:            targetRef,
			BaseCommit:           raw.BaseCommit,
			ExpectedTargetCommit: raw.ExpectedTargetCommit,
			Operations:           raw.Operations,
			Message:              raw.Message,
			Provenance:           raw.Provenance,
		})
	case "append":
		payload, ok, err := loadJSONFlag(flags, "--payload")
		if err != nil {
			return nil, err
		}
		if !ok {
			payload = map[string]any{}
		}
		eventID, err := RequireFlag(flags, "event-id")
		if err != nil {
			return nil, err
		}
		commandID, err := RequireFlag(flags, "command-id")
		if err != nil {
			return nil, err
		}
		repoID, err := RequireFlag(flags, "repo")
		if err != nil {
			return nil, err
		}
		stream, err := RequireFlag(flags, "stream")
		if err != nil {
			return nil, err
		}
		return ws.Writer.AppendIntent(commandID, writer.AppendIntent{
			TargetRepository: kernel.RepositoryID(repoID),
			StreamRef:        stream,
			ExpectedCursor:   FlagString(flags, "cursor"),
			Entries: []repository.AppendEntry{{
				EventID:   eventID,
				EventType: FlagString(flags, "event-type"),
				Payload:   payload,
				SchemaRef: FlagString(flags, "schema-ref"),
			}},
		})
	case "resolve":
		if servingView(flags) {
			access, _, err := openServing(ws, flags)
			if err != nil {
				return nil, err
			}
			objectID, err := RequireFlag(flags, "object")
			if err != nil {
				return nil, err
			}
			return access.Resolve(kernel.ObjectID(objectID))
		}
		repositoryID, commitID, err := pinCommit(ws, flags)
		if err != nil {
			return nil, err
		}
		objectID, err := RequireFlag(flags, "object")
		if err != nil {
			return nil, err
		}
		return ws.Reader.Resolve(kernel.KnowledgeRef{Repository: repositoryID, Object: kernel.ObjectID(objectID)}, commitID)
	case "read":
		if readingCatalog("read", flags) {
			return readCatalogState(ws, flags)
		}
		if servingView(flags) {
			access, cat, err := openServing(ws, flags)
			if err != nil {
				return nil, err
			}
			if FlagString(flags, "aspect") != "" {
				address, err := addressFrom(flags)
				if err != nil {
					return nil, err
				}
				values, err := access.ReadAddress(address)
				if err != nil {
					return nil, err
				}
				return filterViewReads(home, flags, cat, values), nil
			}
			objectID, err := RequireFlag(flags, "object")
			if err != nil {
				return nil, err
			}
			values, err := access.Read(kernel.ObjectID(objectID), aspectSelectorFrom(flags))
			if err != nil {
				return nil, err
			}
			return filterViewReads(home, flags, cat, values), nil
		}
		repositoryID, commitID, err := pinCommit(ws, flags)
		if err != nil {
			return nil, err
		}
		if FlagString(flags, "aspect") != "" {
			address, err := addressFrom(flags)
			if err != nil {
				return nil, err
			}
			return ws.Reader.ReadAddress(repositoryID, address, commitID)
		}
		objectID, err := RequireFlag(flags, "object")
		if err != nil {
			return nil, err
		}
		return ws.Reader.Read(kernel.KnowledgeRef{Repository: repositoryID, Object: kernel.ObjectID(objectID)}, commitID, aspectSelectorFrom(flags))
	case "provenance":
		if servingView(flags) {
			access, _, err := openServing(ws, flags)
			if err != nil {
				return nil, err
			}
			objectID, err := RequireFlag(flags, "object")
			if err != nil {
				return nil, err
			}
			traces, err := access.GetProvenance(kernel.ObjectID(objectID))
			if err != nil {
				return nil, err
			}
			out := []repository.ProvenanceTrace{}
			for _, trace := range traces {
				if allowedRepoRead(home, flags, string(trace.Repository), string(trace.ObjectID)) {
					out = append(out, trace)
				}
			}
			return out, nil
		}
		repositoryID, commitID, err := pinCommit(ws, flags)
		if err != nil {
			return nil, err
		}
		objectID, err := RequireFlag(flags, "object")
		if err != nil {
			return nil, err
		}
		return ws.Reader.GetProvenance(kernel.KnowledgeRef{Repository: repositoryID, Object: kernel.ObjectID(objectID)}, commitID)
	case "stream":
		repoID, err := RequireFlag(flags, "repo")
		if err != nil {
			return nil, err
		}
		stream, err := RequireFlag(flags, "stream")
		if err != nil {
			return nil, err
		}
		req := reader.StreamReadRequest{
			StreamRef:      stream,
			FromCursor:     FlagString(flags, "from-cursor"),
			EventID:        FlagString(flags, "event-id"),
			FromRecordedAt: FlagString(flags, "since"),
			ToRecordedAt:   FlagString(flags, "until"),
			Cut:            FlagString(flags, "cut"),
		}
		if raw := FlagString(flags, "limit"); raw != "" {
			n, convErr := strconv.Atoi(raw)
			if convErr != nil || n < 0 {
				return nil, fmt.Errorf("--limit must be a non-negative number")
			}
			req.Limit = n
		}
		return ws.Reader.QueryStream(kernel.RepositoryID(repoID), req)
	case "list":
		if servingView(flags) {
			access, cat, err := openServing(ws, flags)
			if err != nil {
				return nil, err
			}
			values, err := access.List()
			if err != nil {
				return nil, err
			}
			return filterViewReads(home, flags, cat, values), nil
		}
		repositoryID, commitID, err := pinCommit(ws, flags)
		if err != nil {
			return nil, err
		}
		return ws.Reader.List(repositoryID, commitID)
	case "describe-schema":
		if servingView(flags) {
			access, _, err := openServing(ws, flags)
			if err != nil {
				return nil, err
			}
			reports, err := access.DescribeSchema(kernel.ObjectID(FlagString(flags, "object")))
			if err != nil {
				return nil, err
			}
			out := []reader.SchemaReport{}
			for _, report := range reports {
				if allowedRepoRead(home, flags, string(report.Repository), string(FlagString(flags, "object"))) {
					out = append(out, report)
				}
			}
			return out, nil
		}
		repositoryID, commitID, err := pinCommit(ws, flags)
		if err != nil {
			return nil, err
		}
		return ws.Reader.DescribeSchema(repositoryID, commitID, kernel.ObjectID(FlagString(flags, "object")))
	case "search":
		if servingView(flags) {
			return searchView(ws, home, flags)
		}
		repositoryID, commitID, err := pinCommit(ws, flags)
		if err != nil {
			return nil, err
		}
		repo, err := ws.Store.Require(kernel.RepositoryID(repositoryID), kernel.ErrKnowledgeRefUnresolved)
		if err != nil {
			return nil, err
		}
		if _, err := ws.Index.Ensure(repo, commitID); err != nil {
			return nil, err
		}
		req, err := searchRequestFromFlags(flags)
		if err != nil {
			return nil, err
		}
		return ws.Index.Search(repo, req)
	case "describe-index":
		repoID, err := RequireFlag(flags, "repo")
		if err != nil {
			return nil, err
		}
		repo, err := ws.Store.Require(kernel.RepositoryID(repoID), kernel.ErrKnowledgeRefUnresolved)
		if err != nil {
			return nil, err
		}
		return ws.Index.Describe(repo)
	case "index-sync":
		repositoryID, commitID, err := pinCommit(ws, flags)
		if err != nil {
			return nil, err
		}
		repo, err := ws.Store.Require(repositoryID, kernel.ErrKnowledgeRefUnresolved)
		if err != nil {
			return nil, err
		}
		return ws.Index.Ensure(repo, commitID)
	case "index-plan":
		cat, err := pickCatalog(ws, flags)
		if err != nil {
			return nil, err
		}
		viewID, err := RequireFlag(flags, "view")
		if err != nil {
			return nil, err
		}
		return cat.PlanIndex(viewID)
	case "log":
		if FlagString(flags, "object") == "" && FlagString(flags, "repo") == "" {
			if _, ok := flags["catalog"]; ok {
				return nil, fmt.Errorf("catalog registry history is kc audit")
			}
		}
		if servingView(flags) {
			serving, _, err := openServing(ws, flags)
			if err != nil {
				return nil, err
			}
			objectID, err := RequireFlag(flags, "object")
			if err != nil {
				return nil, err
			}
			limit := 0
			if raw := FlagString(flags, "limit"); raw != "" {
				limit, _ = strconv.Atoi(raw)
			}
			logs, err := serving.Log(kernel.ObjectID(objectID), limit)
			if err != nil {
				return nil, err
			}
			return filterViewLogs(home, flags, logs), nil
		}
		repositoryID, commitID, err := pinCommit(ws, flags)
		if err != nil {
			return nil, err
		}
		objectID, err := RequireFlag(flags, "object")
		if err != nil {
			return nil, err
		}
		limit := 0
		if raw := FlagString(flags, "limit"); raw != "" {
			limit, _ = strconv.Atoi(raw)
		}
		return ws.Reader.Log(repositoryID, kernel.ObjectID(objectID), commitID, limit)
	case "diff":
		repositoryID, err := RequireFlag(flags, "repo")
		if err != nil {
			return nil, err
		}
		objectID, err := RequireFlag(flags, "object")
		if err != nil {
			return nil, err
		}
		from, err := RequireFlag(flags, "from")
		if err != nil {
			return nil, err
		}
		to, err := RequireFlag(flags, "to")
		if err != nil {
			return nil, err
		}
		return ws.Reader.Diff(kernel.RepositoryID(repositoryID), kernel.ObjectID(objectID), kernel.CommitID(from), kernel.CommitID(to))
	case "define-view":
		cat, err := pickCatalog(ws, flags)
		if err != nil {
			return nil, err
		}
		var sources []catalog.ViewSource
		for _, item := range FlagStrings(flags, "source") {
			eq := strings.IndexByte(item, '=')
			if eq < 0 {
				return nil, fmt.Errorf("--source must be repo=selector, got %s", item)
			}
			sources = append(sources, catalog.ViewSource{
				Repository: kernel.RepositoryID(item[:eq]),
				Selector:   item[eq+1:],
			})
		}
		if len(sources) == 0 {
			return nil, fmt.Errorf("define-view requires at least one --source repo=selector")
		}
		revisionRaw, err := RequireFlag(flags, "revision")
		if err != nil {
			return nil, err
		}
		revision, err := strconv.Atoi(revisionRaw)
		if err != nil {
			return nil, fmt.Errorf("--revision must be a number")
		}
		viewID, err := RequireFlag(flags, "view")
		if err != nil {
			return nil, err
		}
		return cat.DefineView(viewID, revision, sources)
	case "propose":
		repositoryID, err := RequireFlag(flags, "repo")
		if err != nil {
			return nil, err
		}
		repo, ok := ws.Store.Get(kernel.RepositoryID(repositoryID))
		if !ok {
			return nil, fmt.Errorf("unknown repository %s", repositoryID)
		}
		targetRef := FlagString(flags, "target")
		if targetRef == "" {
			targetRef = "refs/heads/main"
		}
		operations, err := proposeOperations(flags)
		if err != nil {
			return nil, err
		}
		proposalID, err := RequireFlag(flags, "proposal-id")
		if err != nil {
			return nil, err
		}
		candidate, err := RequireFlag(flags, "candidate")
		if err != nil {
			return nil, err
		}
		base := FlagString(flags, "base")
		if base == "" {
			head, err := repo.Head(targetRef)
			if err != nil {
				return nil, err
			}
			base = string(head)
		}
		proposal, err := ws.ControlPlane.Propose(controlplane.ProposeInput{
			ProposalID:   proposalID,
			RepositoryID: kernel.RepositoryID(repositoryID),
			TargetRef:    targetRef,
			CandidateRef: candidate,
			BaseCommit:   kernel.CommitID(base),
			Operations:   operations,
			Rationale:    FlagString(flags, "message"),
			Provenance:   originFrom(flags),
		})
		if err != nil {
			return nil, err
		}
		ws.Control.Proposals[proposal.ProposalID] = proposal
		if err := PersistControl(ws); err != nil {
			return nil, err
		}
		return proposal, nil
	case "preview":
		plane, err := planeFor(ws, flags)
		if err != nil {
			return nil, err
		}
		proposalID, err := RequireFlag(flags, "proposal")
		if err != nil {
			return nil, err
		}
		proposal, ok := ws.Control.Proposals[proposalID]
		if !ok {
			return nil, fmt.Errorf("unknown proposal; run propose first")
		}
		viewID, err := RequireFlag(flags, "view")
		if err != nil {
			return nil, err
		}
		preview, err := plane.CreatePreview(viewID, proposal)
		if err != nil {
			return nil, err
		}
		ws.Control.Previews[preview.PreviewID] = preview
		if err := PersistControl(ws); err != nil {
			return nil, err
		}
		return preview, nil
	case "validate":
		plane, err := planeFor(ws, flags)
		if err != nil {
			return nil, err
		}
		previewID, err := RequireFlag(flags, "preview")
		if err != nil {
			return nil, err
		}
		preview, ok := ws.Control.Previews[previewID]
		if !ok {
			return nil, fmt.Errorf("unknown preview; run preview first")
		}
		report, err := plane.ValidateStructure(preview)
		if err != nil {
			return nil, err
		}
		ws.Control.Validations[report.ReportID] = report.ValidationReport
		if err := PersistControl(ws); err != nil {
			return nil, err
		}
		return report, nil
	case "record-validation":
		plane, err := planeFor(ws, flags)
		if err != nil {
			return nil, err
		}
		previewID, err := RequireFlag(flags, "preview")
		if err != nil {
			return nil, err
		}
		outcome, err := RequireFlag(flags, "outcome")
		if err != nil {
			return nil, err
		}
		if outcome != "PASSED" && outcome != "FAILED" {
			return nil, fmt.Errorf("--outcome must be PASSED or FAILED")
		}
		suite, err := RequireFlag(flags, "suite")
		if err != nil {
			return nil, err
		}
		preview, ok := ws.Control.Previews[previewID]
		if !ok {
			return nil, fmt.Errorf("unknown preview; run preview first")
		}
		report, err := plane.RecordValidation(preview, suite, outcome)
		if err != nil {
			return nil, err
		}
		ws.Control.Validations[report.ReportID] = report
		if err := PersistControl(ws); err != nil {
			return nil, err
		}
		return report, nil
	case "merge":
		proposalID, err := RequireFlag(flags, "proposal")
		if err != nil {
			return nil, err
		}
		previewID, err := RequireFlag(flags, "preview")
		if err != nil {
			return nil, err
		}
		proposal, ok1 := ws.Control.Proposals[proposalID]
		preview, ok2 := ws.Control.Previews[previewID]
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("merge needs stored --proposal and --preview ids")
		}
		required := ws.mergeRequired(proposal.TargetRepository)
		var validation controlplane.ValidationReport
		if id := FlagString(flags, "validation"); id != "" {
			var ok bool
			validation, ok = ws.Control.Validations[id]
			if !ok {
				return nil, fmt.Errorf("unknown validation %s", id)
			}
		} else if len(required) == 0 {
			return nil, fmt.Errorf("merge needs stored --proposal, --preview and --validation ids")
		}
		plane, err := planeFor(ws, flags)
		if err != nil {
			return nil, err
		}
		commitID, err := plane.Merge(proposal, preview, validation)
		if err != nil {
			return nil, err
		}
		return map[string]any{"commitId": commitID}, nil
	case "register":
		cat, err := pickCatalog(ws, flags)
		if err != nil {
			return nil, err
		}
		repositoryID, err := RequireFlag(flags, "repo")
		if err != nil {
			return nil, err
		}
		if _, ok := ws.Store.Get(kernel.RepositoryID(repositoryID)); !ok {
			return nil, fmt.Errorf("unknown repository %s; run repo-add first", repositoryID)
		}
		if err := cat.RegisterRepository(kernel.RepositoryID(repositoryID)); err != nil {
			return nil, err
		}
		return map[string]any{"catalog": catalogIDOf(ws, flags), "repositoryId": repositoryID}, nil
	case "retire-view":
		cat, err := pickCatalog(ws, flags)
		if err != nil {
			return nil, err
		}
		viewID, err := RequireFlag(flags, "view")
		if err != nil {
			return nil, err
		}
		if err := cat.RetireDefinition(viewID); err != nil {
			return nil, err
		}
		return map[string]any{"view": viewID, "retired": true}, nil
	case "archive-catalog":
		cat, err := pickCatalog(ws, flags)
		if err != nil {
			return nil, err
		}
		if err := cat.Archive(); err != nil {
			return nil, err
		}
		return map[string]any{"catalog": catalogIDOf(ws, flags), "archived": true}, nil
	case "archive-repo":
		repositoryID, err := RequireFlag(flags, "repo")
		if err != nil {
			return nil, err
		}
		repo, ok := ws.Store.Get(kernel.RepositoryID(repositoryID))
		if !ok {
			return nil, fmt.Errorf("unknown repository %s", repositoryID)
		}
		if err := repo.Archive(); err != nil {
			return nil, err
		}
		if err := journal.Finish(ws.Journal, journal.LayerSystem, "repository", "archive-repo", map[string]any{"repositoryId": repositoryID}, nil); err != nil {
			return nil, err
		}
		return map[string]any{"repositoryId": repositoryID, "archived": true}, nil
	default:
		return nil, fmt.Errorf("unknown command %s\n\n%s", command, Help)
	}
}

func errorResult(err error) RunResult {
	if e := kernel.AsIngress(err); e != nil {
		return RunResult{Status: 1, Stdout: jsonOut(map[string]any{"error": map[string]any{"code": e.Code, "message": e.Message}})}
	}
	return RunResult{Status: 1, Stdout: jsonOut(map[string]any{"error": map[string]any{"message": err.Error()}})}
}

func Run(argv []string) RunResult {
	parsed, err := ParseArgs(argv)
	if err != nil {
		return errorResult(err)
	}
	if parsed.Command == "serve" {
		return runServe(parsed.Flags)
	}
	return Invoke(parsed.Command, parsed.Flags)
}

// Invoke is the CLI/HTTP shared entry: one verb + flag map (same names as --flags).
func Invoke(command string, flags map[string]FlagValue) RunResult {
	if flags == nil {
		flags = map[string]FlagValue{}
	}
	result, err := handle(command, flags)
	if home, homeErr := resolveHome(flags); homeErr == nil {
		if aerr := recordAudit(home, command, flags, result, err); aerr != nil && err == nil {
			err = aerr
			result = nil
		}
	}
	if err != nil {
		return errorResult(err)
	}
	if s, ok := result.(string); ok {
		if strings.HasSuffix(s, "\n") {
			return RunResult{Status: 0, Stdout: s}
		}
		return RunResult{Status: 0, Stdout: s + "\n"}
	}
	return RunResult{Status: 0, Stdout: jsonOut(result)}
}

func pickCatalog(ws *OpenWorkspace, flags map[string]FlagValue) (*catalog.Catalog, error) {
	cat, _, err := ws.UseCatalog(FlagString(flags, "catalog"))
	return cat, err
}

func planeFor(ws *OpenWorkspace, flags map[string]FlagValue) (*controlplane.ControlPlane, error) {
	cat, err := pickCatalog(ws, flags)
	if err != nil {
		return nil, err
	}
	if FlagString(flags, "catalog") == "" {
		return ws.ControlPlane, nil
	}
	plane := controlplane.New(ws.Store, ws.Writer, cat)
	plane.SetJournal(ws.Journal)
	ws.attachMergeGate(plane)
	return plane, nil
}

func catalogIDOf(ws *OpenWorkspace, flags map[string]FlagValue) string {
	if id := FlagString(flags, "catalog"); id != "" {
		return id
	}
	if len(ws.File.Catalogs) > 0 {
		return ws.File.Catalogs[0].ID
	}
	return ""
}

func readCatalogState(ws *OpenWorkspace, flags map[string]FlagValue) (any, error) {
	cat, err := pickCatalog(ws, flags)
	if err != nil {
		return nil, err
	}
	return catalog.NormalizeCatalogState(cat.DumpState()), nil
}

func handleAllow(home, command string, flags map[string]FlagValue) (any, error) {
	switch command {
	case "whoami":
		principal := FlagString(flags, "as")
		if principal == "" {
			principal = "owner"
		}
		return map[string]any{"principal": principal}, nil
	case "allow":
		principal, err := RequireFlag(flags, "principal")
		if err != nil {
			return nil, err
		}
		cmds := splitCmds(FlagString(flags, "cmd"))
		if err := validateCmds(cmds); err != nil {
			return nil, err
		}
		repo := FlagString(flags, "repo")
		catalogID := FlagString(flags, "catalog")
		if repo == "" && catalogID == "" {
			return nil, fmt.Errorf("allow requires --repo or --catalog")
		}
		file, err := ReadAllow(home)
		if err != nil {
			return nil, err
		}
		rule := AllowRule{
			ID:        nextRuleID(file.Rules),
			Principal: principal,
			Cmds:      cmds,
			Repo:      repo,
			Catalog:   catalogID,
			Ref:       FlagString(flags, "ref"),
			Object:    FlagString(flags, "object"),
			Aspect:    FlagString(flags, "aspect"),
			Stream:    FlagString(flags, "stream"),
			View:      FlagString(flags, "view"),
		}
		file.Rules = append(file.Rules, rule)
		if err := WriteAllow(home, file); err != nil {
			return nil, err
		}
		return rule, nil
	case "revoke":
		id, err := RequireFlag(flags, "id")
		if err != nil {
			return nil, err
		}
		file, err := ReadAllow(home)
		if err != nil {
			return nil, err
		}
		kept := file.Rules[:0]
		found := false
		for _, rule := range file.Rules {
			if rule.ID == id {
				found = true
				continue
			}
			kept = append(kept, rule)
		}
		if !found {
			return nil, fmt.Errorf("unknown rule %s", id)
		}
		file.Rules = kept
		if err := WriteAllow(home, file); err != nil {
			return nil, err
		}
		return map[string]any{"revoked": id}, nil
	case "allowed":
		file, err := ReadAllow(home)
		if err != nil {
			return nil, err
		}
		principal := FlagString(flags, "principal")
		if principal == "" {
			principal = FlagString(flags, "as")
		}
		cmd := FlagString(flags, "cmd")
		if principal == "" || cmd == "" {
			return file, nil
		}
		q := AllowQuery{
			Principal: principal,
			Cmd:       cmd,
			Repo:      FlagString(flags, "repo"),
			Catalog:   FlagString(flags, "catalog"),
			Ref:       FlagString(flags, "ref"),
			Object:    FlagString(flags, "object"),
			Aspect:    FlagString(flags, "aspect"),
			Stream:    FlagString(flags, "stream"),
			View:      FlagString(flags, "view"),
		}
		rule, ok := MatchAllow(file.Rules, q)
		if !ok {
			return nil, kernel.Fail(kernel.ErrForbidden, "%s is not allowed to %s", principal, cmd)
		}
		return map[string]any{"allow": true, "ruleId": rule.ID}, nil
	default:
		return nil, fmt.Errorf("unknown command %s", command)
	}
}
