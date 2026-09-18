package dolt

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// One `dolt` invocation starts a whole engine, so a per-query process makes a
// bounded read cost grow with call count instead of with stored knowledge. A
// database directory therefore keeps one live `dolt sql` session and sends
// statements to it. Only immutable and metadata queries use the session: a
// live session holds the database write lease, so every mutating command
// closes it first and Dolt stays the single active writer.

// engineAnswer is one statement's reply. Dolt writes a single JSON line per
// result set on stdout, and `--continue` reports a failing statement as one
// stderr line while keeping the session usable.
type engineAnswer struct {
	line string
}

// doltEngine is a live `dolt sql` process bound to one database directory.
// Statements are serialized, so exactly one answer is outstanding and stdout
// and stderr never compete to reply to the same statement.
type doltEngine struct {
	askMu      sync.Mutex
	stateMu    sync.Mutex
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	answers    chan engineAnswer
	statements int
	closed     bool
	waitOnce   sync.Once
	container  string
	rootDir    string
}

var (
	doltEnginesMu sync.Mutex
	doltEngines   = map[engineKey]*doltEngine{}
	// errEngineUnusable means this environment answered nothing on the session
	// transport. The caller falls back to a one-shot process so an unusual
	// KC_DOLT_BIN wrapper keeps working.
	errEngineUnusable = fmt.Errorf("dolt session is unusable")
)

// engineQuery answers query from this database's session, starting it on first
// use. The session is keyed by directory and by the selected executable, so
// changing the engine binary never reaches a session started by another one.
func engineQuery(rootDir, bin string, query string) (string, error) {
	key := engineKey{rootDir: rootDir, bin: bin}
	engine, err := acquireEngine(key)
	if err != nil {
		return "", err
	}
	out, err := engine.ask(query)
	if err == errEngineUnusable {
		releaseEngine(key, engine)
	}
	return out, err
}

type engineKey struct {
	rootDir string
	bin     string
}

func acquireEngine(key engineKey) (*doltEngine, error) {
	doltEnginesMu.Lock()
	if engine, ok := doltEngines[key]; ok {
		doltEnginesMu.Unlock()
		return engine, nil
	}
	doltEnginesMu.Unlock()
	engine, err := startDoltEngine(key.rootDir, key.bin)
	if err != nil {
		return nil, err
	}
	doltEnginesMu.Lock()
	if existing, ok := doltEngines[key]; ok {
		doltEnginesMu.Unlock()
		engine.close()
		return existing, nil
	}
	doltEngines[key] = engine
	doltEnginesMu.Unlock()
	return engine, nil
}

func releaseEngine(key engineKey, engine *doltEngine) {
	doltEnginesMu.Lock()
	if doltEngines[key] == engine {
		delete(doltEngines, key)
	}
	doltEnginesMu.Unlock()
	engine.close()
}

// closeEngineAt releases this directory's write lease. A mutating command runs
// as its own process and cannot proceed while a session holds the database.
func closeEngineAt(rootDir string) {
	doltEnginesMu.Lock()
	released := []*doltEngine{}
	for key, engine := range doltEngines {
		if key.rootDir == rootDir {
			released = append(released, engine)
			delete(doltEngines, key)
		}
	}
	doltEnginesMu.Unlock()
	for _, engine := range released {
		engine.close()
	}
}

