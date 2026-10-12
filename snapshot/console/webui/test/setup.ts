// Node's optional Web Storage globals can shadow happy-dom's storage.
// Use an isolated browser store for each test worker without a disk file.
function storage(): Storage {
    const entries = new Map<string, string>();
    return {
        get length() { return entries.size; },
        clear: () => entries.clear(),
        getItem: (key) => entries.get(key) ?? null,
        key: (index) => Array.from(entries.keys())[index] ?? null,
        removeItem: (key) => { entries.delete(key); },
        setItem: (key, value) => { entries.set(key, String(value)); },
    };
}
Object.defineProperty(window, 'localStorage', { value: storage(), configurable: true });
Object.defineProperty(window, 'sessionStorage', { value: storage(), configurable: true });

// happy-dom 12 leaves pushState/replaceState as no-ops. Supply their browser
// contract for router tests; the built shell is also checked in a real browser.
const browser = window as unknown as { happyDOM: { setURL: (url: string) => void } };
for (const method of ['pushState', 'replaceState'] as const) {
    Object.defineProperty(window.history, method, { configurable: true, value: (state: unknown, _unused: string, url?: string | URL | null) => {
        if (url !== null && url !== undefined) browser.happyDOM.setURL(new URL(String(url), window.location.href).href);
        Object.defineProperty(window.history, 'state', { value: state, configurable: true });
    } });
}
