// Command coupling computes structural coupling metrics for this repository's
// top-level units from the go list import graph. It is the 观层/闸层 tooling for
// docs/reviewed/quality-loop.md §7, not a product surface. Report mode is
// observation only. -strict is the hard ratchet used by make quality: it fails
// when the propagation cost exceeds the recorded baseline, when a unit's
// Instability drifts by 0.05 or more, or when a unit is missing from the
// baseline; new or tightened baselines require an explicit -write-baseline run
// and a TASK.md note.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
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
	Abstractness    float64 `json:"abstractness"`
	MainSeqDistance float64 `json:"mainSeqDistance"`
	PublicPerKLoc   float64 `json:"publicPerKLoc"`
	Interfaces      int     `json:"interfaces"`
	TotalTypes      int     `json:"totalTypes"`
	ExportedDecls   int     `json:"exportedDecls"`
	ProdLines       int     `json:"prodLines"`
	TestLines       int     `json:"testLines"`
	TransitiveReach int     `json:"transitiveReach"`
}

type baseline struct {
	GeneratedAt     string     `json:"generatedAt"`
	Depth           int        `json:"depth"`
	PropagationCost float64    `json:"propagationCost"`
	Units           []unitStat `json:"units"`
}

type declStat struct {
	interfaces int
	totalTypes int
	exported   int
}

var (
	typeLine    = regexp.MustCompile(`^type\s+(\w+)`)
	exportedTop = regexp.MustCompile(`^func\s+\([^)]*\)\s+[A-Z]|^func\s+[A-Z]|^var\s+[A-Z]|^const\s+[A-Z]`)
)

func main() {
	writeBase := flag.Bool("write-baseline", false, "write the computed snapshot to the baseline file and exit")
	strict := flag.Bool("strict", false, "fail when propagation cost exceeds the baseline, a unit drifts by >=0.05 Instability, or a unit is missing from the baseline")
	baselineFlag := flag.String("baseline", "", "baseline JSON path (default: scripts/coupling/baseline.json under the repo root)")
	depth := flag.Int("depth", 1, "unit granularity: path segments after the module prefix (structural report only)")
	cochange := flag.Bool("cochange", false, "print normalized file/unit co-change rates from git history and exit")
	cochangeMin := flag.Int("cochange-min", 4, "minimum shared commits for a co-change pair to be reported")
	filesOf := flag.String("files", "", "print per-file LOC/churn and prefix clusters for one unit and exit")
	flag.Parse()

	root, err := repoRoot()
	if err != nil {
		fatal(err)
	}
	if *cochange {
		if err := runCochange(root, *cochangeMin); err != nil {
			fatal(err)
		}
		return
	}
	if *filesOf != "" {
		if err := runFiles(root, *filesOf); err != nil {
			fatal(err)
		}
		return
	}
	if *strict && *depth != 1 {
		fatal(fmt.Errorf("-strict requires -depth=1: baselines are recorded at unit granularity"))
	}
	basePath := *baselineFlag
	if basePath == "" {
		basePath = filepath.Join(root, "scripts", "coupling", "baseline.json")
	}

	unitImports, prodLines, testLines, decls, order, err := collect(root, *depth)
	if err != nil {
		fatal(err)
	}
	stats := compute(unitImports, prodLines, testLines, decls, order)
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Instability != stats[j].Instability {
			return stats[i].Instability > stats[j].Instability
		}
		return stats[i].Unit < stats[j].Unit
	})
	propagation := propagationCost(stats)

	fmt.Printf("%-20s %3s %4s %5s %5s %5s %8s %9s %12s\n", "unit", "Ce", "Ca", "I", "A", "D", "exp/kL", "prodLOC", "trans-reach")
	for _, s := range stats {
		fmt.Printf("%-20s %3d %4d %5.2f %5.2f %5.2f %8.1f %9d %12d\n",
			s.Unit, s.Ce, s.Ca, s.Instability, s.Abstractness, s.MainSeqDistance, s.PublicPerKLoc, s.ProdLines, s.TransitiveReach)
	}

	if *writeBase {
		doc := baseline{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Depth: *depth, PropagationCost: propagation, Units: stats}
		raw, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			fatal(err)
		}
		if err := os.WriteFile(basePath, append(raw, '\n'), 0o644); err != nil {
			fatal(err)
		}
		fmt.Printf("baseline written: %s (tightening must be noted in TASK.md)\n", basePath)
		return
	}

	base, err := readBaseline(basePath)
	if err != nil {
		if *strict {
			fatal(fmt.Errorf("-strict requires a readable baseline: %v", err))
		}
		fmt.Printf("coupling: units=%d propagation=%.2f%% (no baseline: %v)\n", len(stats), propagation*100, err)
		return
	}

	fmt.Printf("coupling: units=%d propagation=%.2f%% (baseline %.2f%%, delta %+.2fpp)\n",
		len(stats), propagation*100, base.PropagationCost*100, (propagation-base.PropagationCost)*100)
	if *strict {
		if problems := strictCheck(base, stats, propagation); len(problems) > 0 {
			for _, p := range problems {
				fmt.Fprintf(os.Stderr, "coupling ratchet: %s\n", p)
			}
			os.Exit(1)
		}
		fmt.Printf("coupling ratchet: held (propagation <= baseline, instability drift < 0.05, no unknown units)\n")
		return
	}
	if moved := drift(base, stats); len(moved) > 0 {
		fmt.Printf("instability drift >= 0.05: %s\n", strings.Join(moved, ", "))
	} else {
		fmt.Printf("instability drift >= 0.05: none\n")
	}
}

