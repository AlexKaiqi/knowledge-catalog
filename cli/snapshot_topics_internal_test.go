package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	snapshotcli "kc/snapshot/cli"
)

func TestSnapshotHelpTopicsReachPublicConceptAndGroupEntries(t *testing.T) {
	dir := snapshotTopicHelpEnvironment(t)
	for topic, text := range snapshotcli.HelpTopics() {
		t.Run(topic, func(t *testing.T) {
			byTopic := Run([]string{"help", topic})
			byFlag := Run([]string{topic, "--help", "--file", filepath.Join(dir, "missing-input"), "--out", filepath.Join(dir, "output")})
			if byTopic.Status != 0 || byFlag.Status != 0 || byTopic.Stdout != byFlag.Stdout {
				t.Fatalf("static topic is not available through both public help forms: topic=%#v flag=%#v", byTopic, byFlag)
			}
			if !strings.Contains(byTopic.Stdout, strings.TrimSpace(text)) {
				t.Fatalf("host dropped or rewrote module topic %s:\n%s", topic, byTopic.Stdout)
			}
			for _, command := range snapshotcli.Commands() {
				for _, group := range command.Groups {
					if group == topic && !snapshotTopicMenuContains(byTopic.Stdout, command.Path, command.Description) {
						t.Errorf("%s group lost declared module leaf %s or its description", topic, command.Path)
					}
				}
			}
		})
	}
}

func TestSnapshotHelpTopicNavigationIsReachableFromRoot(t *testing.T) {
	snapshotTopicHelpEnvironment(t)
	links := regexp.MustCompile(`kc help ([a-z][a-z-]*(?: +[a-z][a-z-]*)*)`)
	pending := []string{""}
	visited := map[string]bool{}
	for len(pending) != 0 {
		topic := pending[0]
		pending = pending[1:]
		if visited[topic] {
			continue
		}
		visited[topic] = true
		if len(visited) > 128 {
			t.Fatal("help navigation exceeded its finite declared topics")
		}
		result := Run(append([]string{"help"}, strings.Fields(topic)...))
		if result.Status != 0 {
			t.Errorf("help %s was linked but is not executable: %#v", topic, result)
			continue
		}
		for _, match := range links.FindAllStringSubmatch(result.Stdout, -1) {
			pending = append(pending, strings.TrimSpace(match[1]))
		}
	}
	for topic := range snapshotcli.HelpTopics() {
		if !visited[topic] {
			t.Errorf("declared Snapshot topic %s has no executable help path from the root index", topic)
		}
	}
}

func TestSnapshotHelpTopicsDoNotRegisterBusinessCommands(t *testing.T) {
	snapshotTopicHelpEnvironment(t)
	if got := len(CLICommandsForTest()); got != 55 {
		t.Fatalf("static teaching topics changed the selected 55-leaf business surface including plugin explanation: %d", got)
	}
	for topic := range snapshotcli.HelpTopics() {
		t.Run(topic, func(t *testing.T) {
			if CLICommandForTest(topic) {
				t.Fatalf("static teaching topic became a business leaf: %s", topic)
			}
			if _, action := CLICommandActionsForTest()[topic]; action {
				t.Fatalf("teaching topic gained a semantic action: %s", topic)
			}
			if result := Run(strings.Fields(topic)); result.Status == 0 {
				t.Fatalf("incomplete family or concept executed a default business action: %s %#v", topic, result)
			}
		})
	}
	for _, topic := range []string{"dataset", "files", "grant", "registry"} {
		result := Run([]string{topic})
		if result.Status != 1 || !strings.Contains(result.Stdout, strings.TrimSpace(snapshotcli.HelpTopics()[topic])) {
			t.Errorf("incomplete family %s must return its module-owned teaching and options: %#v", topic, result)
		}
	}
	for _, argv := range [][]string{{"help", "not-a-topic"}, {"not-a-topic", "--help"}} {
		result := Run(argv)
		if result.Status != 1 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
			t.Errorf("unknown topic must fail without business execution: argv=%q result=%#v", argv, result)
		}
		if !strings.Contains(result.Stdout, "run kc help to discover task, concept and command topics") {
			t.Errorf("unknown topic must guide users back to the current root help: argv=%q result=%#v", argv, result)
		}
	}
}

func snapshotTopicMenuContains(text, path, description string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "  kc "+path+" ") && strings.Contains(line, description) {
			return true
		}
	}
	return false
}

func snapshotTopicHelpEnvironment(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("KC_SERVER_URL", "http://snapshot-topics.invalid")
	t.Setenv("KC_AUTH_TOKEN", "help-must-not-use-authentication")
	t.Setenv("KC_AS", "help-reader")
	t.Setenv("KC_HOME", filepath.Join(dir, "home"))
	t.Setenv("KC_CONFIG_DIR", filepath.Join(dir, "session"))
	transport := &snapshotHelpRejectingRoundTripper{}
	originalTransport := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
		if calls := transport.calls.Load(); calls != 0 {
			t.Errorf("static help/navigation made %d HTTP requests", calls)
		}
		files, err := os.ReadDir(dir)
		if err != nil || len(files) != 0 {
			t.Errorf("static help/navigation created inputs, Home, session or outputs: %v %v", files, err)
		}
	})
	return dir
}
