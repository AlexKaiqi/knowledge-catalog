package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The task board keeps one Markdown file per task. A task's state is the
// directory it sits in, so moving the file is the only way to change state and
// a task can never be in two states at once.
//
// Tasks that belong to one initiative may sit in a lowercase group directory
// inside todo/ or doing/, with an optional README.md giving that line its order.
// A group is not a state: the enclosing directory still is. done/ stays flat
// because it is history, and a group there would suggest an order that no
// longer exists.
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
	// groupIndex is the optional index of one initiative directory.
	groupIndex = "README.md"
	// historyState is the state directory that must stay flat.
	historyState = "done"
)

var (
	taskStates = []string{"todo", "doing", "done"}
	// New IDs end in a two-digit sequence (see tasks/README.md); archived
	// historical IDs such as CLI-REFACTOR keep their original spelling.
	taskID = regexp.MustCompile(`^[A-Z][A-Z0-9]*(-[A-Z0-9]+)+$`)
	// groupName names an initiative directory. Lowercase so it can never be
	// read as a task ID, which always starts uppercase.
	groupName = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
)

// taskDocuments validates the board layout and returns every Markdown file it
// owns, relative to root: task files plus any initiative index. A checkout
// without tasks/ has no board and no task files.
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
		refs, err := stateFiles(root, state)
		if err != nil {
			return nil, err
		}
		for _, ref := range refs {
			if path.Base(ref) == groupIndex {
				continue
			}
			id := strings.TrimSuffix(path.Base(ref), ".md")
			if previous, dup := seen[id]; dup {
				return nil, fmt.Errorf("%s: task %s is also in %s; a task has exactly one state", ref, id, previous)
			}
			seen[id] = state
		}
		files = append(files, refs...)
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
			return fmt.Errorf("%s/%s is not allowed; tasks live in %s/{%s}/<ID>.md, an initiative's tasks in %s/{todo,doing}/<group>/<ID>.md, and open questions in %s/%s/<ID>.md", taskRoot, entry.Name(), taskRoot, strings.Join(taskStates, ","), taskRoot, taskRoot, exploreDir)
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

// stateFiles returns the Markdown files under one state directory, relative to
// root: the task files plus any initiative index. A task sits directly in the
// state directory or, except in done/, in one lowercase group directory.
func stateFiles(root, state string) ([]string, error) {
	dir := filepath.Join(root, taskRoot, state)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%s/%s is missing", taskRoot, state)
	}
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		rel := taskRoot + "/" + state + "/" + entry.Name()
		if entry.Name() == taskKeeper && !entry.IsDir() {
			continue
		}
		if entry.IsDir() {
			if state == historyState {
				return nil, fmt.Errorf("%s: %s/ is history and stays flat; keep finished tasks directly in %s/%s", rel, historyState, taskRoot, historyState)
			}
			grouped, err := groupFiles(root, rel, entry.Name())
			if err != nil {
				return nil, err
			}
			files = append(files, grouped...)
			continue
		}
		if err := validateTaskFile(root, rel, entry.Name()); err != nil {
			return nil, err
		}
		files = append(files, rel)
	}
	return files, nil
}

// groupFiles validates one initiative directory and returns the task files in
// it plus its optional index. Groups do not nest: an initiative is one level,
// so the board never becomes a tree that hides where a task lives.
func groupFiles(root, relDir, name string) ([]string, error) {
	if !groupName.MatchString(name) {
		return nil, fmt.Errorf("%s is not a task group; a group is lowercase, such as object-versioning, and a task is <ID>.md", relDir)
	}
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(relDir)))
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		rel := relDir + "/" + entry.Name()
		if entry.Name() == taskKeeper && !entry.IsDir() {
			continue
		}
		if entry.IsDir() {
			return nil, fmt.Errorf("%s is not a task file; groups do not nest, keep one level under %s", rel, path.Dir(relDir))
		}
		if entry.Name() == groupIndex {
			if err := validateGroupIndex(root, rel); err != nil {
				return nil, err
			}
			files = append(files, rel)
			continue
		}
		if err := validateTaskFile(root, rel, entry.Name()); err != nil {
			return nil, err
		}
		files = append(files, rel)
	}
	return files, nil
}

// validateGroupIndex keeps an initiative index to the same title-first shape as
// the rest of the board, so it cannot become frontmatter or a file with no
// heading to anchor links.
func validateGroupIndex(root, rel string) error {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# ") {
			return nil
		}
		break
	}
	return fmt.Errorf("%s must start with a \"# \" title", rel)
}

// validateTaskFile checks that one entry is <ID>.md and starts with that ID.
func validateTaskFile(root, rel, name string) error {
	id := strings.TrimSuffix(name, ".md")
	if id == name || !taskID.MatchString(id) {
		return fmt.Errorf("%s is not a task file; name it <ID>.md with an ID like PLUGIN-INTERPRETER-01", rel)
	}
	return validateTaskTitle(root, rel, id)
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
