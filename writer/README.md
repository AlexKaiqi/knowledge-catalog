# writer/

**Writer 是写入面**：一次一个 target、一种 Surface、一个 `command_id`。

```text
COMMIT / PROPOSAL  →  ⓪ Snapshot（ChangeSet 的 PUT/REMOVE 是 ② Aspect 分区）
APPEND             →  ⓪ Stream（有序段，不是 git；JSONL 同居 packing）
```

变更代数只有 `PUT(address, full_value)` / `REMOVE(address)`。Create = PUT + `IF_ABSENT`；Update = PUT + `IF_DIGEST_EQUALS`；Upsert = PUT 无目标条件。

`Ingest` / `Reconcile` **只出 ChangeSet 预览**，不是 Surface，也不是采集框架。确认后走 `Commit`。Address 级、按变化源拆 Scope 的源对账在 `connector/`（见 `docs/CONNECTORS.md`）。Writer 不 import 该包。

## 谁被创建

| 对象 | 怎么来 | 之后 |
|---|---|---|
| Snapshot commit | `Commit` / `CommitIntent` | 推 target Ref；Receipt `APPLIED` / `REPLAYED` |
| Candidate commit | `Propose` | 只动 candidate；合入是 ControlPlane `merge` |
| Stream records | `Append` / `AppendIntent` | cursor 前进；git HEAD 不动 |
| ChangeSet 预览 | `Ingest` / `Reconcile` | 不写库；`kc commit --changeset` |
| 幂等记录 | 成功写入后记入 `.kc/writer.json` | 同 id 同 digest → 原 Receipt |

不要直写 git / 工作区文件绕过 Writer。不要为场景再开一种 Surface。

## 文件（按 Surface / 变化拆）

| 文件 | 负责 |
|---|---|
| `writer.go` | `Writer`：构造、校验 ChangeSet、`applySnapshot`（COMMIT 与 PROPOSAL 共用） |
| `schema.go` | 写时校验 `schema_ref`：target 仓内 `schema/*` 必须可解析 |
| `commit.go` | COMMIT：`CommitIntent` 从当前 Ref 填 CAS |
| `propose.go` | PROPOSAL：建/用 candidate Ref，Receipt `Surface=PROPOSAL` |
| `append.go` | APPEND：`AppendIntent` 从当前 stream 填 cursor |
| `receipt.go` | Durable Receipt（`APPLIED` / `REPLAYED`） |
| `idempotency.go` | `command_id` 日志：Lookup / 落盘 `.kc/writer.json` |
| `preview.go` | `Ingest` / `Reconcile`：预览，不写 |

`GroundingCitation` 在 `reader/`：它是 READ 结果的引用投影（D12），不是写面。

## 执行与幂等

```text
填 CAS/cursor → canonicalize + digest → 查 command_id
  同 id 同 digest → 原 Receipt（REPLAYED）
  同 id 异 digest → IDEMPOTENCY_CONFLICT（换新 id）
  首次 → 校验 Address / provenance / schema_ref → 调 SnapshotStore 或 Stream → 记日志 → APPLIED
```

Intent 首次从当前 Ref / cursor 填前置条件；重试复用已存请求的 CAS/cursor。再取一遍「现在的 head」是另一条命令。

DERIVATION 必须带固定 `inputViewReadVersionRef` + algorithm，否则拒写。仓已 `archive-repo` → `REPOSITORY_ARCHIVED`。空 ChangeSet → `WRITE_TARGET_REQUIRED`。带 `schema_ref` 的 COMMIT / PROPOSAL / APPEND 必须在 target 仓解析到 `schema/*`，否则 `SCHEMA_REVISION_UNRESOLVED`。

## CLI

```bash
go run ./cmd/kc -- init --catalog acme/catalog
go run ./cmd/kc -- repo-add --repo kr://acme/public/core

go run ./cmd/kc -- put --command-id sync-1 --repo kr://acme/public/core \
  --object runbooks/oncall --value '{"text":"check freeze"}' \
  --if-absent --origin-kind SOURCE

go run ./cmd/kc -- ingest --repo kr://acme/public/core --dir ./draft --out cs.json
go run ./cmd/kc -- commit --command-id ingest-1 --changeset cs.json
go run ./cmd/kc -- receipt --command-id ingest-1

go run ./cmd/kc -- append --command-id run-1 --repo kr://acme/public/core \
  --stream runs --event-id evt-1 --payload '{"status":"ok"}'
```

`kc propose` 走 `Writer.Propose`（candidate）；`merge` 快进成员 Ref，**不**自动 promote。

默认闭环：`init → repo-add → put/commit/append → read --repo`。View / Release 不是写入的前提。
