// Command coupling computes structural coupling metrics for this repository's
// top-level units from the go list import graph. It is observation tooling for
// docs/reviewed/quality-loop.md (观层), not a product surface: it prints a report and
// compares it against scripts/coupling/baseline.json. Nothing fails on drift;
// upgrading a coupling signal to a hard gate requires its own TASK.md item.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type listPackage struct {
	ImportPath string
	Standard   bool
	Module     *struct {
		Path string
	}
	Dir          string
	Imports      []string
	GoFiles      []string
	CgoFiles     []string
	TestGoFiles  []string
	XTestGoFiles []string
}

type unitStat struct {
	Unit            string  `json:"unit"`
	Ce              int     `json:"ce"`
	Ca              int     `json:"ca"`
	Instability     float64 `json:"instability"`
	ProdLines       int     `json:"prodLines"`
	TestLines       int     `json:"testLines"`
	TransitiveReach int     `json:"transitiveReach"`
}

type baseline struct {
	GeneratedAt     string     `json:"generatedAt"`
	PropagationCost float64    `json:"propagationCost"`
	Units           []unitStat `json:"units"`
}

func main() {
	writeBase := flag.Bool("write-baseline", false, "write the computed snapshot to the baseline file and exit")
	baselineFlag := flag.String("baseline", "", "baseline JSON path (default: scripts/coupling/baseline.json under the repo root)")
	flag.Parse()

	root, err := repoRoot()
	if err != nil {
		fatal(err)
	}
	basePath := *baselineFlag
	if basePath == "" {
		basePath = filepath.Join(root, "scripts", "coupling", "baseline.json")
	}

	unitImports, prodLines, testLines, order, err := collect(root)
	if err != nil {
		fatal(err)
	}
	stats := compute(unitImports, prodLines, testLines, order)
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Instability != stats[j].Instability {
			return stats[i].Instability > stats[j].Instability
		}
		return stats[i].Unit < stats[j].Unit
	})
	propagation := propagationCost(stats)

	fmt.Printf("%-22s %3s %4s %6s %9s %12s\n", "unit", "Ce", "Ca", "I", "prodLOC", "trans-reach")
	for _, s := range stats {
		fmt.Printf("%-22s %3d %4d %6.2f %9d %12d\n", s.Unit, s.Ce, s.Ca, s.Instability, s.ProdLines, s.TransitiveReach)
	}

	if *writeBase {
		doc := baseline{GeneratedAt: time.Now().UTC().Format(time.RFC3339), PropagationCost: propagation, Units: stats}
		raw, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			fatal(err)
		}
		if err := os.WriteFile(basePath, append(raw, '\n'), 0o644); err != nil {
			fatal(err)
		}
		fmt.Printf("baseline written: %s\n", basePath)
		return
	}

	fmt.Printf("coupling: units=%d propagation=%.2f%%", len(stats), propagation*100)
	base, err := readBaseline(basePath)
	switch {
	case err != nil:
		fmt.Printf(" (no baseline: %v)\n", err)
	default:
		fmt.Printf(" (baseline %.2f%%, delta %+.2fpp)\n", base.PropagationCost*100, (propagation-base.PropagationCost)*100)
		if moved := drift(base, stats); len(moved) > 0 {
			fmt.Printf("instability drift >= 0.05: %s\n", strings.Join(moved, ", "))
		} else {
			fmt.Printf("instability drift >= 0.05: none\n")
		}
	}
}

