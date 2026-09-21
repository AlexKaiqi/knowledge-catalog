package cli

import (
	"fmt"
	"os"
	"strings"

	"kc/catalog"
	"kc/delivery"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	knowledgeserving "kc/knowledge/serving"
	"kc/retrieval"
)

// Pin, Serving and consumer authorization shared by workspace read/search.
// Product file entry remains Workspace File Gateway / kcfs.

func servingWorkspace(flags map[string]FlagValue) bool {
	return suppliedKnowledgeSet(flags) != nil ||
		FlagString(flags, "pin") != "" ||
		FlagString(flags, "dataset-file") != "" ||
		(setIDOf(flags) != "" && !readingCatalogCommand(flags))
}

func setIDOf(flags map[string]FlagValue) string {
	return FlagString(flags, "dataset")
}

func setIDFlag(flags map[string]FlagValue) (string, error) {
	workspace := setIDOf(flags)
	if workspace == "" {
		return "", fmt.Errorf("missing --dataset")
	}
	return workspace, nil
}

func readingCatalogCommand(flags map[string]FlagValue) bool {
	if suppliedKnowledgeSet(flags) != nil {
		return false
	}
	if FlagString(flags, "repo") != "" || FlagString(flags, "commit") != "" || FlagString(flags, "ref") != "" {
		return false
	}
	if FlagString(flags, "object") != "" || FlagString(flags, "aspect") != "" {
		return false
	}
	if setIDOf(flags) != "" {
		return false
	}
	_, ok := flags["catalog"]
	return ok
}

func rejectRemovedFlags(flags map[string]FlagValue) error {
	for _, name := range []string{"view", "release", "generation", "base-generation", "input-vrv", "session-id"} {
		if _, exists := flags[name]; exists {
			return fmt.Errorf("unknown flag --%s", name)
		}
	}
	return nil
}

func rejectMixedKnowledgeBasis(flags map[string]FlagValue) error {
	if FlagString(flags, "dataset") == "" && suppliedKnowledgeSet(flags) == nil && FlagString(flags, "pin") == "" && FlagString(flags, "dataset-file") == "" {
		return nil
	}
	if FlagString(flags, "repo") != "" || FlagString(flags, "commit") != "" || FlagString(flags, "ref") != "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "choose a Workspace definition/pin or a Repository basis; do not mix")
	}
	return nil
}

func openServing(ws *Home, flags map[string]FlagValue) (*reader.Serving, *catalog.Catalog, error) {
	return openServingAt(nil, ws, flags)
}

func openServingAt(cx *invocation, ws *Home, flags map[string]FlagValue) (*reader.Serving, *catalog.Catalog, error) {
	if FlagString(flags, "repo") != "" || FlagString(flags, "commit") != "" || FlagString(flags, "ref") != "" {
		if err := rejectMixedKnowledgeBasis(flags); err != nil {
			return nil, nil, err
		}
		return nil, nil, kernel.Fail(kernel.ErrUsageInvalid, "choose a Workspace definition/pin or a Repository basis; do not mix")
	}
	cat, err := pickCatalog(ws, flags)
	if err != nil {
		return nil, nil, err
	}
	setID := setIDOf(flags)
	if setID == "" && suppliedKnowledgeSet(flags) == nil &&
		FlagString(flags, "pin") == "" && FlagString(flags, "dataset-file") == "" {
		return nil, nil, fmt.Errorf("missing --dataset or temporary definition")
	}
	home := ""
	if ws != nil {
		home = ws.Dir
	}
	resolved, err := resolveOrReplay(ws, home, cat, setID, flags)
	if err != nil {
		return nil, nil, err
	}
	serving := reader.Open(ws.Reader.Lookup(cat.Require), knowledgeSetPin(resolved))
	serving.SetHydrator(ws.Hydrator)
	return serving, cat, nil
}

// openCompleteServing is for consumer operations whose public response has no
// coverage envelope (READ/RESOLVE/RELATIONS/LOG/PROVENANCE and similar exact reads). Those
// operations must not silently turn an authorization gap into an empty or
// apparently complete result. SEARCH keeps every pin member in the candidate
// set and applies the delivery chain after hydrate.
func openCompleteServing(cx *invocation, object string) (*reader.Serving, *catalog.Catalog, error) {
	serving, cat, err := openServingAt(cx, cx.WS, cx.Flags)
	if err != nil {
		return nil, nil, err
	}
	if err := requireCompleteWorkspaceRead(cx.WS.Dir, cx.Flags, serving.Pin(), object); err != nil {
		return nil, nil, err
	}
	return serving, cat, nil
}

