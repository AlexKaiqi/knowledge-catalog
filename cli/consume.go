package cli

import (
	"fmt"
	"os"
	"strings"

	"kc/catalog"
	"kc/kernel"
	"kc/knowledge"
	"kc/reader"
)

func servingWorkspace(flags map[string]FlagValue) bool {
	return workspaceIDOf(flags) != "" && !readingCatalogCommand(flags)
}

func workspaceIDOf(flags map[string]FlagValue) string {
	return FlagString(flags, "workspace")
}

func workspaceIDFlag(flags map[string]FlagValue) (string, error) {
	workspace := FlagString(flags, "workspace")
	if workspace == "" {
		return "", fmt.Errorf("missing --workspace")
	}
	return workspace, nil
}

func readingCatalogCommand(flags map[string]FlagValue) bool {
	if FlagString(flags, "repo") != "" || FlagString(flags, "commit") != "" || FlagString(flags, "ref") != "" {
		return false
	}
	if FlagString(flags, "object") != "" || FlagString(flags, "aspect") != "" {
		return false
	}
	if workspaceIDOf(flags) != "" {
		return false
	}
	_, ok := flags["catalog"]
	return ok
}

func readingCatalog(command string, flags map[string]FlagValue) bool {
	return command == "read" && readingCatalogCommand(flags)
}

func consumerAllowCmd(command string, flags map[string]FlagValue) string {
	if readingCatalog(command, flags) {
		return "read-catalog"
	}
	if servingWorkspace(flags) {
		switch command {
		case "read", "list", "relations", "search", "provenance", "describe-schema", "resolve", "resolve-binding", "log", "checkout", "inspect", "describe-access",
			"sync", "vfs-read", "vfs-list":
			return "read-workspace"
		}
	}
	return command
}

func rejectRemovedFlags(flags map[string]FlagValue) error {
	for _, name := range []string{"view", "release", "generation", "base-generation", "input-vrv"} {
		if _, exists := flags[name]; exists {
			return fmt.Errorf("unknown flag --%s", name)
		}
	}
	return nil
}

func openServing(ws *Home, flags map[string]FlagValue) (*reader.Serving, *catalog.Catalog, error) {
	if FlagString(flags, "repo") != "" || FlagString(flags, "commit") != "" || FlagString(flags, "ref") != "" {
		return nil, nil, fmt.Errorf("--workspace cannot be combined with --repo, --commit, or --ref")
	}
	cat, err := pickCatalog(ws, flags)
	if err != nil {
		return nil, nil, err
	}
	workspaceID, err := workspaceIDFlag(flags)
	if err != nil {
		return nil, nil, err
	}
	resolved, err := resolveOrReplay(ws, ws.Dir, cat, workspaceID, flags)
	if err != nil {
		return nil, nil, err
	}
	serving := reader.Open(knowledge.Lookup(cat.Require), workspacePin(resolved))
	return serving, cat, nil
}

// openCompleteServing is for consumer operations whose public response has no
// coverage envelope (READ/LIST/LOG/PROVENANCE and similar exact reads). Those
// operations must not silently turn an authorization gap into an empty or
// apparently complete result. SEARCH has its own partial-coverage envelope and
// therefore performs authorization-aware fan-out in searchWorkspace instead.
func openCompleteServing(ws *Home, flags map[string]FlagValue, object string) (*reader.Serving, *catalog.Catalog, error) {
	serving, cat, err := openServing(ws, flags)
	if err != nil {
		return nil, nil, err
	}
	if err := requireCompleteWorkspaceRead(ws.Dir, flags, serving.Pin(), object); err != nil {
		return nil, nil, err
	}
	return serving, cat, nil
}

// requireCompleteWorkspaceRead fails closed when a bare-result consumer
// operation cannot see every member at the requested object scope. The error is
// intentionally generic: callers learn that the Workspace view is incomplete,
// not which hidden repository may contain the object.
func requireCompleteWorkspaceRead(home string, flags map[string]FlagValue, pin reader.WorkspacePin, object string) error {
	if ownerBypass(flags) {
		return nil
	}
	for repositoryID := range pin.Repositories {
		if !allowedRepoRead(home, flags, string(repositoryID), object) {
			return kernel.Fail(kernel.ErrForbidden, "workspace read is incomplete because one or more members are not authorized")
		}
	}
	return nil
}

