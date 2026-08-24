package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"kc/validation/scenarios"
)

type stepResult struct {
	ID         string        `json:"id"`
	Title      string        `json:"title"`
	Roles      []string      `json:"roles"`
	StartedAt  time.Time     `json:"startedAt"`
	Duration   time.Duration `json:"duration"`
	Outcome    string        `json:"outcome"`
	Error      string        `json:"error,omitempty"`
	Assertions []string      `json:"assertions"`
}

type runReport struct {
	LibraryVersion  int          `json:"libraryVersion"`
	Fixture         string       `json:"fixture"`
	Target          string       `json:"target"`
	Goal            string       `json:"goal,omitempty"`
	ExpectedOutcome string       `json:"expectedOutcome,omitempty"`
	StartedAt       time.Time    `json:"startedAt"`
	FinishedAt      time.Time    `json:"finishedAt"`
	Outcome         string       `json:"outcome"`
	Steps           []stepResult `json:"steps"`
}

func main() {
	os.Exit(run())
}

func run() int {
	flags := flag.NewFlagSet("scenario-runner", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	list := flags.Bool("list", false, "list the unified scenario library")
	listActions := flags.Bool("list-actions", false, "include executable implementation actions")
	planOnly := flags.Bool("plan", false, "print the expanded action plan without running it")
	rootFlag := flags.String("root", "", "scene worktree root (auto-detected by default)")
	reportFlag := flags.String("report", "", "JSON report path relative to the scene root")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return 2
	}

	library, err := scenarios.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *list {
		printLibrary(library, *listActions)
		return 0
	}

	targetName := "all"
	if flags.NArg() > 0 {
		targetName = flags.Arg(0)
	}
	if flags.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "usage: scenario-runner [--list] [--plan] [scenario]")
		return 2
	}
	target, ok := library.Resolve(targetName)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown scenario %q; use --list to inspect the library\n", targetName)
		return 2
	}
	plan, err := library.Plan(target.ID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *planOnly {
		printPlan(target, plan)
		return 0
	}

	root := *rootFlag
	if root == "" {
		root, err = findSceneRoot()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	} else {
		root, err = filepath.Abs(root)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	if err := preflight(plan); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	reportPath := *reportFlag
	if reportPath == "" {
		reportPath = filepath.Join(".data", "datawarehouse", "scenarios", target.ID+".json")
	}
	if !filepath.IsAbs(reportPath) {
		reportPath = filepath.Join(root, reportPath)
	}

	report := runReport{
		LibraryVersion:  library.Version,
		Fixture:         library.Fixture.ID,
		Target:          target.ID,
		Goal:            target.Goal,
		ExpectedOutcome: target.Outcome,
		StartedAt:       time.Now(),
		Outcome:         "PASSED",
	}
	fmt.Printf("scenario target: %s (%s)\n", target.ID, target.Title)
	if target.Goal != "" {
		fmt.Printf("user goal: %s\n", target.Goal)
	}
	fmt.Printf("fixture: %s\n", library.Fixture.ID)
	fmt.Printf("action plan: %d step(s)\n", len(plan))

	for i, scenario := range plan {
		step := stepResult{
			ID: scenario.ID, Title: scenario.Title, Roles: scenario.Roles,
			StartedAt: time.Now(), Outcome: "PASSED", Assertions: scenario.Assertions,
		}
		fmt.Printf("\n[%d/%d] %s\n", i+1, len(plan), scenario.ID)
		fmt.Printf("roles: %s\n", strings.Join(scenario.Roles, ", "))
		cmd := exec.Command(scenario.Command[0], scenario.Command[1:]...)
		cmd.Dir = root
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		cmd.Env = scenarioEnvironment(scenario, plan[i+1:], root)
		err := cmd.Run()
		step.Duration = time.Since(step.StartedAt)
		if err != nil {
			step.Outcome = "FAILED"
			step.Error = err.Error()
			report.Outcome = "FAILED"
		}
		report.Steps = append(report.Steps, step)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s FAILED after %s: %v\n", scenario.ID, step.Duration.Round(time.Millisecond), err)
			break
		}
		fmt.Printf("%s PASSED in %s\n", scenario.ID, step.Duration.Round(time.Millisecond))
	}
	report.FinishedAt = time.Now()
	if err := writeReport(reportPath, report); err != nil {
		fmt.Fprintf(os.Stderr, "write scenario report: %v\n", err)
		return 1
	}
	fmt.Printf("\n%s: %s\nreport: %s\n", target.ID, report.Outcome, reportPath)
	if report.Outcome != "PASSED" {
		return 1
	}
	return 0
}

func printLibrary(library *scenarios.Library, includeActions bool) {
	fmt.Printf("fixture: %s — %s\n\n", library.Fixture.ID, library.Fixture.Description)
	for _, scenario := range library.Sorted() {
		if scenario.Kind == "action" && !includeActions {
			continue
		}
		aliases := ""
		if len(scenario.Aliases) != 0 {
			aliases = " [" + strings.Join(scenario.Aliases, ", ") + "]"
		}
		fmt.Printf("%-36s %-4s%s\n  %s\n  roles: %s\n", scenario.ID, scenario.Kind, aliases, scenario.Title, strings.Join(scenario.Roles, ", "))
		if scenario.Goal != "" {
			fmt.Printf("  goal: %s\n  outcome: %s\n", scenario.Goal, scenario.Outcome)
		}
	}
}

func printPlan(target scenarios.Scenario, plan []scenarios.Scenario) {
	fmt.Printf("%s — %s\n", target.ID, target.Title)
	if target.Goal != "" {
		fmt.Printf("goal: %s\noutcome: %s\n", target.Goal, target.Outcome)
	}
	for i, scenario := range plan {
		fmt.Printf("%d. %s (%s)\n", i+1, scenario.ID, strings.Join(scenario.Roles, ", "))
	}
}

func preflight(plan []scenarios.Scenario) error {
	checked := map[string]bool{}
	for _, scenario := range plan {
		for _, required := range append([]string{scenario.Command[0]}, scenario.Requires...) {
			if checked[required] {
				continue
			}
			checked[required] = true
			if _, err := exec.LookPath(required); err != nil {
				return fmt.Errorf("scenario %q requires command %q: %w", scenario.ID, required, err)
			}
		}
	}
	return nil
}

func scenarioEnvironment(scenario scenarios.Scenario, remaining []scenarios.Scenario, root string) []string {
	env := append([]string{}, os.Environ()...)
	env = setEnv(env, "KC_DW_DEPENDENCIES_READY", "1")
	keepMySQL := false
	if strings.HasPrefix(scenario.ID, "producer.mysql.") {
		for _, next := range remaining {
			if strings.HasPrefix(next.ID, "producer.mysql.") {
				keepMySQL = true
				break
			}
		}
	}
	if keepMySQL {
		env = setEnv(env, "KC_DW_KEEP_MYSQL", "1")
	} else if os.Getenv("KC_DW_KEEP_MYSQL") == "" {
		env = setEnv(env, "KC_DW_KEEP_MYSQL", "0")
	}
	for key, value := range scenario.Env {
		value = strings.ReplaceAll(value, "${SCENE_ROOT}", root)
		env = setEnv(env, key, value)
	}
	return env
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i := range env {
		if strings.HasPrefix(env[i], prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func findSceneRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if regular(filepath.Join(dir, "go.mod")) && regular(filepath.Join(dir, "validation", "scenarios", "library.yaml")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("cannot find scene root from %s", dir)
		}
		dir = parent
	}
}

func regular(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func writeReport(path string, report runReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}
