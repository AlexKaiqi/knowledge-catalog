# retrieval/sqlite/

本机 reference Projection provider。SQLite 只保存可重建候选索引；命中后由 `index/` 在同一 basis 回读 Canonical。

| 文件 | 负责 |
|---|---|
| `sqlite.go` | engine 生命周期、schema、Meta 与 Rebuild/Apply |
| `query.go` | logical clause → SQLite 条件与参数 |
| `search.go` | capability Probe、分页检索与 CandidateRef |
