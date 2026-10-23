package datasets

import (
	"context"
	"errors"
	"testing"

	"kc/kernel"
	"kc/snapshot/dataset"
)

func TestAcceptedReleasePassBuildsCurrentUnreadyReleaseOfEachDataset(t *testing.T) {
	releases := newFakeAcceptedReleases(
		acceptedRelease("alpha", 1, "kr://acme/one"),
		acceptedRelease("beta", 2, "kr://acme/two"),
		acceptedRelease("gamma", 1, "kr://acme/three"),
		acceptedRelease("delta", 2, "kr://acme/four"),
		acceptedRelease("delta", 3, "kr://acme/four"),
	)
	retired := releases.current["gamma"]
	retired.Retired = true
	releases.current["gamma"] = retired
	build := &recordingBuild{}
	demand := AcceptedReleaseDemand{Releases: releases, Build: build.build,
		Ready: func(_ context.Context, setID string, revision int) (bool, error) {
			releases.probes = append(releases.probes, dataset.PublicationLookup{SetID: setID, Revision: revision})
			return setID == "beta", nil
		}}

	if err := demand.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	// gamma is retired and beta is already ready, so neither is a target. delta
	// is listed at its current revision only: v2 is history, not a build.
	if want := []dataset.PublicationLookup{{SetID: "alpha", Revision: 1}, {SetID: "delta", Revision: 3}}; !sameReleases(build.built, want...) {
		t.Fatalf("built %#v, want %#v", build.built, want)
	}
	// Listing probes once per current release and the build re-probes its own
	// target, because readiness is a fact that can change in between.
	if want := []dataset.PublicationLookup{
		{SetID: "alpha", Revision: 1}, {SetID: "beta", Revision: 2}, {SetID: "delta", Revision: 3},
		{SetID: "alpha", Revision: 1}, {SetID: "delta", Revision: 3},
	}; !sameReleaseSet(releases.probes, want...) {
		t.Fatalf("readiness probed %#v, want %#v", releases.probes, want)
	}
}

func TestAcceptedReleasePassBuildsOnlyReleasesThisInstanceOwns(t *testing.T) {
	releases := newFakeAcceptedReleases(
		acceptedRelease("local", 1, "kr://acme/local"),
		acceptedRelease("straddling", 1, "kr://acme/local", "kr://acme/foreign"),
		acceptedRelease("foreign", 1, "kr://acme/foreign"),
	)
	build := &recordingBuild{}
	demand := AcceptedReleaseDemand{Releases: releases, Build: build.build,
		Ready: func(_ context.Context, setID string, revision int) (bool, error) {
			releases.probes = append(releases.probes, dataset.PublicationLookup{SetID: setID, Revision: revision})
			return false, nil
		},
		// A release belongs to the instance that admits all of its sources.
		Owns: func(def dataset.Dataset) bool {
			for _, source := range def.Sources {
				if source.Repository != "kr://acme/local" {
					return false
				}
			}
			return len(def.Sources) > 0
		}}

	if err := demand.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	built := map[string]bool{}
	for _, key := range build.built {
		built[key.SetID] = true
	}
	if len(built) != 1 || !built["local"] {
		t.Fatalf("built unowned releases: %#v", build.built)
	}
	// An unowned release must not even be probed: another instance is the one
	// that will build it, and probing would suggest this one is working on it.
	// Listing then building the one owned release probes it twice; no other
	// release is probed at all.
	if want := []dataset.PublicationLookup{{SetID: "local", Revision: 1}, {SetID: "local", Revision: 1}}; !sameReleaseSet(releases.probes, want...) {
		t.Fatalf("probed unowned releases: %#v", releases.probes)
	}
}

