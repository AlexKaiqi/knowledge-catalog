package testkit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// sharedSearchOwnerEnv marks a process that already belongs to a run which
// owns this instance's projection indices. Tests re-execute their own binary as
// a service (the serve lifecycle fixtures do), and that child also runs
// TestMain: without this marker it would join the session as a member and hold
// the reclaim back forever, or reclaim indices its still-running parent is
// asserting on.
const sharedSearchOwnerEnv = "KC_TEST_OPENSEARCH_OWNER"

// sharedSearchSessionDirEnv overrides where the cross-process session state
// lives. Only tests of this package set it, because they must exercise the
// multi-member rule without touching the session a real run is using.
const sharedSearchSessionDirEnv = "KC_TEST_OPENSEARCH_SESSION_DIR"

const (
	sharedSearchSessionLockFile   = "session.lock"
	sharedSearchSessionBeforeFile = "before.json"
	sharedSearchMemberPrefix      = "member-"
)

// sharedSearchMemberCounter keeps member lease names unique within one process,
// which a test joining twice needs and a second run never sees.
var sharedSearchMemberCounter atomic.Uint64

// OwnSharedSearchIndices joins the run's shared-instance search session and
// returns the function that ends this process's membership.
//
// Suites pointed at one explicit endpoint adopt its legacy shared resources:
// their fixtures create projection indices that nobody deletes, so a package
// that indexes a few hundred repositories per run leaves the same number of
// shards behind and eventually fails on the cluster shard limit rather than on
// an assertion. Reclaiming is the same rule the test-group entries apply at
// their boundary (scripts/opensearch_fixture.py guard): take one inventory,
// run, then delete exactly what appeared. Existing indices are never touched,
// because they belong to whoever created them.
//
// The session exists because Go runs packages in parallel: `go test ./...` puts
// cli and home on this instance at the same time, and each of them derives its
// own difference set. A difference set cannot tell one package's new index from
// another's, so a package reclaiming while a sibling is still asserting deletes
// live indices. Membership is therefore a cross-process lease, and only the
// last member out reclaims.
//
// Ownership cannot be claimed by name instead. Deriving one namespace per test
// would need the fixtures to choose it, and the fixtures are product code that
// must keep the empty-namespace contract; deriving one namespace per run makes
// two homes share physical indices, which is a correctness change to the tests
// rather than a cleanup. The difference-set rule needs neither.
func OwnSharedSearchIndices() func() error {
	endpoint := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	if endpoint == "" || !sharedSearchSessionLockSupported {
		return nil
	}
	if os.Getenv(sharedSearchOwnerEnv) != "" {
		return nil
	}
	session, err := joinSharedSearchSession(endpoint)
	if err != nil {
		// A session this process cannot join is a run that cannot clean up
		// after itself, so it must not proceed pretending it can.
		return func() error { return fmt.Errorf("join the shared search session: %w", err) }
	}
	if err := os.Setenv(sharedSearchOwnerEnv, session.id); err != nil {
		session.member.release()
		return func() error { return fmt.Errorf("record projection ownership: %w", err) }
	}
	return func() error {
		defer func() { _ = os.Unsetenv(sharedSearchOwnerEnv) }()
		return session.reclaim()
	}
}

// sharedSearchSession is one process's membership in a run against a shared
// instance. Every member shares the inventory the first member took, so the
// difference set is the whole run's rather than one package's.
type sharedSearchSession struct {
	endpoint string
	dir      string
	id       string
	member   *sessionMember
}

func joinSharedSearchSession(endpoint string) (*sharedSearchSession, error) {
	dir := sharedSearchSessionDir(endpoint)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	session := &sharedSearchSession{endpoint: endpoint, dir: dir}
	unlock, err := lockSharedSearchSession(dir)
	if err != nil {
		return nil, err
	}
	defer unlock()
	alive, err := session.reapMembers()
	if err != nil {
		return nil, err
	}
	if alive == 0 {
		// The first member of a session records what the instance already held.
		// A snapshot left behind by a crashed session is kept instead of
		// retaken: its entries stay untouched, and everything the crashed run
		// created is still outside it, so the next session's reclaim picks that
		// up rather than adopting it as pre-existing.
		if err := session.recordInventory(); err != nil {
			return nil, err
		}
	}
	member, err := acquireSessionMember(dir)
	if err != nil {
		return nil, err
	}
	session.member = member
	session.id = filepath.Base(member.path)
	return session, nil
}

// reclaim ends this process's membership and, when it was the last one, removes
// the projection indices the whole session added.
func (s *sharedSearchSession) reclaim() error {
	unlock, err := lockSharedSearchSession(s.dir)
	if err != nil {
		return err
	}
	defer unlock()
	s.member.release()
	s.member = nil
	alive, err := s.reapMembers()
	if err != nil {
		return err
	}
	if alive > 0 {
		// Sibling packages are still running against the same instance. Their
		// indices are indistinguishable from this one's by name, so reclaiming
		// now would delete what they are still asserting on. The last member
		// out reclaims once, after all of them have closed their homes.
		return nil
	}
	before, err := s.readInventory()
	if err != nil {
		return err
	}
	after, err := sharedSearchInventory(s.endpoint)
	if err != nil {
		return fmt.Errorf("inventory test projection indices after the run: %w", err)
	}
	names := make([]string, 0, len(after))
	for name := range after {
		names = append(names, name)
	}
	sort.Strings(names)
	problems := make([]error, 0)
	for _, name := range names {
		if _, existed := before[name]; existed {
			continue
		}
		if !strings.HasPrefix(name, "kc-proj-") && !strings.HasPrefix(name, "kc-projection-control-v1") {
			// Another program's index appeared during the run; it is not ours.
			continue
		}
		if err := deleteProjectionIndexNow(s.endpoint, name); err != nil {
			problems = append(problems, err)
		}
	}
	_ = os.Remove(filepath.Join(s.dir, sharedSearchSessionBeforeFile))
	if len(problems) == 0 {
		return nil
	}
	return problems[0]
}