// searchVisiblePin returns the repositories that may participate in discovery.
// Search requires a repository-wide read grant: object-scoped grants cannot
// safely authorize discovery of objects the caller does not know yet.
func searchVisiblePin(home string, flags map[string]FlagValue, pin reader.WorkspacePin) (reader.WorkspacePin, int) {
	if ownerBypass(flags) {
		return pin, 0
	}
	visible := reader.WorkspacePin{
		WorkspaceID:  pin.WorkspaceID,
		Revision:     pin.Revision,
		Repositories: map[kernel.RepositoryID]kernel.CommitID{},
	}
	omitted := 0
	for repositoryID, commitID := range pin.Repositories {
		if allowedRepoRead(home, flags, string(repositoryID), "") {
			visible.Repositories[repositoryID] = commitID
			continue
		}
		omitted++
	}
	return visible, omitted
}

func resolveOrReplay(ws *Home, home string, cat *catalog.Catalog, workspaceID string, flags map[string]FlagValue) (catalog.ResolvedWorkspace, error) {
	def, err := effectiveWorkspace(ws, home, cat, workspaceID, flags)
	if err != nil {
		return catalog.ResolvedWorkspace{}, err
	}
	if pinPath := FlagString(flags, "pin"); pinPath != "" {
		resolved, replayErr := replayPin(cat, def, pinPath)
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

func replayPin(cat *catalog.Catalog, def catalog.WorkspaceDefinition, pinPath string) (catalog.ResolvedWorkspace, error) {
	raw := []byte(pinPath)
	if !strings.HasPrefix(strings.TrimSpace(pinPath), "{") {
		var err error
		raw, err = os.ReadFile(pinPath)
		if err != nil {
			return catalog.ResolvedWorkspace{}, err
		}
	}
	var pin catalog.ResolvedWorkspace
	if err := catalog.DecodeJSON(raw, &pin); err != nil {
		return catalog.ResolvedWorkspace{}, kernel.Fail(kernel.ErrUsageInvalid, "--pin is not a ResolvedWorkspace JSON file")
	}
	workspaceID := def.WorkspaceID
	if pin.WorkspaceID != "" && pin.WorkspaceID != workspaceID {
		return catalog.ResolvedWorkspace{}, kernel.Fail(kernel.ErrUsageInvalid, "--pin is for workspace %s, not %s", pin.WorkspaceID, workspaceID)
	}
	pin.WorkspaceID = workspaceID
	want := map[kernel.RepositoryID]struct{}{}
	for _, src := range def.Sources {
		want[src.Repository] = struct{}{}
	}
	if len(want) != len(pin.Repositories) {
		return catalog.ResolvedWorkspace{}, kernel.Fail(kernel.ErrUsageInvalid, "--pin membership does not match workspace %s", workspaceID)
	}
	for id := range pin.Repositories {
		if _, ok := want[id]; !ok {
			return catalog.ResolvedWorkspace{}, kernel.Fail(kernel.ErrUsageInvalid, "--pin names repository %s which is not in workspace %s", id, workspaceID)
		}
	}
	if check := cat.CheckResolved(pin); check.Outcome != "PASSED" {
		return catalog.ResolvedWorkspace{}, kernel.Fail(kernel.ErrWorkspaceInvalid, "replayed pin failed CheckResolved")
	}
	return pin, nil
}

func workspacePin(resolved catalog.ResolvedWorkspace) reader.WorkspacePin {
	return reader.WorkspacePin{WorkspaceID: resolved.WorkspaceID, Revision: resolved.Revision, Repositories: resolved.Repositories}
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
	file, err := ReadAllow(home)
	if err != nil {
		return true
	}
	_, ok := MatchAllow(file.Rules, AllowQuery{Principal: FlagString(flags, "as"), Cmd: "read", Repo: repo, Object: object})
	return ok
}

func filterWorkspaceReads(home string, flags map[string]FlagValue, _ *catalog.Catalog, values []reader.FederatedValue) []reader.FederatedValue {
	out := []reader.FederatedValue{}
	for _, item := range values {
		if allowedRepoRead(home, flags, string(item.Repository), string(item.ObjectID)) {
			out = append(out, item)
		}
	}
	return out
}

func filterKnowledgeReads(home string, flags map[string]FlagValue, values []knowledge.KnowledgeValue) []knowledge.KnowledgeValue {
	out := []knowledge.KnowledgeValue{}
	for _, item := range values {
		if allowedRepoRead(home, flags, string(item.Repository), string(item.Address.ObjectID)) {
			out = append(out, item)
		}
	}
	return out
}

func filterWorkspaceLogs(home string, flags map[string]FlagValue, logs []reader.ObjectLog) []reader.ObjectLog {
	out := []reader.ObjectLog{}
	for _, item := range logs {
		if allowedRepoRead(home, flags, string(item.Repository), string(item.ObjectID)) {
			out = append(out, item)
		}
	}
	return out
}
