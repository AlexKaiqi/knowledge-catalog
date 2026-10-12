package invocation

// Session persistence and managed-repository validation helpers shared by the
// registry operations and the login flows. (CAP-ORG-01 B6.)

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	kchome "kc/home"
	"kc/identity"
	knowledge "kc/knowledge"
	"kc/snapshot"
	"os"
	"path/filepath"
	"strings"

	"compress/gzip"
	"context"
	"io"
	"kc/delivery"
	"kc/invoke"
	"kc/kernel"
	reader "kc/knowledge/reader"
	knowledgeserving "kc/knowledge/serving"
	knowledgeapp "kc/knowledgeapp"
	"kc/retrieval"
	"kc/snapshot/dataset"
	"net/http"
)

type ClientEndpoint struct {
	Server string `json:"server"`
}

func SavedClientServer() string {
	for _, path := range []string{filepath.Join(ConfigDir(), "client.json"), filepath.Join(ConfigDir(), "session-taihu.json"), filepath.Join(ConfigDir(), "session-local.json")} {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var config ClientEndpoint
		if json.Unmarshal(raw, &config) == nil && NormalizeLoginServer(config.Server) != "" {
			return NormalizeLoginServer(config.Server)
		}
	}
	return ""
}

func ConfigDir() string {
	if dir := strings.TrimSpace(os.Getenv("KC_CONFIG_DIR")); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".kc")
	}
	return filepath.Join(home, ".config", "kc")
}

func NormalizeLoginServer(server string) string {
	return strings.TrimRight(strings.TrimSpace(server), "/")
}

func RequireRemoteFlag(flags map[string]invoke.FlagValue, name string) (string, error) {
	value := strings.TrimSpace(invoke.FlagString(flags, name))
	if value == "" {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "remote command requires --%s", name)
	}
	return value, nil
}

func WriteJSONFile(path string, data any) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".kc-session-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func ValidateManagedRepositoryCoordinates(flags map[string]invoke.FlagValue) error {
	if invoke.FlagString(flags, "name") != "" {
		if invoke.FlagString(flags, "repo") != "" || invoke.FlagString(flags, "command-id") != "" {
			return fmt.Errorf("--name cannot be combined with --repo or --command-id; LakeFS uses --name as the repository id")
		}
		return nil
	}
	for _, name := range []string{"repo", "command-id"} {
		if _, err := RequireRemoteFlag(flags, name); err != nil {
			return err
		}
	}
	return nil
}

// requireRepo resolves an attached Knowledge Repository. Every verb that names --repo goes
// through here so the error code and wording are the same everywhere.
// Verbs needing layer ② ask ws.Reader to interpret the mounted TreeStore.
func RequireRepo(ws *kchome.Home, repositoryID string) (snapshot.Store, error) {
	repo, ok := ws.Store.Get(kernel.RepositoryID(repositoryID))
	if !ok {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown repository %s", repositoryID)
	}
	return repo, nil
}

// ensureManagedRepositoryGrant applies a deployment-approved creation policy
// exactly once. The HTTP application write lock serializes this with normal
// grant/revoke operations. The receipt and rule share one durable policy write;
// a crash after this write cannot make a retry undo an intervening revocation.
func EnsureManagedRepositoryGrant(dir string, g kchome.ManagedRepositoryGrant) error {
	if g.AllocationID == "" || g.Principal == "" || g.RepositoryID == "" || len(g.Actions) == 0 {
		return kernel.Fail(kernel.ErrPreconditionFailed, "managed repository initial policy is incomplete")
	}
	if info, err := os.Stat(AllowPath(dir)); err != nil || !info.Mode().IsRegular() {
		return kernel.Fail(kernel.ErrPreconditionFailed, "durable authorization policy is unavailable")
	}
	policy, err := ReadAllow(dir)
	if err != nil {
		return err
	}
	digest := kernel.CanonicalDigest(g)
	if previous, ok := policy.InitialGrants[g.AllocationID]; ok {
		if previous != digest {
			return kernel.Fail(kernel.ErrIdempotencyConflict, "managed repository initial policy does not match its allocation")
		}
		return nil
	}
	principal, err := identity.ResolvePrincipal(filepath.Join(dir, identity.Filename), g.Principal)
	if err != nil {
		return err
	}
	if policy.InitialGrants == nil {
		policy.InitialGrants = map[string]kernel.Digest{}
	}
	policy.InitialGrants[g.AllocationID] = digest
	policy.Rules = append(policy.Rules, AllowRule{
		ID:        "managed_" + g.AllocationID,
		Principal: principal,
		Repo:      g.RepositoryID,
		Actions:   append([]string(nil), g.Actions...),
	})
	return WriteAllow(dir, policy)
}

