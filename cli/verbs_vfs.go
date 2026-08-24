package cli

import (
	"bytes"
	"encoding/base64"
	"strings"

	"kc/catalog"
	"kc/kernel"
	"kc/repository"
)

// Virtual filesystem verbs: a raw path read/write/list over a Workspace's
// composed tree, no real checkout on disk. This is docs/COMPOSITION.md's
// RawFileStore lifted to `kc`/`kc serve` — the primitive an external agent
// harness's own filesystem provider calls per file, not a second consumption
// path alongside object_id reads (read/list/search stay what they were).
//
// vfs-write's target repository is only known after RouteMount runs inside
// the body (the caller names a --workspace + --path, not a --repo), so it is
// exempted from the generic --repo-flag-based authorize() gate (see
// authorize's exemption switch) and checks the *routed* repository itself,
// the same way `ingest` defers its own check to the `commit` that follows it.

func vfsVerbs() map[string]command {
	return map[string]command{
		"vfs-read":  {stage: stageGoverned, run: verbVFSRead},
		"vfs-list":  {stage: stageGoverned, run: verbVFSList},
		"vfs-write": {stage: stageGoverned, run: verbVFSWrite},
	}
}

func verbVFSRead(cx *invocation) (any, error) {
	workspaceID, err := cx.workspaceID()
	if err != nil {
		return nil, err
	}
	path, err := cx.require("path")
	if err != nil {
		return nil, err
	}
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	def, err := effectiveWorkspace(cx.WS, cx.Home, cat, workspaceID, cx.Flags)
	if err != nil {
		return nil, err
	}
	resolved, err := resolveOrReplay(cx.WS, cx.Home, cat, workspaceID, cx.Flags)
	if err != nil {
		return nil, err
	}
	file, err := cat.ReadVirtualFileAt(def, resolved, path)
	if err != nil {
		return nil, err
	}
	if !allowedRepoRead(cx.Home, cx.Flags, string(file.Repository), "") {
		return nil, kernel.Fail(kernel.ErrForbidden, "%s is not allowed to read %s", cx.flag("as"), file.Repository)
	}
	return map[string]any{
		"path":       file.Path,
		"repository": file.Repository,
		"commit":     file.Commit,
		"encoding":   "base64",
		"content":    base64.StdEncoding.EncodeToString(file.Content),
	}, nil
}

func verbVFSList(cx *invocation) (any, error) {
	workspaceID, err := cx.workspaceID()
	if err != nil {
		return nil, err
	}
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	def, err := effectiveWorkspace(cx.WS, cx.Home, cat, workspaceID, cx.Flags)
	if err != nil {
		return nil, err
	}
	resolved, err := resolveOrReplay(cx.WS, cx.Home, cat, workspaceID, cx.Flags)
	if err != nil {
		return nil, err
	}
	entries, err := cat.ListVirtualFilesAt(def, resolved)
	if err != nil {
		return nil, err
	}
	mounts, err := catalog.ListVirtualMountsAt(def, resolved)
	if err != nil {
		return nil, err
	}
	prefix := cx.flag("prefix")
	out := []catalog.VirtualEntry{}
	for _, e := range entries {
		if prefix != "" && !strings.HasPrefix(e.Path, prefix) {
			continue
		}
		if !allowedRepoRead(cx.Home, cx.Flags, string(e.Repository), "") {
			continue
		}
		out = append(out, e)
	}
	visibleMounts := []catalog.VirtualMount{}
	for _, mount := range mounts {
		// Mount membership is governed metadata too. Do not reveal an empty or
		// otherwise unreadable repository merely because the recipe names it.
		if !allowedRepoRead(cx.Home, cx.Flags, string(mount.Repository), "") {
			continue
		}
		visibleMounts = append(visibleMounts, mount)
	}
	return map[string]any{"entries": out, "mounts": visibleMounts}, nil
}