func startDoltEngine(rootDir, bin string) (*doltEngine, error) {
	cmd, container := doltInvocation(rootDir, bin, true, dockerSessionName(rootDir), "sql", "-r", "json", "--continue")
	if container != "" {
		removeDockerContainer(container)
	}
	configureEngineCmd(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, childOutput, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	// Both child descriptors point at the same OS pipe. Dolt writes a
	// diagnostic before it accepts the following acknowledgement statement;
	// retaining that order prevents a stderr reader race from attributing the
	// diagnostic to the next query.
	cmd.Stdout = childOutput
	cmd.Stderr = childOutput
	if err := cmd.Start(); err != nil {
		_ = output.Close()
		_ = childOutput.Close()
		removeDockerContainer(container)
		return nil, err
	}
	_ = childOutput.Close()
	engine := &doltEngine{cmd: cmd, stdin: stdin, answers: make(chan engineAnswer, 8), container: container, rootDir: rootDir}
	go func() {
		defer close(engine.answers)
		defer output.Close()
		lines := bufio.NewScanner(output)
		lines.Buffer(make([]byte, 0, 64*1024), maxEngineAnswerBytes)
		for lines.Scan() {
			if text := strings.TrimSpace(lines.Text()); text != "" {
				engine.answers <- engineAnswer{line: text}
			}
		}
	}()
	return engine, nil
}

// maxEngineAnswerBytes bounds one result line. A native read is
// already paged by its caller, so an answer beyond this is a transport fault
// rather than a larger legitimate result.
const maxEngineAnswerBytes = 64 << 20

// ask sends one statement and returns its answer. Provider SQL spans lines and
// a diagnostic echoes the failing statement, so neither replies nor failures
// are reliably one line. Each statement is therefore followed by an
// acknowledgement query whose reply marks the end of that statement's output,
// which keeps every result and diagnostic attributable to its own call.
func (e *doltEngine) ask(query string) (string, error) {
	e.askMu.Lock()
	defer e.askMu.Unlock()
	e.stateMu.Lock()
	if e.closed {
		e.stateMu.Unlock()
		return "", errEngineUnusable
	}
	e.statements++
	ack := fmt.Sprintf("kc_session_ack_%d", e.statements)
	stdin := e.stdin
	e.stateMu.Unlock()
	statement := strings.TrimRight(strings.TrimSpace(query), ";")
	script := statement + ";\nSELECT " + sqlString(ack) + " AS kc_session_ack;\n"
	if _, err := io.WriteString(stdin, script); err != nil {
		return "", errEngineUnusable
	}
	var (
		result   string
		failures []string
	)
	for {
		answer, ok := <-e.answers
		if !ok {
			// The engine exited without acknowledging. This environment cannot
			// serve the session transport; the caller retries as one process.
			e.stateMu.Lock()
			e.closed = true
			e.stateMu.Unlock()
			return "", errEngineUnusable
		}
		// The acknowledgement closes this statement's output whether it was
		// answered or reported as an error.
		if strings.Contains(answer.line, ack) {
			break
		}
		if !isDoltJSONResult(answer.line) {
			failures = append(failures, answer.line)
			continue
		}
		if result != "" {
			failures = append(failures, "multiple result sets for one statement")
			continue
		}
		result = answer.line
	}
	if len(failures) > 0 {
		return "", fmt.Errorf("dolt sql: %s", strings.Join(failures, " "))
	}
	if result == "" {
		return "", fmt.Errorf("dolt sql: %s returned no result set", statement)
	}
	return result, nil
}

func isDoltJSONResult(line string) bool {
	raw := []byte(stripANSI(line))
	// Dolt emits {} for a valid empty result in some versions. Preserve the
	// one-shot decoder's behavior: any JSON object is a result; plain
	// diagnostics are failures.
	return len(raw) >= 2 && raw[0] == '{' && json.Valid(raw)
}

func (e *doltEngine) close() {
	e.stateMu.Lock()
	if e.closed {
		e.stateMu.Unlock()
		e.wait()
		return
	}
	e.closed = true
	_ = e.stdin.Close()
	cmd := e.cmd
	container := e.container
	e.stateMu.Unlock()
	// Docker fallback bind-mounts the database with the container cwd inside
	// it. Killing only the docker client leaves that container holding
	// `.dolt/noms`, so Close must remove the named container before the host
	// directory can be released.
	removeDockerContainer(container)
	if cmd != nil && cmd.Process != nil {
		killEngineCmd(cmd)
	}
	for range e.answers {
		// Drain so the stream readers finish before the process is reaped.
	}
	e.wait()
	killWrapperSession(e.rootDir)
}

func (e *doltEngine) wait() {
	e.waitOnce.Do(func() {
		_ = e.cmd.Wait()
	})
}

func dockerExecContainer(script string) string {
	idx := strings.Index(script, "docker exec")
	if idx < 0 {
		return ""
	}
	fields := strings.Fields(script[idx+len("docker exec"):])
	skipValue := false
	for _, field := range fields {
		if skipValue {
			skipValue = false
			continue
		}
		if strings.HasPrefix(field, "-") {
			if field == "-i" || field == "-t" || field == "-d" || field == "-it" || strings.Contains(field, "=") {
				continue
			}
			skipValue = true
			continue
		}
		return field
	}
	return ""
}

// killWrapperSession reaps a `dolt sql --continue` left in a shared docker
// exec engine after the client was killed. Named docker-run sessions are
// already removed by removeDockerContainer.
func killWrapperSession(rootDir string) {
	if strings.TrimSpace(rootDir) == "" {
		return
	}
	bin := doltBinary()
	if bin == "" {
		return
	}
	raw, err := os.ReadFile(bin)
	if err != nil {
		return
	}
	container := dockerExecContainer(string(raw))
	if container == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	script := `for pid in /proc/[0-9]*; do
  cwd=$(readlink "$pid/cwd" 2>/dev/null) || continue
  if [ "$cwd" = "$KC_KILL_ROOT" ]; then kill -9 "$(basename "$pid")" 2>/dev/null || true; fi
done`
	_ = exec.CommandContext(ctx, "docker", "exec", "-e", "KC_KILL_ROOT="+rootDir, container, "/bin/sh", "-c", script).Run()
}

