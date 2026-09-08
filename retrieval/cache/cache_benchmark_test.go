package cache_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval/cache"
)

// This fixture models only decoding an immutable Snapshot value. It performs
// no network/storage work and adds no artificial sleep; results quantify local
// cache overhead and avoided source calls, not production authority latency.
type decodingRepository struct {
	knowledge.Repository
	id    kernel.RepositoryID
	body  []byte
	reads int
}

func (r *decodingRepository) ID() kernel.RepositoryID { return r.id }
func (r *decodingRepository) ReadMany(ids []knowledge.ObjectID, commit kernel.CommitID) (map[knowledge.ObjectID]knowledge.KnowledgeValue, error) {
	r.reads++
	out := make(map[knowledge.ObjectID]knowledge.KnowledgeValue, len(ids))
	for _, id := range ids {
		var decoded knowledge.KnowledgeValue
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
	const objectID knowledge.ObjectID = "document/reference"
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
			ids := []knowledge.ObjectID{objectID}
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
				var values map[knowledge.ObjectID]knowledge.KnowledgeValue
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
