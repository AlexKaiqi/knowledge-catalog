package testkit

import (
	"testing"

	"kc/kernel"
	"kc/repository"
)

// RepositoryContract runs T12 against any Snapshot+Knowledge factory.
// SnapshotStore: ID, Head, GetRef, HasCommit, CreateRef, Merge, ApplyCommit, Archive.
// Knowledge: Resolve, Read, ResolveAddress, ReadAddress, GetProvenance, Log, Diff, Search, List.
// Dynamic observations are intentionally outside the Snapshot contract.
//
// Args:
//
//	t: test handle.
//	create: builds an empty repository for the given id.
func RepositoryContract(t *testing.T, create func(t *testing.T, id string) repository.Repository) {
	t.Helper()

	t.Run("preserves object identity across path moves and pinned versions", func(t *testing.T) {
		repo := create(t, "kr://conformance/identity")
		if repo.ID() != kernel.RepositoryID("kr://conformance/identity") {
			t.Fatalf("id %s", repo.ID())
		}
		root := MustHead(t, repo, "refs/heads/main")
		if head, ok := repo.GetRef("HEAD"); !ok || head != root {
			t.Fatalf("HEAD %s %v", head, ok)
		}
		first, err := repo.ApplyCommit(CommitChange(repo.ID(), root, "policy/P-1", map[string]any{"version": 1}, "policies/P-1.json"))
		if err != nil {
			t.Fatal(err)
		}
		second, err := repo.ApplyCommit(CommitChange(repo.ID(), first, "policy/P-1", map[string]any{"version": 2}, "archive/P-1.json"))
		if err != nil {
			t.Fatal(err)
		}
		res, err := repo.Resolve("policy/P-1", second)
		if err != nil || res.PathHint != "archive/P-1.json" {
			t.Fatalf("%#v %v", res, err)
		}
		v1, err := repo.Read("policy/P-1", first)
		if err != nil || asInt(v1.Value.(map[string]any)["version"]) != 1 {
			t.Fatalf("%#v %v", v1, err)
		}
	})

	t.Run("rejects stale ref preconditions", func(t *testing.T) {
		repo := create(t, "kr://conformance/cas")
		root := MustHead(t, repo, "refs/heads/main")
		if _, err := repo.ApplyCommit(CommitChange(repo.ID(), root, "a", 1, "")); err != nil {
			t.Fatal(err)
		}
		_, err := repo.ApplyCommit(CommitChange(repo.ID(), root, "b", 2, ""))
		ExpectCode(t, err, kernel.ErrNonFastForward)
	})

	t.Run("distinguishes unresolved version from absent object", func(t *testing.T) {
		repo := create(t, "kr://conformance/version")
		root := MustHead(t, repo, "refs/heads/main")
		_, err := repo.Read("absent", root)
		ExpectCode(t, err, kernel.ErrKnowledgeRefUnresolved)
		_, err = repo.Read("absent", "missing-version")
		ExpectCode(t, err, kernel.ErrVersionUnresolved)
		_, err = repo.Head("refs/heads/missing")
		if kernel.AsIngress(err) == nil {
			t.Fatalf("expected IngressError, got %v", err)
		}
		if _, ok := repo.GetRef("refs/heads/missing"); ok {
			t.Fatal("missing ref should not resolve")
		}
		if !repo.HasCommit(root) {
			t.Fatal("head commit must exist")
		}
		if repo.HasCommit("missing-version") {
			t.Fatal("unknown sha must not HasCommit")
		}
	})

	t.Run("treats aspect writes as independent units", func(t *testing.T) {
		repo := create(t, "kr://conformance/aspect")
		root := MustHead(t, repo, "refs/heads/main")
		first, err := repo.ApplyCommit(repository.CommitChangeSet{
			TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
			BaseCommit: root, ExpectedTargetCommit: root,
			Operations: []repository.Operation{{
				Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: "dataset/1", AspectName: "structure"}, Value: map[string]any{"cols": 1},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		second, err := repo.ApplyCommit(repository.CommitChangeSet{
			TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
			BaseCommit: first, ExpectedTargetCommit: first,
			Operations: []repository.Operation{{
				Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: "dataset/1", AspectName: "ownership"}, Value: map[string]any{"owner": "ops"},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		value, err := repo.Read("dataset/1", second)
		if err != nil {
			t.Fatal(err)
		}
		got := value.Value.(map[string]any)
		if asInt(got["structure"].(map[string]any)["cols"]) != 1 {
			t.Fatalf("%#v", value.Value)
		}
		addr := kernel.Address{Kind: kernel.KindAspect, ObjectID: "dataset/1", AspectName: "structure"}
		unit, err := repo.ReadAddress(addr, second)
		if err != nil || asInt(unit.Value.(map[string]any)["cols"]) != 1 {
			t.Fatalf("readAddress %#v %v", unit, err)
		}
		res, err := repo.ResolveAddress(addr, second)
		if err != nil || res.Status != repository.StatusResolved {
			t.Fatalf("resolveAddress %#v %v", res, err)
		}
	})

	t.Run("logs introducing commits and diffs two pinned versions", func(t *testing.T) {
		repo := create(t, "kr://conformance/log")
		root := MustHead(t, repo, "refs/heads/main")
		first, err := repo.ApplyCommit(CommitChange(repo.ID(), root, "policy/P-1", map[string]any{"version": 1}, ""))
		if err != nil {
			t.Fatal(err)
		}
		second, err := repo.ApplyCommit(CommitChange(repo.ID(), first, "policy/P-1", map[string]any{"version": 2}, ""))
		if err != nil {
			t.Fatal(err)
		}
		later, err := repo.ApplyCommit(CommitChange(repo.ID(), second, "other/unrelated", map[string]any{"x": 1}, ""))
		if err != nil {
			t.Fatal(err)
		}
		history, err := repo.Log("policy/P-1", later, 0)
		if err != nil {
			t.Fatal(err)
		}
		if history[0].Commit != second {
			t.Fatalf("%#v", history)
		}
		sawFirst, sawLater := false, false
		for _, item := range history {
			if item.Commit == first {
				sawFirst = true
			}
			if item.Commit == later {
				sawLater = true
			}
		}
		if !sawFirst || sawLater {
			t.Fatalf("history %#v", history)
		}
		delta, err := repo.Diff("policy/P-1", first, second)
		if err != nil {
			t.Fatal(err)
		}
		if asInt(delta.From.Value.(map[string]any)["version"]) != 1 {
			t.Fatalf("%#v", delta)
		}
	})

	t.Run("lists live objects and keeps provenance on the unit", func(t *testing.T) {
		repo := create(t, "kr://conformance/list")
		root := MustHead(t, repo, "refs/heads/main")
		first, err := repo.ApplyCommit(repository.CommitChangeSet{
			TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
			BaseCommit: root, ExpectedTargetCommit: root, Message: "seed",
			Provenance: &kernel.ProvenanceEnvelope{OriginKind: kernel.OriginSource, SourceRefs: []string{"handbook"}},
			Operations: PutEntity("policy/A", map[string]any{"v": 1, "note": "handbook"}, ""),
		})
		if err != nil {
			t.Fatal(err)
		}
		second, err := repo.ApplyCommit(CommitChange(repo.ID(), first, "policy/B", map[string]any{"v": 2}, ""))
		if err != nil {
			t.Fatal(err)
		}
		listed, err := repo.List(second)
		if err != nil || len(listed) != 2 {
			t.Fatalf("list %d %v", len(listed), err)
		}
		ids := map[string]bool{}
		for _, item := range listed {
			ids[string(item.Address.ObjectID)] = true
		}
		if !ids["policy/A"] || !ids["policy/B"] {
			t.Fatalf("%#v", ids)
		}
		prov, err := repo.GetProvenance("policy/A", first)
		if err != nil || len(prov.Chain) == 0 || prov.Chain[0].OriginKind != kernel.OriginSource {
			t.Fatalf("%#v %v", prov, err)
		}
		hits, err := repo.Search("handbook", first)
		if err != nil || len(hits) == 0 {
			t.Fatalf("contains search missed provenance/value: %d %v", len(hits), err)
		}
	})

	t.Run("remove marks the object REMOVED and keeps the prior commit readable", func(t *testing.T) {
		repo := create(t, "kr://conformance/remove")
		root := MustHead(t, repo, "refs/heads/main")
		first, err := repo.ApplyCommit(CommitChange(repo.ID(), root, "policy/drop", map[string]any{"v": 1}, ""))
		if err != nil {
			t.Fatal(err)
		}
		second, err := repo.ApplyCommit(repository.CommitChangeSet{
			TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
			BaseCommit: first, ExpectedTargetCommit: first,
			Operations: []repository.Operation{{
				Op: repository.OpRemove, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "policy/drop"},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		res, err := repo.Resolve("policy/drop", second)
		if err != nil || res.Status != repository.StatusRemoved {
			t.Fatalf("%#v %v", res, err)
		}
		_, err = repo.Read("policy/drop", second)
		ExpectCode(t, err, kernel.ErrKnowledgeRefUnresolved)
		kept, err := repo.Read("policy/drop", first)
		if err != nil || asInt(kept.Value.(map[string]any)["v"]) != 1 {
			t.Fatalf("%#v %v", kept, err)
		}
		listed, err := repo.List(second)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range listed {
			if item.Address.ObjectID == "policy/drop" {
				t.Fatalf("removed object still listed: %#v", listed)
			}
		}
	})

	t.Run("rejects IfAbsent on an existing unit", func(t *testing.T) {
		repo := create(t, "kr://conformance/if-absent")
		root := MustHead(t, repo, "refs/heads/main")
		first, err := repo.ApplyCommit(CommitChange(repo.ID(), root, "policy/A", 1, ""))
		if err != nil {
			t.Fatal(err)
		}
		_, err = repo.ApplyCommit(repository.CommitChangeSet{
			TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
			BaseCommit: first, ExpectedTargetCommit: first,
			Operations: []repository.Operation{{
				Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "policy/A"}, Value: 2,
				Precondition: &repository.Precondition{Type: repository.IfAbsent},
			}},
		})
		ExpectCode(t, err, kernel.ErrPreconditionFailed)
		if MustHead(t, repo, "refs/heads/main") != first {
			t.Fatal("failed IfAbsent moved HEAD")
		}
	})

	t.Run("merges a fast-forward candidate and rejects a diverged one", func(t *testing.T) {
		repo := create(t, "kr://conformance/merge")
		root := MustHead(t, repo, "refs/heads/main")
		if err := repo.CreateRef("refs/heads/candidate", root); err != nil {
			t.Fatal(err)
		}
		candidate, err := repo.ApplyCommit(repository.CommitChangeSet{
			TargetRepository: repo.ID(), TargetRef: "refs/heads/candidate",
			BaseCommit: root, ExpectedTargetCommit: root,
			Operations: PutEntity("from-candidate", map[string]any{"v": 1}, ""),
		})
		if err != nil {
			t.Fatal(err)
		}
		merged, err := repo.Merge("refs/heads/main", candidate, root)
		if err != nil || merged != candidate {
			t.Fatalf("ff merge %s %v", merged, err)
		}
		if MustHead(t, repo, "refs/heads/main") != candidate {
			t.Fatal("main did not fast-forward")
		}
		got, err := repo.Read("from-candidate", candidate)
		if err != nil || asInt(got.Value.(map[string]any)["v"]) != 1 {
			t.Fatalf("%#v %v", got, err)
		}

		diverged := create(t, "kr://conformance/merge-diverge")
		base := MustHead(t, diverged, "refs/heads/main")
		if err := diverged.CreateRef("refs/heads/candidate", base); err != nil {
			t.Fatal(err)
		}
		side, err := diverged.ApplyCommit(repository.CommitChangeSet{
			TargetRepository: diverged.ID(), TargetRef: "refs/heads/candidate",
			BaseCommit: base, ExpectedTargetCommit: base,
			Operations: PutEntity("candidate", 1, ""),
		})
		if err != nil {
			t.Fatal(err)
		}
		main, err := diverged.ApplyCommit(repository.CommitChangeSet{
			TargetRepository: diverged.ID(), TargetRef: "refs/heads/main",
			BaseCommit: base, ExpectedTargetCommit: base,
			Operations: PutEntity("main", 2, ""),
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = diverged.Merge("refs/heads/main", side, main)
		ExpectCode(t, err, kernel.ErrNonFastForward)

		stale := create(t, "kr://conformance/merge-stale")
		staleBase := MustHead(t, stale, "refs/heads/main")
		if err := stale.CreateRef("refs/heads/candidate", staleBase); err != nil {
			t.Fatal(err)
		}
		staleSide, err := stale.ApplyCommit(repository.CommitChangeSet{
			TargetRepository: stale.ID(), TargetRef: "refs/heads/candidate",
			BaseCommit: staleBase, ExpectedTargetCommit: staleBase,
			Operations: PutEntity("candidate", 1, ""),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := stale.ApplyCommit(repository.CommitChangeSet{
			TargetRepository: stale.ID(), TargetRef: "refs/heads/main",
			BaseCommit: staleBase, ExpectedTargetCommit: staleBase,
			Operations: PutEntity("main", 2, ""),
		}); err != nil {
			t.Fatal(err)
		}
		_, err = stale.Merge("refs/heads/main", staleSide, staleBase)
		ExpectCode(t, err, kernel.ErrNonFastForward)
	})

	t.Run("createRef rejects duplicates and unknown commits", func(t *testing.T) {
		repo := create(t, "kr://conformance/create-ref")
		root := MustHead(t, repo, "refs/heads/main")
		if err := repo.CreateRef("refs/heads/feature", root); err != nil {
			t.Fatal(err)
		}
		if got, ok := repo.GetRef("refs/heads/feature"); !ok || got != root {
			t.Fatalf("feature %s %v", got, ok)
		}
		err := repo.CreateRef("refs/heads/feature", root)
		ExpectCode(t, err, kernel.ErrPreconditionFailed)
		err = repo.CreateRef("refs/heads/other", "missing-version")
		ExpectCode(t, err, kernel.ErrVersionUnresolved)
	})

	t.Run("archive rejects later writes", func(t *testing.T) {
		repo := create(t, "kr://conformance/archive")
		root := MustHead(t, repo, "refs/heads/main")
		if err := repo.Archive(); err != nil {
			t.Fatal(err)
		}
		if !repo.Archived() {
			t.Fatal("archive did not stick")
		}
		_, err := repo.ApplyCommit(CommitChange(repo.ID(), root, "after", 1, ""))
		ExpectCode(t, err, kernel.ErrRepositoryArchived)
		err = repo.CreateRef("refs/heads/later", root)
		ExpectCode(t, err, kernel.ErrRepositoryArchived)
		_, err = repo.Merge("refs/heads/main", root, root)
		ExpectCode(t, err, kernel.ErrRepositoryArchived)
	})
}

func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return -1
	}
}