// requireCompleteWorkspaceRead fails closed when a bare-result consumer
// operation cannot see every member at the requested object scope. The error is
// intentionally generic: callers learn that the Workspace read is incomplete,
// not which hidden repository may contain the object.
func requireCompleteWorkspaceRead(home string, flags map[string]FlagValue, pin reader.KnowledgeSetPin, object string) error {
	if ownerBypass(flags) {
		return nil
	}
	if allowedDatasetFileRead(home, flags) {
		return nil
	}
	for repositoryID := range pin.Repositories {
		if !allowedRepoRead(home, flags, string(repositoryID), object) {
			return kernel.Fail(kernel.ErrForbidden, "workspace read is incomplete because one or more members are not authorized")
		}
	}
	return nil
}

// searchVisiblePin returns every pin member. Dataset path membership is
// checked before hydrate; unauthorized knowledge.read bodies are stripped
// after hydrate by the delivery chain.
func searchVisiblePin(_ string, _ map[string]FlagValue, pin reader.KnowledgeSetPin) (reader.KnowledgeSetPin, int) {
	return pin, 0
}

func deliverSearchHit(home string, flags map[string]FlagValue, hit retrieval.KnowledgeHit) (retrieval.KnowledgeHit, error) {
	env, err := knowledgeDelivery(home, flags).Apply(delivery.Context{Principal: FlagString(flags, "as")}, delivery.FromValue(hit.Knowledge, hit.Version.Observations))
	if err != nil {
		return retrieval.KnowledgeHit{}, err
	}
	hit.Knowledge, hit.Version.Observations = env.WriteBody(hit.Knowledge)
	return hit, nil
}

func deliverSearchResult(home string, flags map[string]FlagValue, out retrieval.SearchResult) (retrieval.SearchResult, error) {
	for i := range out.Hits {
		hit, err := deliverSearchHit(home, flags, out.Hits[i])
		if err != nil {
			return retrieval.SearchResult{}, err
		}
		out.Hits[i] = hit
	}
	return out, nil
}

func knowledgeDelivery(home string, flags map[string]FlagValue) delivery.Chain {
	return delivery.Chain{delivery.RepositoryRead{Allowed: func(_ string, ref knowledge.KnowledgeRef) bool {
		return allowedRepoRead(home, flags, string(ref.Repository), string(ref.Object))
	}}}
}

func resolveOrReplay(ws *Home, home string, cat *catalog.Catalog, setID string, flags map[string]FlagValue) (catalog.ResolvedKnowledgeSet, error) {
	return resolveKnowledgeSet(ws, home, cat, setID, flags)
}

func resolveKnowledgeSet(ws *Home, home string, cat *catalog.Catalog, setID string, flags map[string]FlagValue) (catalog.ResolvedKnowledgeSet, error) {
	var def catalog.KnowledgeSet
	if supplied := suppliedKnowledgeSet(flags); supplied != nil {
		// Temporary definitions replay through the pin; a caller label must not
		// borrow a published Workspace grant when an explicit recipe is present.
		setID = ""
		def = *supplied
		if err := catalog.ValidateKnowledgeSet(def); err != nil {
			return catalog.ResolvedKnowledgeSet{}, err
		}
		for _, source := range def.Sources {
			if !cat.HasRepository(source.Repository) {
				return catalog.ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "repository %s is not registered in this catalog", source.Repository)
			}
		}
	} else {
		var err error
		def, err = ensureWorkspace(ws, home, cat, setID)
		// The local unrestricted owner may inspect a personal overlay. It is
		// never used for a server identity's named Dataset grant or pin replay.
		if err == nil && ws.Deployment == nil && ownerBypass(flags) && FlagString(flags, "pin") == "" {
			def, err = applyOverlay(home, "", def)
		}
		if err != nil {
			return catalog.ResolvedKnowledgeSet{}, err
		}
	}
	if pinPath := FlagString(flags, "pin"); pinPath != "" {
		var resolved catalog.ResolvedKnowledgeSet
		pin, replayErr := parseReplayPin(pinPath)
		if replayErr == nil {
			if suppliedKnowledgeSet(flags) == nil {
				if pin.SetID != "" && pin.SetID != setID {
					return catalog.ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrUsageInvalid, "pin names a different dataset")
				}
				pin.SetID = setID
				resolved, replayErr = cat.ReplayPublished(pin)
			} else {
				resolved, replayErr = replayPin(cat, def, pinPath)
			}
		}
		if replayErr == nil {
			flags[resolvedPinFlag] = resolved.PinID
		}
		return resolved, replayErr
	}
	resolved, resolveErr := cat.ResolveDefinition(def)
	if resolveErr == nil {
		flags[resolvedPinFlag] = resolved.PinID
	}
	return resolved, resolveErr
}