// collect runs go list -json once, groups module packages into top-level
// units, aggregates production/test line counts, and resolves intra-module
// import edges after the whole stream has been decoded.
func collect(root string) (map[string]map[string]bool, map[string]int, map[string]int, []string, error) {
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		detail := ""
		if exit, ok := err.(*exec.ExitError); ok {
			detail = strings.TrimSpace(string(exit.Stderr))
		}
		return nil, nil, nil, nil, fmt.Errorf("go list: %w %s", err, detail)
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	module := ""
	pathUnit := map[string]string{}
	importsByPath := map[string][]string{}
	prodLines := map[string]int{}
	testLines := map[string]int{}
	for dec.More() {
		var p listPackage
		if err := dec.Decode(&p); err != nil {
			return nil, nil, nil, nil, err
		}
		if p.Standard || p.ImportPath == "" {
			continue
		}
		if module == "" && p.Module != nil {
			module = p.Module.Path
		}
		if len(p.GoFiles)+len(p.CgoFiles)+len(p.TestGoFiles)+len(p.XTestGoFiles) == 0 {
			continue
		}
		u, ok := unit(module, p.ImportPath)
		if !ok {
			continue
		}
		pathUnit[p.ImportPath] = u
		importsByPath[p.ImportPath] = p.Imports
		prodLines[u] += countLines(p.Dir, append(append([]string{}, p.GoFiles...), p.CgoFiles...))
		testLines[u] += countLines(p.Dir, append(append([]string{}, p.TestGoFiles...), p.XTestGoFiles...))
	}
	if module == "" {
		return nil, nil, nil, nil, fmt.Errorf("go list output has no module path")
	}
	unitImports := map[string]map[string]bool{}
	for path, u := range pathUnit {
		deps := unitImports[u]
		if deps == nil {
			deps = map[string]bool{}
			unitImports[u] = deps
		}
		for _, imp := range importsByPath[path] {
			if v, ok := pathUnit[imp]; ok && v != u {
				deps[v] = true
			}
		}
	}
	order := make([]string, 0, len(unitImports))
	for u := range unitImports {
		order = append(order, u)
	}
	sort.Strings(order)
	return unitImports, prodLines, testLines, order, nil
}

func unit(module, importPath string) (string, bool) {
	rest, ok := strings.CutPrefix(importPath, module+"/")
	if !ok {
		return "", false
	}
	return strings.SplitN(rest, "/", 2)[0], true
}

func compute(unitImports map[string]map[string]bool, prodLines, testLines map[string]int, order []string) []unitStat {
	fanIn := map[string]int{}
	for _, u := range order {
		for v := range unitImports[u] {
			fanIn[v]++
		}
	}
	stats := make([]unitStat, 0, len(order))
	for _, u := range order {
		s := unitStat{Unit: u, Ce: len(unitImports[u]), Ca: fanIn[u]}
		if s.Ce+s.Ca > 0 {
			s.Instability = float64(s.Ce) / float64(s.Ce+s.Ca)
		}
		s.ProdLines = prodLines[u]
		s.TestLines = testLines[u]
		s.TransitiveReach = len(reach(u, unitImports, map[string]bool{}))
		stats = append(stats, s)
	}
	return stats
}

func reach(u string, deps map[string]map[string]bool, seen map[string]bool) map[string]bool {
	for v := range deps[u] {
		if seen[v] {
			continue
		}
		seen[v] = true
		reach(v, deps, seen)
	}
	return seen
}

func propagationCost(stats []unitStat) float64 {
	if len(stats) <= 1 {
		return 0
	}
	total := 0
	for _, s := range stats {
		total += s.TransitiveReach
	}
	return float64(total) / float64(len(stats)*(len(stats)-1))
}

func countLines(dir string, files []string) int {
	lines := 0
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			continue
		}
		lines += bytes.Count(raw, []byte{'\n'})
		if len(raw) > 0 && raw[len(raw)-1] != '\n' {
			lines++
		}
	}
	return lines
}

func readBaseline(path string) (*baseline, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var b baseline
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

func drift(base *baseline, stats []unitStat) []string {
	old := map[string]float64{}
	for _, u := range base.Units {
		old[u.Unit] = u.Instability
	}
	var moved []string
	for _, s := range stats {
		if prev, ok := old[s.Unit]; ok {
			d := s.Instability - prev
			if d < 0 {
				d = -d
			}
			if d >= 0.05 {
				moved = append(moved, fmt.Sprintf("%s %.2f->%.2f", s.Unit, prev, s.Instability))
			}
		}
	}
	sort.Strings(moved)
	return moved
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from %s", dir)
		}
		dir = parent
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "coupling: %v\n", err)
	os.Exit(1)
}