// collect runs go list -json once, groups module packages into units of the
// requested path depth, aggregates production/test line counts and declaration
// proxies, and resolves intra-module import edges after the whole stream has
// been decoded.
func collect(root string, depth int) (map[string]map[string]bool, map[string]int, map[string]int, map[string]*declStat, []string, error) {
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		detail := ""
		if exit, ok := err.(*exec.ExitError); ok {
			detail = strings.TrimSpace(string(exit.Stderr))
		}
		return nil, nil, nil, nil, nil, fmt.Errorf("go list: %w %s", err, detail)
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	module := ""
	pathUnit := map[string]string{}
	importsByPath := map[string][]string{}
	prodLines := map[string]int{}
	testLines := map[string]int{}
	decls := map[string]*declStat{}
	for dec.More() {
		var p listPackage
		if err := dec.Decode(&p); err != nil {
			return nil, nil, nil, nil, nil, err
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
		u, ok := unitAt(module, p.ImportPath, depth)
		if !ok {
			continue
		}
		pathUnit[p.ImportPath] = u
		importsByPath[p.ImportPath] = p.Imports
		prodLines[u] += countLines(p.Dir, append(append([]string{}, p.GoFiles...), p.CgoFiles...))
		testLines[u] += countLines(p.Dir, append(append([]string{}, p.TestGoFiles...), p.XTestGoFiles...))
		if decls[u] == nil {
			decls[u] = &declStat{}
		}
		for _, f := range p.GoFiles {
			scanDecls(filepath.Join(p.Dir, f), decls[u])
		}
	}
	if module == "" {
		return nil, nil, nil, nil, nil, fmt.Errorf("go list output has no module path")
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
	return unitImports, prodLines, testLines, decls, order, nil
}

// scanDecls counts declaration proxies from source text: interface vs total
// top-level type declarations and capitalized top-level declarations. Block
// form var/const members are not counted; this is a declared proxy, not a
// go/types parse.
func scanDecls(path string, s *declStat) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimRight(line, "\r")
		if m := typeLine.FindStringSubmatch(line); m != nil {
			s.totalTypes++
			rest := strings.TrimSpace(line[len(m[0])-len(m[1]):])
			if strings.HasPrefix(rest, "interface") {
				s.interfaces++
			}
			if unicode.IsUpper(firstRune(m[1])) {
				s.exported++
			}
			continue
		}
		if exportedTop.MatchString(line) {
			s.exported++
		}
	}
}

func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}

func unitAt(module, importPath string, depth int) (string, bool) {
	rest, ok := strings.CutPrefix(importPath, module+"/")
	if !ok {
		return "", false
	}
	segs := strings.Split(rest, "/")
	if len(segs) < depth {
		depth = len(segs)
	}
	return strings.Join(segs[:depth], "/"), true
}