func replayPin(cat *catalog.Catalog, def catalog.KnowledgeSet, pinPath string) (catalog.ResolvedKnowledgeSet, error) {
	pin, err := decodeReplayPin(def, pinPath)
	if err != nil {
		return catalog.ResolvedKnowledgeSet{}, err
	}
	if check := cat.CheckResolved(pin); check.Outcome != "PASSED" {
		return catalog.ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "replayed pin failed CheckResolved")
	}
	return pin, nil
}

func decodeReplayPin(def catalog.KnowledgeSet, pinPath string) (catalog.ResolvedKnowledgeSet, error) {
	pin, err := parseReplayPin(pinPath)
	if err != nil {
		return catalog.ResolvedKnowledgeSet{}, err
	}
	return catalog.ReplayDefinition(def, pin)
}

func parseReplayPin(pinPath string) (catalog.ResolvedKnowledgeSet, error) {
	raw := []byte(pinPath)
	if !strings.HasPrefix(strings.TrimSpace(pinPath), "{") {
		var err error
		raw, err = os.ReadFile(pinPath)
		if err != nil {
			return catalog.ResolvedKnowledgeSet{}, err
		}
	}
	var pin catalog.ResolvedKnowledgeSet
	if err := catalog.DecodeJSON(raw, &pin); err != nil {
		return catalog.ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrUsageInvalid, "--pin is not a ResolvedKnowledgeSet JSON file")
	}
	return pin, nil
}

func knowledgeSetPin(resolved catalog.ResolvedKnowledgeSet) reader.KnowledgeSetPin {
	items := make([]reader.DatasetItem, 0, len(resolved.Items))
	for _, item := range resolved.Items {
		items = append(items, reader.DatasetItem{
			Target: item.Target, Repository: item.Repository, Commit: item.Commit,
			Kind: item.Kind, Prefix: item.Prefix, File: item.File,
		})
	}
	return reader.KnowledgeSetPin{SetID: resolved.SetID, Revision: resolved.Revision, Repositories: resolved.Repositories, Items: items}
}

func aspectSelectorFrom(flags map[string]FlagValue) *knowledge.AspectSelector {
	include := FlagStrings(flags, "include")
	exclude := FlagStrings(flags, "exclude")
	if len(include) == 0 && len(exclude) == 0 {
		return nil
	}
	return &knowledge.AspectSelector{Include: include, Exclude: exclude}
}

func allowedRepoRead(home string, flags map[string]FlagValue, repo, object string) bool {
	if ownerBypass(flags) {
		return true
	}
	if servingWorkspace(flags) && allowedDatasetFileRead(home, flags) {
		return true
	}
	as := FlagString(flags, "as")
	if repositoryAuthenticatedAllowed(home, as, "knowledge.read", repo) {
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

func allowedDatasetFileRead(home string, flags map[string]FlagValue) bool {
	as := FlagString(flags, "as")
	if as == "" {
		return true
	}
	file, err := ReadAllow(home)
	if err != nil {
		return false
	}
	q := AllowQuery{
		Principal: as,
		Action:    "file.read",
		Catalog:   FlagString(flags, "catalog"),
		Dataset:   setIDOf(flags),
	}
	if q.Catalog == "" {
		q.Catalog = FlagString(flags, "_default-catalog")
	}
	if q.Catalog == "" {
		q.Catalog = defaultAllowCatalog(home, "")
	}
	_, ok := MatchAllow(file.Rules, q)
	return ok
}

func filterKnowledgeServingReads(home string, flags map[string]FlagValue, _ *catalog.Catalog, values []knowledgeserving.ReadResult) []knowledgeserving.ReadResult {
	out := []knowledgeserving.ReadResult{}
	for _, item := range values {
		if allowedRepoRead(home, flags, string(item.Repository), string(item.ObjectID)) {
			out = append(out, item)
		}
	}
	return out
}
