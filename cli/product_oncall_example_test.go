package cli_test

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"kc/cli"
)

func TestProductBookOncallExampleMatchesDecision(t *testing.T) {
	world := readExampleWorld(t)
	d := openExampleDeployment(t, world)
	d.publish(t, world, true)
	reads := make(map[string]map[string]any)
	revision := ""
	for _, line := range productBookOncallCommands(t) {
		args, err := splitSceneArgs(line)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := cli.ParseArgs(args)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(line, "store-oncall") {
			selected, ok := parsed.Flags["revision"].(string)
			if !ok || selected == "" || (revision != "" && selected != revision) {
				t.Fatalf("product example must fix one revision for its show, list and reads: %q", line)
			}
			revision = selected
		}
		result := d.as(t, "agent:product-book-reader", args...)
		if result.Status != 0 {
			t.Fatalf("product example %q failed: %s", line, result.Stdout)
		}
		if parsed.Command == "read" {
			read := asMap(t, body(t, result))
			reads[fmt.Sprint(read["reference"])] = asMap(t, read["value"])
		}
	}
	runbook := reads["urn:kc:object:service/qinghe.order-service#/runbook"]
	if runbook["revision"] != "2" {
		t.Fatalf("product decision needs updated runbook v2, selected Dataset revision %s returned %#v", revision, runbook)
	}
	steps := fmt.Sprint(runbook["firstSteps"])
	for _, expected := range []string{"firstDayNotice", "800ms", "错误率低于 1%", "不要重启"} {
		if !strings.Contains(steps, expected) {
			t.Errorf("product decision lacks its published runbook condition %q: %s", expected, steps)
		}
	}
	campaign := reads["urn:kc:object:campaign/qinghe.autumn-2026"]
	definition := asMap(t, campaign["definition"])
	notice, _ := definition["firstDayNotice"].(string)
	if !strings.Contains(notice, "3 倍") || !strings.Contains(notice, "800ms") {
		t.Fatalf("product decision needs the first-day traffic and applicable threshold: %#v", campaign)
	}
}

func productBookOncallCommands(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "doc", "product.html"))
	if err != nil {
		t.Fatal(err)
	}
	var commands []string
	for _, block := range regexp.MustCompile(`(?is)<pre\b[^>]*>\s*<code\b[^>]*>(.*?)</code\s*>\s*</pre\s*>`).FindAllSubmatch(raw, -1) {
		text := html.UnescapeString(string(block[1]))
		if !strings.Contains(text, "store-oncall") {
			continue
		}
		for _, line := range strings.Split(text, "\n") {
			if line = strings.TrimSpace(line); strings.HasPrefix(line, "kc ") {
				commands = append(commands, line)
			}
		}
	}
	if len(commands) == 0 {
		t.Fatal("product book has no executable store-oncall example")
	}
	return commands
}
