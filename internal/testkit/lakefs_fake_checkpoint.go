package testkit

import "net/http"

// SnapshotStamps restores owned refs through the same HTTP checkpoint protocol
// as the live fixture. Physical authorities and immutable commits remain stable.
func (f *LakeFSFake) SnapshotStamps(home string) (func() error, error) {
	f.mu.Lock()
	names := make([]string, 0, len(f.repos))
	namespaces := map[string]string{}
	for name, repo := range f.repos {
		names = append(names, name)
		namespaces[name] = repo.storageNamespace
		if namespaces[name] == "" {
			namespaces[name] = "s3://bucket/" + name
		}
	}
	f.mu.Unlock()
	checkpoint := &LakeFSLive{origin: f.Origin(), cred: f.Credential(), client: &http.Client{}, created: names, checkpointNamespaces: namespaces}
	return checkpoint.SnapshotStamps(home)
}