func compute(unitImports map[string]map[string]bool, prodLines, testLines map[string]int, decls map[string]*declStat, order []string) []unitStat {
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
		if d := decls[u]; d != nil {
			s.Interfaces = d.interfaces
			s.TotalTypes = d.totalTypes
			s.ExportedDecls = d.exported
			if d.totalTypes > 0 {
				s.Abstractness = float64(d.interfaces) / float64(d.totalTypes)
			}
		}
		s.MainSeqDistance = math.Abs(s.Abstractness + s.Instability - 1)
		if prodLines[u] > 0 {
			s.PublicPerKLoc = float64(s.ExportedDecls) / (float64(prodLines[u]) / 1000)
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

// strictCheck implements the ratchet: metrics may improve but must not get
// worse, and the unit set may only grow via an explicit baseline tightening.
func strictCheck(base *baseline, stats []unitStat, propagation float64) []string {
	var problems []string
	if propagation > base.PropagationCost+1e-9 {
		problems = append(problems, fmt.Sprintf("propagation cost %.2f%% exceeds baseline %.2f%%", propagation*100, base.PropagationCost*100))
	}
	old := map[string]float64{}
	for _, u := range base.Units {
		old[u.Unit] = u.Instability
	}
	for _, s := range stats {
		prev, ok := old[s.Unit]
		if !ok {
			problems = append(problems, fmt.Sprintf("unit %s is not in the baseline; tighten explicitly with -write-baseline", s.Unit))
			continue
		}
		if math.Abs(s.Instability-prev) >= 0.05 {
			problems = append(problems, fmt.Sprintf("instability drift %s %.2f -> %.2f (>= 0.05)", s.Unit, prev, s.Instability))
		}
	}
	return problems
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

// gitCommits returns per-commit non-test Go file sets from git history. The
// %x01 record separator keeps adjacent commits from merging even when a
// commit carries no files.
func gitCommits(root string) [][]string {
	cmd := exec.Command("git", "log", "--pretty=format:%x01", "--name-only")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var commits [][]string
	for _, rec := range strings.Split(string(out), "\x01") {
		var files []string
		for _, l := range strings.Split(rec, "\n") {
			l = strings.TrimSpace(l)
			if l == "" || !strings.HasSuffix(l, ".go") || strings.Contains(l, "_test.go") {
				continue
			}
			files = append(files, l)
		}
		if len(files) > 0 {
			commits = append(commits, files)
		}
	}
	return commits
}

func fileChurn(commits [][]string) map[string]int {
	churn := map[string]int{}
	for _, files := range commits {
		for _, f := range files {
			churn[f]++
		}
	}
	return churn
}

// runCochange prints co-change pairs with a normalization guard: raw
// co-occurrence is biased toward high-churn files, so pairs are ranked by
// co / min(churn) and only pairs with enough support are shown.
func runCochange(root string, minSupport int) error {
	commits := gitCommits(root)
	if commits == nil {
		return fmt.Errorf("git history unavailable")
	}
	churn := fileChurn(commits)
	filePairs := map[[2]string]int{}
	unitPairs := map[[2]string]int{}
	unitChurn := map[string]int{}
	for _, files := range commits {
		units := map[string]bool{}
		for _, f := range files {
			units[strings.SplitN(f, "/", 2)[0]] = true
		}
		// Unit churn counts commits, not file touches: co-change rates compare
		// commit support on both sides.
		for u := range units {
			unitChurn[u]++
		}
		if len(files) < 2 || len(files) > 60 {
			continue
		}
		fs := append([]string{}, files...)
		sort.Strings(fs)
		for i := 0; i < len(fs); i++ {
			for j := i + 1; j < len(fs); j++ {
				filePairs[[2]string{fs[i], fs[j]}]++
			}
		}
		us := make([]string, 0, len(units))
		for u := range units {
			us = append(us, u)
		}
		sort.Strings(us)
		for i := 0; i < len(us); i++ {
			for j := i + 1; j < len(us); j++ {
				unitPairs[[2]string{us[i], us[j]}]++
			}
		}
	}
	type row struct {
		rate   float64
		co     int
		a, b   string
		na, nb int
	}
	var frows []row
	for p, co := range filePairs {
		if co < minSupport || churn[p[0]] < minSupport || churn[p[1]] < minSupport {
			continue
		}
		d := min(churn[p[0]], churn[p[1]])
		frows = append(frows, row{float64(co) / float64(d), co, p[0], p[1], churn[p[0]], churn[p[1]]})
	}
	sort.Slice(frows, func(i, j int) bool { return frows[i].rate > frows[j].rate })
	fmt.Printf("=== co-change file pairs (co >= %d, rate = co/min churn) ===\n", minSupport)
	for i, r := range frows {
		if i == 15 {
			fmt.Println("...")
			break
		}
		fmt.Printf("%5.0f%% %4dx (%d/%d)  %s  <->  %s\n", r.rate*100, r.co, r.na, r.nb, r.a, r.b)
	}
	if len(frows) == 0 {
		fmt.Println("(no pair met the support threshold)")
	}
	var urows []row
	for p, co := range unitPairs {
		if co < minSupport || unitChurn[p[0]] < minSupport || unitChurn[p[1]] < minSupport {
			continue
		}
		d := min(unitChurn[p[0]], unitChurn[p[1]])
		urows = append(urows, row{float64(co) / float64(d), co, p[0], p[1], unitChurn[p[0]], unitChurn[p[1]]})
	}
	sort.Slice(urows, func(i, j int) bool { return urows[i].rate > urows[j].rate })
	fmt.Printf("=== co-change unit pairs ===\n")
	for i, r := range urows {
		if i == 10 {
			fmt.Println("...")
			break
		}
		fmt.Printf("%5.0f%% %4dx (%d/%d)  %s  <->  %s\n", r.rate*100, r.co, r.na, r.nb, r.a, r.b)
	}
	return nil
}

// runFiles prints per-file LOC and churn for one unit plus prefix clusters:
// the seam-finding view for large single-package units like cli.
func runFiles(root, unitName string) error {
	cmd := exec.Command("go", "list", "-json", "./"+unitName+"/...")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("go list: %w", err)
	}
	loc := map[string]int{}
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var p listPackage
		if err := dec.Decode(&p); err != nil {
			return err
		}
		for _, f := range p.GoFiles {
			raw, err := os.ReadFile(filepath.Join(p.Dir, f))
			if err != nil {
				continue
			}
			rel, err := filepath.Rel(root, filepath.Join(p.Dir, f))
			if err != nil {
				continue
			}
			loc[filepath.ToSlash(rel)] = bytes.Count(raw, []byte{'\n'})
		}
	}
	commits := gitCommits(root)
	churn := fileChurn(commits)
	type frow struct {
		loc, churn int
		path       string
	}
	var rows []frow
	type cluster struct {
		files, loc, churn int
	}
	clusters := map[string]*cluster{}
	var names []string
	for path, l := range loc {
		ch := churn[path]
		rows = append(rows, frow{l, ch, path})
		base := filepath.Base(path)
		prefix := base
		if i := strings.Index(base, "_"); i > 0 {
			prefix = base[:i]
		}
		c := clusters[prefix]
		if c == nil {
			c = &cluster{}
			clusters[prefix] = c
			names = append(names, prefix)
		}
		c.files++
		c.loc += l
		c.churn += ch
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].churn != rows[j].churn {
			return rows[i].churn > rows[j].churn
		}
		if rows[i].loc != rows[j].loc {
			return rows[i].loc > rows[j].loc
		}
		return rows[i].path < rows[j].path
	})
	fmt.Printf("=== unit %s: %d prod files ===\n", unitName, len(rows))
	fmt.Printf("%6s %7s  file\n", "churn", "LOC")
	for i, r := range rows {
		if i == 30 {
			fmt.Println("...")
			break
		}
		fmt.Printf("%6d %7d  %s\n", r.churn, r.loc, r.path)
	}
	sort.Slice(names, func(i, j int) bool { return clusters[names[i]].churn > clusters[names[j]].churn })
	fmt.Printf("=== prefix clusters (by churn) ===\n")
	fmt.Printf("%6s %7s %8s  prefix\n", "files", "LOC", "churn")
	for _, n := range names {
		c := clusters[n]
		fmt.Printf("%6d %7d %8d  %s\n", c.files, c.loc, c.churn, n)
	}
	return nil
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