func ServerSessionPath(server, filename string) string {
	digest := sha256.Sum256([]byte(NormalizeLoginServer(server)))
	return filepath.Join(ConfigDir(), "sessions", hex.EncodeToString(digest[:]), filename)
}

// DropByID removes one identified rule from a list, reporting whether it was
// there. Shared by allow / hook / gate, which all revoke by id.
func DropByID[T any](items []T, id string, idOf func(T) string) ([]T, bool) {
	kept := make([]T, 0, len(items))
	found := false
	for _, item := range items {
		if idOf(item) == id {
			found = true
			continue
		}
		kept = append(kept, item)
	}
	return kept, found
}

// originFrom builds the provenance envelope, or nil when no origin flag was
// given. DERIVATION without a pinned input is rejected downstream by kernel.
func OriginFrom(flags map[string]invoke.FlagValue) *knowledge.ProvenanceEnvelope {
	originKind := invoke.FlagString(flags, "origin-kind")
	sourceRefs := invoke.FlagStrings(flags, "source-ref")
	evidenceRefs := invoke.FlagStrings(flags, "evidence-ref")
	actor := invoke.FlagString(flags, "actor-ref")
	activity := invoke.FlagString(flags, "activity-ref")
	inputWorkspaceVersion := invoke.FlagString(flags, "input-dataset-version")
	spec := invoke.FlagString(flags, "algorithm-spec")
	model := invoke.FlagString(flags, "algorithm-model")
	hash := invoke.FlagString(flags, "algorithm-hash")
	produced := invoke.FlagString(flags, "produced-at")
	if originKind == "" && len(sourceRefs) == 0 && len(evidenceRefs) == 0 && actor == "" && activity == "" && inputWorkspaceVersion == "" && spec == "" && model == "" && hash == "" && produced == "" {
		return nil
	}
	kind := knowledge.OriginKind(originKind)
	if kind == "" {
		kind = knowledge.OriginSource
	}
	env := &knowledge.ProvenanceEnvelope{
		OriginKind:               kind,
		ActorRef:                 actor,
		ActivityRef:              activity,
		SourceRefs:               sourceRefs,
		EvidenceRefs:             evidenceRefs,
		InputWorkspaceVersionRef: inputWorkspaceVersion,
		ProducedAt:               produced,
	}
	if spec != "" || model != "" || hash != "" {
		env.Algorithm = &knowledge.AlgorithmRef{
			DerivationSpecRef: spec,
			ModelRef:          model,
			CodeHash:          hash,
		}
	}
	return env
}

func ServingWorkspace(flags map[string]invoke.FlagValue) bool {
	return SuppliedKnowledgeSet(flags) != nil ||
		invoke.FlagString(flags, "pin") != "" ||
		invoke.FlagString(flags, "dataset-file") != "" ||
		(invoke.DatasetOf(flags) != "" && !ReadingInventoryCommand(flags))
}

func DeliverSearchResult(home string, flags map[string]invoke.FlagValue, out retrieval.SearchResult) (retrieval.SearchResult, error) {
	for i := range out.Hits {
		hit, err := DeliverSearchHit(home, flags, out.Hits[i])
		if err != nil {
			return retrieval.SearchResult{}, err
		}
		out.Hits[i] = hit
	}
	return out, nil
}

func SearchWorkspace(cx *Invocation) (any, error) {
	req, err := SearchRequestFromFlags(cx.Flags)
	if err != nil {
		return nil, err
	}
	// The HTTP server passes its request-time embedding provider through the
	// internal _embedder stamp (like _search-request). The plain CLI leaves
	// it unset and dataset semantic recall fails closed there.
	embedder, _ := cx.Flags["_embedder"].(retrieval.Embedder)
	return (knowledgeapp.DatasetSearchExecutor{
		Embedder:  embedder,
		Authorize: func(context.Context) error { return Authorize(cx.Home, "knowledge.search", cx.Flags, nil, cx.WS) },
		Resolve: func(context.Context) (*reader.Serving, *knowledgeserving.Service, error) {
			serving, _, err := OpenServingAt(cx, cx.WS, cx.Flags)
			if err != nil {
				return nil, nil, err
			}
			logical, err := LogicalWorkspaceServing(cx, serving)
			return serving, logical, err
		},
		Repositories: cx.WS.Reader, Projection: cx.WS.Index,
		Deliver: func(_ context.Context, hit retrieval.KnowledgeHit) (retrieval.KnowledgeHit, error) {
			return DeliverSearchHit(cx.Home, cx.Flags, hit)
		},
	}).Execute(cx.Context, req)
}