func verbVFSWrite(cx *invocation) (any, error) {
	workspaceID, err := cx.workspaceID()
	if err != nil {
		return nil, err
	}
	path, err := cx.require("path")
	if err != nil {
		return nil, err
	}
	commandID, err := cx.require("command-id")
	if err != nil {
		return nil, err
	}
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	def, err := effectiveWorkspace(cx.WS, cx.Home, cat, workspaceID, cx.Flags)
	if err != nil {
		return nil, err
	}
	route, err := catalog.RouteMount(def, path)
	if err != nil {
		return nil, err
	}
	if err := authorizeRoutedWrite(cx, route.Repository); err != nil {
		return nil, err
	}
	remove := FlagBool(cx.Flags, "remove")
	var content []byte
	if !remove {
		encoded := cx.flag("content")
		if encoded == "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "vfs-write requires --content (base64) unless --remove")
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "--content must be base64: %v", err)
		}
		content = decoded
	}
	targetRef := cx.targetRef("ref")
	// An omitted --base means "use the Workspace pin for the first attempt",
	// not "make the base part of the caller's logical payload". On a retry —
	// including after service restart — replay the persisted canonical request
	// before resolving today's HEAD. Otherwise an identical VFS retry would
	// manufacture a different base and falsely become IDEMPOTENCY_CONFLICT.
	if prior, ok := cx.WS.Writer.Lookup(commandID); ok && prior.Request.RawChangeSet != nil {
		stored := prior.Request.RawChangeSet
		same := stored.TargetRepository == route.Repository && stored.TargetRef == targetRef &&
			stored.Message == cx.flag("message") && len(stored.Changes) == 1 &&
			stored.Changes[0].Path == route.Path && stored.Changes[0].Remove == remove &&
			bytes.Equal(stored.Changes[0].Content, content)
		if explicit := kernel.CommitID(cx.flag("base")); explicit != "" && stored.BaseCommit != explicit {
			same = false
		}
		if explicit := kernel.CommitID(cx.flag("expected")); explicit != "" && stored.ExpectedTargetCommit != explicit {
			same = false
		}
		if same {
			replay := *stored
			// These are server stamps added after the idempotency digest was
			// calculated on the first attempt; restore the caller-shaped request.
			replay.Author = ""
			replay.RequestID = ""
			replay.RuleID = ""
			return cx.WS.Writer.RawWrite(commandID, replay)
		}
	}
	// --base pins the precondition to what the caller already read (vfs-read
	// and vfs-list both return commit): that is what makes a retry with the
	// same --command-id an actual replay instead of a fresh write against a
	// ref that has since moved, and what turns a genuine race into
	// NON_FAST_FORWARD instead of a silent lost update. Omit it only for a
	// first write to a path nobody is racing on.
	base := kernel.CommitID(cx.flag("base"))
	if base == "" {
		resolved, err := resolveOrReplay(cx.WS, cx.Home, cat, workspaceID, cx.Flags)
		if err != nil {
			return nil, err
		}
		b, ok := resolved.Repositories[route.Repository]
		if !ok {
			return nil, kernel.Fail(kernel.ErrWorkspaceInvalid, "resolved pin has no commit for repository %s", route.Repository)
		}
		base = b
	}
	expected := kernel.CommitID(cx.flag("expected"))
	if expected == "" {
		expected = base
	}
	return cx.WS.Writer.RawWrite(commandID, repository.RawFileChangeSet{
		TargetRepository:     route.Repository,
		TargetRef:            targetRef,
		BaseCommit:           base,
		ExpectedTargetCommit: expected,
		Changes:              []repository.RawFileChange{{Path: route.Path, Content: content, Remove: remove}},
		Message:              cx.flag("message"),
	})
}

// authorizeRoutedWrite re-runs the allow.json gate with the routed
// repository standing in for --repo, since the caller only ever named
// --workspace + --path: the target repository is a fact of the recipe, not
// something the caller chose, so it cannot be checked before RouteMount runs.
func authorizeRoutedWrite(cx *invocation, target kernel.RepositoryID) error {
	scoped := make(map[string]FlagValue, len(cx.Flags)+1)
	for k, v := range cx.Flags {
		// authorize() skips "commit" when --workspace/--workspace is set so the
		// verb can route first. This IS the post-route check: drop those
		// flags or the grant on --repo never runs and --as can write.
		if k == "workspace" {
			continue
		}
		scoped[k] = v
	}
	scoped["repo"] = string(target)
	return authorize(cx.Home, "commit", scoped)
}
