package identity

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Settings is the deployment configuration one authenticator factory reads.
// Names are the `kc serve` flag names; the caller adapts its own flag shape so
// authentication never depends on a command vocabulary.
type Settings interface {
	String(name string) string
	Strings(name string) []string
}

// Factory builds one authenticator from deployment settings. Each factory
// validates its own required settings.
type Factory func(Settings) (HTTPAuthenticator, error)

var (
	registryMu sync.RWMutex
	factories  = map[string]Factory{}
)

// Register adds a named authenticator. The name becomes the value accepted by
// `--auth`. Registration is append-only; registering a name twice panics.
func Register(name string, factory Factory) {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" || factory == nil {
		panic("identity: empty authenticator registration")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := factories[name]; dup {
		panic(fmt.Sprintf("identity: duplicate authenticator %q", name))
	}
	factories[name] = factory
}

// Lookup returns the registered factory for one `--auth` name.
func Lookup(name string) (Factory, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	factory, ok := factories[strings.TrimSpace(strings.ToLower(name))]
	return factory, ok
}

// Names lists registered authenticator names in sorted order.
func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// settingsError is returned verbatim by Resolve; other factory errors are
// prefixed with the selected mode.
type settingsError struct{ message string }

func (e *settingsError) Error() string { return e.message }

// SettingsError reports a complete operator-facing configuration message.
func SettingsError(format string, args ...any) error {
	return &settingsError{message: fmt.Sprintf(format, args...)}
}

// Resolve selects and builds one registered authenticator.
func Resolve(name string, settings Settings) (HTTPAuthenticator, error) {
	name = strings.TrimSpace(strings.ToLower(name))
	factory, ok := Lookup(name)
	if !ok {
		return nil, fmt.Errorf("kc serve requires --auth %s", strings.Join(Names(), ", "))
	}
	authenticator, err := factory(settings)
	if err != nil {
		var verbatim *settingsError
		if errors.As(err, &verbatim) {
			return nil, err
		}
		return nil, fmt.Errorf("--auth %s: %v", name, err)
	}
	return authenticator, nil
}