// PinCommit freezes {Repository, commit} for the duration of one command: an explicit
// --commit, else the tip of --ref at this instant. Nothing re-reads the ref later.
func PinCommit(ws *kchome.Home, flags map[string]invoke.FlagValue) (kernel.RepositoryID, kernel.CommitID, error) {
	repositoryID, err := invoke.RequireFlag(flags, "repo")
	if err != nil {
		return "", "", err
	}
	repo, err := RequireRepo(ws, repositoryID)
	if err != nil {
		return "", "", err
	}
	if commit := invoke.FlagString(flags, "commit"); commit != "" {
		flags[ResolvedCommitFlag] = commit
		return kernel.RepositoryID(repositoryID), kernel.CommitID(commit), nil
	}
	ref := snapshot.RefOrDefault(invoke.FlagString(flags, "ref"))
	commitID, ok := repo.GetRef(ref)
	if !ok {
		return "", "", fmt.Errorf("ref %s does not exist in %s", ref, repositoryID)
	}
	flags[ResolvedCommitFlag] = string(commitID)
	return kernel.RepositoryID(repositoryID), commitID, nil
}

func WriteOperation(flags map[string]invoke.FlagValue, op knowledge.OpKind, value any) (knowledge.Operation, error) {
	address, err := AddressFrom(flags)
	if err != nil {
		return knowledge.Operation{}, err
	}
	pre, err := preconditionFrom(flags)
	if err != nil {
		return knowledge.Operation{}, err
	}
	var source *knowledge.ValueSource
	if raw := invoke.FlagString(flags, "value-source"); raw != "" {
		if op != knowledge.OpPut {
			return knowledge.Operation{}, fmt.Errorf("--value-source is only valid with PUT")
		}
		var parsed knowledge.ValueSource
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			return knowledge.Operation{}, fmt.Errorf("--value-source is not valid JSON")
		}
		if err := knowledge.ValidateValueSource(&parsed); err != nil {
			return knowledge.Operation{}, err
		}
		source = &parsed
	}
	return knowledge.Operation{
		Op:           op,
		Address:      address,
		Value:        value,
		PathHint:     invoke.FlagString(flags, "path-hint"),
		SchemaRef:    invoke.FlagString(flags, "schema-ref"),
		ValueSource:  source,
		Precondition: pre,
	}, nil
}

func ReadingInventoryCommand(flags map[string]invoke.FlagValue) bool { return false }

func DeliverSearchHit(home string, flags map[string]invoke.FlagValue, hit retrieval.KnowledgeHit) (retrieval.KnowledgeHit, error) {
	env, err := KnowledgeDelivery(home, flags).Apply(delivery.Context{Principal: invoke.FlagString(flags, "as")}, delivery.FromValue(hit.Knowledge, hit.Version.Observations))
	if err != nil {
		return retrieval.KnowledgeHit{}, err
	}
	hit.Knowledge, hit.Version.Observations = env.WriteBody(hit.Knowledge)
	return hit, nil
}

func OpenServingAt(cx *Invocation, ws *kchome.Home, flags map[string]invoke.FlagValue) (*reader.Serving, dataset.Service, error) {
	if invoke.FlagString(flags, "repo") != "" || invoke.FlagString(flags, "commit") != "" || invoke.FlagString(flags, "ref") != "" {
		if err := RejectMixedKnowledgeBasis(flags); err != nil {
			return nil, nil, err
		}
		return nil, nil, kernel.Fail(kernel.ErrUsageInvalid, "choose a Workspace definition/pin or a Repository basis; do not mix")
	}
	cat, err := PickDatasets(ws, flags)
	if err != nil {
		return nil, nil, err
	}
	setID := invoke.DatasetOf(flags)
	if setID == "" && SuppliedKnowledgeSet(flags) == nil &&
		invoke.FlagString(flags, "pin") == "" && invoke.FlagString(flags, "dataset-file") == "" {
		return nil, nil, fmt.Errorf("missing --dataset or temporary definition")
	}
	home := ""
	if ws != nil {
		home = ws.Dir
	}
	resolved, err := ResolveOrReplay(ws, home, cat, setID, flags)
	if err != nil {
		return nil, nil, err
	}
	serving := reader.Open(ws.Reader.Lookup(cat.Require), KnowledgeSetPin(resolved))
	serving.SetHydrator(ws.Hydrator)
	return serving, cat, nil
}

func LogicalWorkspaceServing(cx *Invocation, declarations *reader.Serving) (*knowledgeserving.Service, error) {
	request, err := stateRequestContextFrom(cx)
	if err != nil {
		return nil, err
	}
	return knowledgeserving.OpenRequest(declarations, cx.State, request), nil
}