func TestAcceptedReleasePassKeepsGoingAndLeavesFailedReleaseBuildable(t *testing.T) {
	releases := newFakeAcceptedReleases(
		acceptedRelease("alpha", 1, "kr://acme/one"),
		acceptedRelease("beta", 1, "kr://acme/two"),
	)
	failure := errors.New("corpus storage unavailable")
	build := &recordingBuild{fail: map[dataset.PublicationLookup]error{
		{SetID: "alpha", Revision: 1}: failure,
	}}
	demand := AcceptedReleaseDemand{Releases: releases, Build: build.build,
		Ready: func(context.Context, string, int) (bool, error) { return false, nil }}

	err := demand.CatchUp(context.Background())
	if !errors.Is(err, failure) {
		t.Fatalf("failed build was not reported: %v", err)
	}
	if want := []dataset.PublicationLookup{{SetID: "alpha", Revision: 1}, {SetID: "beta", Revision: 1}}; !sameReleases(build.built, want...) {
		t.Fatalf("one failure stopped the pass: built %#v", build.built)
	}
	// Acceptance is a metadata decision the build cannot undo, and a build that
	// failed is not a ready corpus, so the next pass must retry it.
	if current, _ := releases.Set("alpha"); current.Revision != 1 || current.Retired {
		t.Fatalf("failed build changed acceptance: %#v", current)
	}
	targets, err := demand.Builds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := []dataset.PublicationLookup{{SetID: "alpha", Revision: 1}, {SetID: "beta", Revision: 1}}; !sameReleases(targets, want...) {
		t.Fatalf("retry targets %#v, want %#v", targets, want)
	}
}

func TestAcceptedReleasePassSkipsReleaseSupersededOrRetiredAfterListing(t *testing.T) {
	for _, phase := range []string{"superseded", "retired", "gone"} {
		t.Run(phase, func(t *testing.T) {
			releases := newFakeAcceptedReleases(acceptedRelease("alpha", 1, "kr://acme/one"))
			build := &recordingBuild{}
			demand := AcceptedReleaseDemand{Releases: releases, Build: build.build,
				Ready: func(context.Context, string, int) (bool, error) { return false, nil }}
			target := dataset.PublicationLookup{SetID: "alpha", Revision: 1}

			switch phase {
			case "superseded":
				// A newer release was accepted between listing and building, so
				// v1 is history and only v2 is still worth a corpus.
				releases.accept(acceptedRelease("alpha", 2, "kr://acme/one"))
			case "retired":
				releases.current["alpha"] = dataset.Dataset{SetID: "alpha", Revision: 1, Retired: true}
			case "gone":
				// The version itself no longer resolves.
				releases.versionErr = kernel.Fail(kernel.ErrVersionUnresolved, "no such version")
			}

			if err := demand.build(context.Background(), target); err != nil {
				t.Fatalf("%s release became a pass failure: %v", phase, err)
			}
			if len(build.built) != 0 {
				t.Fatalf("%s release was still built: %#v", phase, build.built)
			}
		})
	}
}

func TestAcceptedReleasePassReportsReadinessFailureAndHonoursCancellation(t *testing.T) {
	releases := newFakeAcceptedReleases(acceptedRelease("alpha", 1, "kr://acme/one"))
	build := &recordingBuild{}

	// Without a build the pass is simply absent, not an error.
	if err := (AcceptedReleaseDemand{Releases: releases}).CatchUp(context.Background()); err != nil {
		t.Fatalf("absent build pass errored: %v", err)
	}

	readinessFailure := errors.New("readiness authority unavailable")
	demand := AcceptedReleaseDemand{Releases: releases, Build: build.build,
		Ready: func(context.Context, string, int) (bool, error) { return false, readinessFailure }}
	if err := demand.CatchUp(context.Background()); !errors.Is(err, readinessFailure) {
		t.Fatalf("readiness failure was hidden: %v", err)
	}

	demand.Ready = func(context.Context, string, int) (bool, error) { return false, nil }
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := demand.CatchUp(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was hidden: %v", err)
	}
	if len(build.built) != 0 {
		t.Fatalf("cancelled pass still built: %#v", build.built)
	}
}
