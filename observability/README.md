# observability/

非 Canonical 的过程证据。身份与 trace 是横切上下文；access / feedback 是原始账；hitmap 是从版本化 access 证据派生的统计。

| 文件组 | 负责 |
|---|---|
| `identity.go` / `trace.go` | 身份、代理关系与关联上下文校验 |
| `access.go` / `feedback.go` | 版本化事件契约 |
| `hitmap.go` | 派生访问聚合 `HitmapEntry`；不是 Retrieval `KnowledgeHit` |
| `file_store.go` | JSONL 记录入口 |
| `access_store.go` / `trace_store.go` / `hitmap_store.go` | 各查询视图 |
| `jsonl.go` | 本包私有 JSONL 读取 plumbing |