func KnowledgeDelivery(home string, flags map[string]invoke.FlagValue) delivery.Chain {
	return delivery.Chain{delivery.RepositoryRead{Allowed: func(_ string, ref knowledge.KnowledgeRef) bool {
		return AllowedRepoRead(home, flags, string(ref.Repository), string(ref.Object))
	}}}
}

func preconditionFrom(flags map[string]invoke.FlagValue) (*knowledge.Precondition, error) {
	ifAbsent := invoke.FlagBool(flags, "if-absent")
	digest := invoke.FlagString(flags, "if-digest")
	if ifAbsent && digest != "" {
		return nil, fmt.Errorf("use only one of --if-absent or --if-digest")
	}
	if ifAbsent {
		return &knowledge.Precondition{Type: knowledge.IfAbsent}, nil
	}
	if digest != "" {
		return &knowledge.Precondition{Type: knowledge.IfDigestEquals, Digest: kernel.Digest(digest)}, nil
	}
	return nil, nil
}

func stateRequestContextFrom(cx *Invocation) (knowledgeserving.RequestContext, error) {
	identity, err := IdentityContextFrom(cx.Flags)
	if err != nil {
		return knowledgeserving.RequestContext{}, err
	}
	trace, err := TraceContextFrom(cx.Flags)
	if err != nil {
		return knowledgeserving.RequestContext{}, err
	}
	requestID, err := RequestIDFrom(cx.Flags)
	if err != nil {
		return knowledgeserving.RequestContext{}, err
	}
	return knowledgeserving.RequestContext{
		Identity: identity, Trace: trace, RequestID: requestID,
	}, nil
}

func AllowedRepoRead(home string, flags map[string]invoke.FlagValue, repo, object string) bool {
	if OwnerBypass(flags) {
		return true
	}
	if ServingWorkspace(flags) && AllowedDatasetFileRead(home, flags) {
		return true
	}
	as := invoke.FlagString(flags, "as")
	if RepositoryAuthenticatedAllowed(home, as, "knowledge.read", repo) {
		return true
	}
	file, err := ReadAllow(home)
	if err != nil {
		// Authorization state is part of the security boundary. A missing or
		// unreadable policy must never broaden access merely because the
		// secondary per-repository check could not be evaluated.
		return false
	}
	_, ok := MatchAllow(file.Rules, AllowQuery{Principal: as, Action: "knowledge.read", Repo: repo, Object: object})
	return ok
}

// requireCompleteWorkspaceRead fails closed when a bare-result consumer
// operation cannot see every member at the requested object scope. The error is
// intentionally generic: callers learn that the Workspace read is incomplete,
// not which hidden repository may contain the object.
func RequireCompleteWorkspaceRead(home string, flags map[string]invoke.FlagValue, pin reader.KnowledgeSetPin, object string) error {
	if OwnerBypass(flags) {
		return nil
	}
	if AllowedDatasetFileRead(home, flags) {
		return nil
	}
	for repositoryID := range pin.Repositories {
		if !AllowedRepoRead(home, flags, string(repositoryID), object) {
			return kernel.Fail(kernel.ErrForbidden, "workspace read is incomplete because one or more members are not authorized")
		}
	}
	return nil
}

// DecodeServiceRequestBody decodes exactly one JSON object from the request
// body under the given decompressed-size cap.
func DecodeServiceRequestBody(w http.ResponseWriter, r *http.Request, target any, limit int) bool {
	stream, err := RequestJSONStream(r, limit)
	if err != nil {
		invoke.WriteJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "decode request: %v", err)))
		return false
	}
	decoder := json.NewDecoder(stream)
	decoder.DisallowUnknownFields()
	if err := kernel.DecodeJSON(decoder, target); err != nil {
		invoke.WriteJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "decode request: %v", err)))
		return false
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		invoke.WriteJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "request body must contain one JSON object")))
		return false
	}
	return true
}

// RequestJSONStream returns the request body as a bounded JSON stream,
// transparently decompressing Content-Encoding: gzip. The size cap applies to
// the decompressed bytes, so a small gzip payload cannot bypass the limit.
func RequestJSONStream(r *http.Request, limit int) (io.Reader, error) {
	var body io.Reader = r.Body
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("Content-Encoding")), "gzip") {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			return nil, err
		}
		body = gz
	}
	return io.LimitReader(body, int64(limit)+1), nil
}

// MaxServiceRequestBytes bounds one typed service request body. (CAP-ORG-01 B6.)
const MaxServiceRequestBytes = 8 << 20

// DecodeServiceRequest decodes one typed service body with the shared size
// bound; see DecodeServiceRequestBody for explicit limits.
func DecodeServiceRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	return DecodeServiceRequestBody(w, r, target, MaxServiceRequestBytes)
}
