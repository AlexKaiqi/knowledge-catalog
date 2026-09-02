# observability/

非 Canonical 的过程证据。身份与 trace 是横切上下文；access / retrieval / refine / feedback 是原始账；hitmap 与 training sample 是可重建派生视图。

本包负责知识访问证据，不等于完整的运行监控。metric、诊断日志、distributed trace、健康检查与 SLO 的边界见 [`docs/SYSTEM_OBSERVABILITY.md`](../docs/SYSTEM_OBSERVABILITY.md)。写入与访问语义见 [`docs/OBSERVABILITY.md`](../docs/OBSERVABILITY.md)。

协议口：

- `Recorder`：fail-closed 追加；access/retrieval/refine 在耐久 ack 后返回 `Receipt.EvidenceID`。
- `AccessLog`：`GetAccess(evidenceId)` 与带时间窗的 `Access` 分页查询。
- `FileStore`：本机 JSONL adapter，同时实现上述口。其它介质只要满足同一语义即可替换。

| 文件组 | 负责 |
|---|---|
| `observability.go` | Recorder / AccessLog / Receipt |
| `identity.go` / `trace.go` | 身份、代理关系与关联上下文校验 |
| `access.go` / `retrieval.go` / `refine.go` / `feedback.go` | 版本化访问、候选检索、语义推理与监督反馈合同 |
| `hitmap.go` | 派生访问聚合 `HitmapEntry`；不是 Retrieval `KnowledgeHit` |
| `file_store.go` | JSONL 记录入口（本机 adapter） |
| `access_store.go` / `retrieval_store.go` / `refine_store.go` / `trace_store.go` / `hitmap_store.go` | 各查询视图 |
| `training.go` | 从 retrieval/refine + feedback 重建带标签强度的样本；不是原始账 |
| `jsonl.go` | 本包私有 JSONL 读取 plumbing |
