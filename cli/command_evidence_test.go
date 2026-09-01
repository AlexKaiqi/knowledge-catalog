package cli_test

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"kc/cli"
)

var (
	assertedSuccesses sync.Map
	assertedFailures  sync.Map
	commandEvidenceMu sync.Mutex
	commandEvidence   = map[string]*operationEvidenceState{}
	runSequence       atomic.Uint64
)

type operationEvidence struct {
	Calls            int                `json:"calls"`
	SuccessfulRuns   int                `json:"successfulRuns"`
	Successes        int                `json:"assertedSuccesses"`
	SuccessScenarios []string           `json:"successScenarios,omitempty"`
	Boundaries       int                `json:"assertedBoundaries"`
	ErrorCodes       map[string]int     `json:"errorCodes,omitempty"`
	Scenarios        []boundaryEvidence `json:"boundaryScenarios,omitempty"`
}

type boundaryEvidence struct {
	Scenario string `json:"scenario"`
	Code     string `json:"code"`
}

type operationEvidenceState struct {
	Calls            int
	SuccessfulRuns   int
	SuccessScenarios map[string]struct{}
	Boundaries       map[string]string
}

type commandCoverageRow struct {
	Command            string `json:"command"`
	Action             string `json:"action"`
	RequiredBoundaries int    `json:"requiredAssertedBoundaries"`
	operationEvidence
}

type commandCoverageReport struct {
	TotalCommands                      int                  `json:"totalCommands"`
	SuccessfulCommands                 int                  `json:"successfulCommands"`
	CommandsWithBoundaries             int                  `json:"commandsWithAssertedBoundaries"`
	CommandsMeetingBoundaryRequirement int                  `json:"commandsMeetingBoundaryRequirement"`
	Commands                           []commandCoverageRow `json:"commands"`
}

type kcRunResult struct {
	cli.RunResult
	operation string
	runID     uint64
}

func recordCommandRun(operation string, success bool) {
	if operation == "" {
		return
	}
	commandEvidenceMu.Lock()
	defer commandEvidenceMu.Unlock()
	evidence := commandEvidence[operation]
	if evidence == nil {
		evidence = newOperationEvidenceState()
		commandEvidence[operation] = evidence
	}
	evidence.Calls++
	if success {
		evidence.SuccessfulRuns++
	}
}

func newOperationEvidenceState() *operationEvidenceState {
	return &operationEvidenceState{SuccessScenarios: map[string]struct{}{}, Boundaries: map[string]string{}}
}

func recordAssertedSuccess(t *testing.T, result kcRunResult) {
	t.Helper()
	if result.operation == "" || result.Status != 0 {
		return
	}
	if _, loaded := assertedSuccesses.LoadOrStore(result.runID, true); loaded {
		return
	}
	commandEvidenceMu.Lock()
	defer commandEvidenceMu.Unlock()
	evidence := commandEvidence[result.operation]
	if evidence == nil {
		evidence = newOperationEvidenceState()
		commandEvidence[result.operation] = evidence
	}
	evidence.SuccessScenarios[assertionScenario(t)] = struct{}{}
}

func recordAssertedFailure(t *testing.T, result kcRunResult, code string) {
	t.Helper()
	if result.operation == "" || result.Status == 0 {
		return
	}
	if _, loaded := assertedFailures.LoadOrStore(result.runID, true); loaded {
		return
	}
	commandEvidenceMu.Lock()
	defer commandEvidenceMu.Unlock()
	evidence := commandEvidence[result.operation]
	if evidence == nil {
		evidence = newOperationEvidenceState()
		commandEvidence[result.operation] = evidence
	}
	scenario := assertionScenario(t)
	evidence.Boundaries[scenario] = code
}

func assertionScenario(t *testing.T) string {
	_, file, line, ok := runtime.Caller(3)
	if !ok {
		return t.Name()
	}
	return fmt.Sprintf("%s@%s:%d", t.Name(), filepath.Base(file), line)
}

func commandCoverageSnapshot() commandCoverageReport {
	commandEvidenceMu.Lock()
	defer commandEvidenceMu.Unlock()
	commands := cli.CLICommandsForTest()
	actions := cli.CLICommandActionsForTest()
	report := commandCoverageReport{TotalCommands: len(commands), Commands: make([]commandCoverageRow, 0, len(commands))}
	for _, command := range commands {
		action := actions[command]
		row := commandCoverageRow{
			Command:            command,
			Action:             action,
			RequiredBoundaries: requiredAssertedBoundaries(action),
			operationEvidence:  operationEvidence{ErrorCodes: map[string]int{}},
		}
		if evidence := commandEvidence[command]; evidence != nil {
			row.Calls = evidence.Calls
			row.SuccessfulRuns = evidence.SuccessfulRuns
			for scenario := range evidence.SuccessScenarios {
				row.SuccessScenarios = append(row.SuccessScenarios, scenario)
			}
			sort.Strings(row.SuccessScenarios)
			row.Successes = len(row.SuccessScenarios)
			for scenario, code := range evidence.Boundaries {
				row.ErrorCodes[code]++
				row.Scenarios = append(row.Scenarios, boundaryEvidence{Scenario: scenario, Code: code})
			}
			sort.Slice(row.Scenarios, func(i, j int) bool { return row.Scenarios[i].Scenario < row.Scenarios[j].Scenario })
			row.Boundaries = len(row.Scenarios)
		}
		if row.Successes > 0 {
			report.SuccessfulCommands++
		}
		if row.Boundaries > 0 {
			report.CommandsWithBoundaries++
		}
		if row.Boundaries >= row.RequiredBoundaries {
			report.CommandsMeetingBoundaryRequirement++
		}
		report.Commands = append(report.Commands, row)
	}
	return report
}

// State-changing commands need evidence from at least two independent failure
// scenarios. Read-only commands still need one meaningful protocol boundary.
// The policy is based on the semantic action so a new public alias cannot
// silently weaken its coverage requirement.
func requiredAssertedBoundaries(action string) int {
	if strings.HasSuffix(action, ".manage") ||
		strings.HasPrefix(action, "governance.") {
		return 2
	}
	switch action {
	case "local.init", "local.catalog.attach", "local.repository.attach", "local.workspace.overlay",
		"local.system.publish",
		"writer.commit", "writer.preview", "feedback.write", "local.grant.bootstrap":
		return 2
	default:
		return 1
	}
}
