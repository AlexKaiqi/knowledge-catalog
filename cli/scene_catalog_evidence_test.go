package cli_test

import "testing"

func TestSceneCatalogGoEvidenceUsesEveryMatchingGroup(t *testing.T) {
	const source = "binding.observation"
	notice := "TestProjectionControllerNoticePullsStateWithoutChangingSnapshot"
	state := "TestStateRefreshFindsDynamicValueWithoutChangingSnapshot"
	null := "TestObservedNullProvesMissingAndFailedRefreshKeepsPublishedRevision"
	audit := sceneCatalogProcess{Source: source, Surface: "go-test", Evidence: []string{"TestServerLiveAccessAuditRestoresOriginalReadAndRechecksSeparatePermission"}}
	legacy := sceneCatalogProcess{Source: source, Surface: "go-test", Evidence: []string{notice, state}}
	observedNull := sceneCatalogProcess{Source: source, Surface: "go-test", Evidence: []string{null}}
	for label, processes := range map[string][]sceneCatalogProcess{
		"audit-before-legacy": {audit, legacy, observedNull},
		"legacy-before-audit": {observedNull, legacy, audit},
	} {
		t.Run(label, func(t *testing.T) {
			catalog := sceneCatalogState{Processes: processes}
			for _, name := range []string{notice, state, null, audit.Evidence[0]} {
				if !hasGoEvidence(catalog, source, name) {
					t.Errorf("independent matching group was shadowed: %s", name)
				}
			}
		})
	}
	for label, processes := range map[string][]sceneCatalogProcess{
		"wrong-source": {{Source: "resource.handle", Surface: "go-test", Evidence: []string{notice}}},
		"feature":      {{Source: source, Surface: "feature", File: "notice.feature", Evidence: []string{notice}}},
		"missing":      {audit, {Source: source, Surface: "go-test", Evidence: []string{state, null}}},
	} {
		t.Run(label, func(t *testing.T) {
			if hasGoEvidence(sceneCatalogState{Processes: processes}, source, notice) {
				t.Fatal("unrelated, non-executable or missing evidence satisfied the required Go test")
			}
		})
	}
}
