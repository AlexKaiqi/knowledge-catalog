package cache_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/cache"
)

// This fixture models only decoding an immutable Snapshot value. It performs
// no network/storage work and adds no artificial sleep; results quantify local
// cache overhead and avoided source calls, not production authority latency.
type decodingRepository struct {
	objectmodel.Repository
	id    kernel.RepositoryID
	body  []byte
	reads int
}

func (r *decodingRepository) ID() kernel.RepositoryID { return r.id }
func (r *decodingRepository) ReadMany(ids []objectmodel.ObjectID, commit kernel.CommitID) (map[objectmodel.ObjectID]objectmodel.ObjectValue, error) {
	r.reads++
	out := make(map[objectmodel.ObjectID]objectmodel.ObjectValue, len(ids))
	for _, id := range ids {
		var decoded objectmodel.ObjectValue
		if err := json.Unmarshal(r.body, &decoded); err != nil {
			return nil, err
		}
		out[id] = decoded
	}
	return out, nil
}

func BenchmarkSnapshotBodyRead(b *testing.B) {
	const repositoryID kernel.RepositoryID = "kr://benchmark/snapshot"
	const commit kernel.CommitID = "immutable-commit"
	const objectID objectmodel.ObjectID = "document/reference"
	content := map[string]any{"body": strings.Repeat("Snapshot reference content. ", 1024)}
	for i := 0; i < 32; i++ {
		content[fmt.Sprintf("field_%02d", i)] = []any{float64(i), "value", map[string]any{"nested": true}}
	}
	body, err := json.Marshal(value(repositoryID, commit, objectID, content))
	if err != nil {
		b.Fatal(err)
	}
	for _, mode := range []string{"direct_authority", "cold_cache", "hot_cache"} {
		b.Run(mode, func(b *testing.B) {
			repository := &decodingRepository{id: repositoryID, body: body}
			c, err := cache.New(cache.Config{})
			if err != nil {
				b.Fatal(err)
			}
			ids := []objectmodel.ObjectID{objectID}
			if mode == "hot_cache" {
				if _, err := c.ReadMany(repository, commit, ids); err != nil {
					b.Fatal(err)
				}
			}
			repository.reads = 0
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var values map[objectmodel.ObjectID]objectmodel.ObjectValue
				var err error
				switch mode {
				case "direct_authority":
					values, err = repository.ReadMany(ids, commit)
				case "cold_cache":
					c.Clear()
					values, err = c.ReadMany(repository, commit, ids)
				case "hot_cache":
					values, err = c.ReadMany(repository, commit, ids)
				}
				if err != nil || len(values) != 1 {
					b.Fatalf("read failed: %v", err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(repository.reads)/float64(b.N), "source_reads/op")
		})
	}
}
