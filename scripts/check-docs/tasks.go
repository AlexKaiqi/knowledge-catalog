package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The task board keeps one Markdown file per task. A task's state is the
// directory it sits in, so moving the file is the only way to change state and
// a task can never be in two states at once.
const (
	taskRoot   = "tasks"
	taskIndex  = "tasks/README.md"
	taskKeeper = ".gitkeep"
	// exploreDir holds open questions, not tasks. An exploration has no agreed
	// deliverable until it is answered, so it cannot sit in todo/ without
	// pretending to have acceptance criteria. It is not a fourth state: its
	// files never move between states, they are concluded in place and then
	// retired, while any confirmed issue becomes a task of its own.
	exploreDir = "explore"
)

var (
	taskStates = []string{"todo", "doing", "done"}
	// New IDs end in a two-digit sequence (see tasks/README.md); archived
	// historical IDs such as CLI-REFACTOR keep their original spelling.
	taskID = regexp.MustCompile(`^[A-Z][A-Z0-9]*(-[A-Z0-9]+)+$`)
)

// taskDocuments validates the board layout and returns every task file,
// relative to root. A checkout without tasks/ has no board and no task files.
func taskDocuments(root string) ([]string, error) {
	base := filepath.Join(root, taskRoot)
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return nil, nil
	}
	if err := validateTaskRoot(base); err != nil {
		return nil, err
	}
	seen := map[string]string{}
	var files []string
	for _, state := range taskStates {
		ids, err := stateTasks(root, state)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if previous, dup := seen[id]; dup {
				return nil, fmt.Errorf("%s/%s: task %s is also in %s; a task has exactly one state", taskRoot, state, id, previous)
			}
			seen[id] = state
			files = append(files, taskRoot+"/"+state+"/"+id+".md")
		}
	}
	explorations, err := exploreDocuments(root)
	if err != nil {
		return nil, err
	}
	for _, id := range explorations {
		if previous, dup := seen[id]; dup {
			return nil, fmt.Errorf("%s/%s: %s is also a task in %s; a confirmed question becomes a new task, it does not share the exploration's ID", taskRoot, exploreDir, id, previous)
		}
		seen[id] = exploreDir
		files = append(files, taskRoot+"/"+exploreDir+"/"+id+".md")
	}
	sort.Strings(files)
	return files, nil
}

// exploreDocuments returns the exploration IDs in tasks/explore/ after checking
// that every entry is a Markdown file named after its own title.
func exploreDocuments(root string) ([]string, error) {
	dir := filepath.Join(root, taskRoot, exploreDir)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, entry := range entries {
		rel := taskRoot + "/" + exploreDir + "/" + entry.Name()
		if entry.Name() == taskKeeper && !entry.IsDir() {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".md")
		if entry.IsDir() || id == entry.Name() || !taskID.MatchString(id) {
			return nil, fmt.Errorf("%s is not an exploration file; name it <ID>.md with an ID like PERM-GATE-01", rel)
		}
		if err := validateTaskTitle(root, rel, id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// validateTaskRoot allows only the board README and the state directories.
func validateTaskRoot(base string) error {
	entries, err := os.ReadDir(base)
	if err != nil {
		return err
	}
	allowed := map[string]bool{"README.md": false, exploreDir: true}
	for _, state := range taskStates {
		allowed[state] = true
	}
	hasIndex := false
	for _, entry := range entries {
		wantDir, ok := allowed[entry.Name()]
		if !ok || wantDir != entry.IsDir() {
			return fmt.Errorf("%s/%s is not allowed; tasks live in %s/{%s}/<ID>.md and open questions in %s/%s/<ID>.md", taskRoot, entry.Name(), taskRoot, strings.Join(taskStates, ","), taskRoot, exploreDir)
		}
		if entry.Name() == "README.md" {
			hasIndex = true
		}
	}
	if !hasIndex {
		return fmt.Errorf("%s is missing", taskIndex)
	}
	return nil
}

// stateTasks returns the task IDs in one state directory after checking that
// every file is a task whose title starts with its own ID.
func stateTasks(root, state string) ([]string, error) {
	dir := filepath.Join(root, taskRoot, state)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%s/%s is missing", taskRoot, state)
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, entry := range entries {
		rel := taskRoot + "/" + state + "/" + entry.Name()
		if entry.Name() == taskKeeper && !entry.IsDir() {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".md")
		if entry.IsDir() || id == entry.Name() || !taskID.MatchString(id) {
			return nil, fmt.Errorf("%s is not a task file; name it <ID>.md with an ID like PLUGIN-INTERPRETER-01", rel)
		}
		if err := validateTaskTitle(root, rel, id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func validateTaskTitle(root, rel, id string) error {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "# "+id || strings.HasPrefix(line, "# "+id+" ") {
			return nil
		}
		break
	}
	return fmt.Errorf("%s must start with the title \"# %s · …\"", rel, id)
}