// recordInventory writes the session's opening inventory unless a crashed
// session already left one.
func (s *sharedSearchSession) recordInventory() error {
	path := filepath.Join(s.dir, sharedSearchSessionBeforeFile)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	inventory, err := sharedSearchInventory(s.endpoint)
	if err != nil {
		return fmt.Errorf("inventory test projection indices before the run: %w", err)
	}
	names := make([]string, 0, len(inventory))
	for name := range inventory {
		names = append(names, name)
	}
	sort.Strings(names)
	raw, err := json.Marshal(names)
	if err != nil {
		return err
	}
	// Write through a temporary name so a reader never sees a half snapshot and
	// treats the truncated result as a complete one.
	temporary := path + ".tmp-" + strconv.Itoa(os.Getpid())
	if err := os.WriteFile(temporary, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func (s *sharedSearchSession) readInventory() (indexInventory, error) {
	path := filepath.Join(s.dir, sharedSearchSessionBeforeFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		// Without the opening inventory there is no difference to take, and
		// guessing one would delete indices that existed before the run.
		return nil, fmt.Errorf("read the session's opening projection inventory: %w", err)
	}
	var names []string
	if err := json.Unmarshal(raw, &names); err != nil {
		return nil, fmt.Errorf("read the session's opening projection inventory: %w", err)
	}
	inventory := make(indexInventory, len(names))
	for _, name := range names {
		inventory[name] = struct{}{}
	}
	return inventory, nil
}

// reapMembers drops the leases of members that have exited and reports how many
// still hold one. A lease is held by flock, so a member that was killed without
// running its own cleanup is detected the same way as one that exited.
func (s *sharedSearchSession) reapMembers() (int, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0, err
	}
	alive := 0
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), sharedSearchMemberPrefix) {
			continue
		}
		path := filepath.Join(s.dir, entry.Name())
		file, err := os.OpenFile(path, os.O_RDWR, 0o600)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return alive, err
		}
		acquired, err := tryLockExclusive(file)
		if err != nil {
			_ = file.Close()
			return alive, err
		}
		if acquired {
			_ = os.Remove(path)
			_ = file.Close()
			continue
		}
		_ = file.Close()
		alive++
	}
	return alive, nil
}

func acquireSessionMember(dir string) (*sessionMember, error) {
	for attempt := 0; attempt < 16; attempt++ {
		name := sharedSearchMemberPrefix + strconv.Itoa(os.Getpid()) + "-" +
			strconv.FormatInt(time.Now().UnixNano(), 10) + "-" +
			strconv.FormatUint(sharedSearchMemberCounter.Add(1), 10)
		path := filepath.Join(dir, name)
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if err != nil {
			if os.IsExist(err) {
				continue
			}
			return nil, err
		}
		acquired, err := tryLockExclusive(file)
		if err != nil {
			_ = file.Close()
			_ = os.Remove(path)
			return nil, err
		}
		if !acquired {
			_ = file.Close()
			_ = os.Remove(path)
			return nil, fmt.Errorf("session member lease %s is already held", name)
		}
		return &sessionMember{path: path, file: file}, nil
	}
	return nil, fmt.Errorf("allocate a session member lease")
}

// sessionMember is one process's lease. Removing the name before closing keeps a
// reaper that already opened the file from mistaking the released lease for a
// live one: flock stays held until the close that follows.
type sessionMember struct {
	path string
	file *os.File
}

func (m *sessionMember) release() {
	if m == nil || m.file == nil {
		return
	}
	_ = os.Remove(m.path)
	_ = m.file.Close()
	m.file = nil
}

func sharedSearchSessionDir(endpoint string) string {
	if explicit := strings.TrimSpace(os.Getenv(sharedSearchSessionDirEnv)); explicit != "" {
		return explicit
	}
	sum := sha256.Sum256([]byte(endpoint))
	return filepath.Join(os.TempDir(), "kc-test-shared-search-"+hex.EncodeToString(sum[:8]))
}

func sharedSearchInventory(endpoint string) (indexInventory, error) {
	// kc-proj* matches both the generation indices (kc-proj-<scope>-<repo>-g-<gen>)
	// and the legacy control index (kc-projection-control-v1), which is also
	// created on demand and would otherwise survive the run.
	response, err := http.Get(endpoint + "/_cat/indices/kc-proj*?format=json&h=index&expand_wildcards=all")
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		return nil, fmt.Errorf("projection inventory HTTP %d", response.StatusCode)
	}
	var rows []struct {
		Index string `json:"index"`
	}
	if err := json.NewDecoder(response.Body).Decode(&rows); err != nil {
		return nil, err
	}
	inventory := make(indexInventory, len(rows))
	for _, row := range rows {
		inventory[row.Index] = struct{}{}
	}
	return inventory, nil
}

// indexInventory is the projection index names present at one moment.
type indexInventory map[string]struct{}
