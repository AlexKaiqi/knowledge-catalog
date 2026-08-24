# connector/

**入站 kit（②）**：外部系统才是领域权威时，把「源当前态」对账成 ChangeSet 预览。不是采集框架，不是 Write Surface，不连源。挂用户 git（⓪）不是 connector。

Connector 是墙外进程。本包只做 Address 级、带 Scope 的 diff。确认后走 Writer `Commit` 或 `kc commit --changeset`。Writer / Catalog / CLI 不 import 本包。

```text
signal(keys) → 墙外 GET 当前态 → Desired Units
仓内 digest → Observed
connector.Preview(patch|reconcile) → ChangeSet
empty → 跳过；否则 COMMIT（origin_kind=SOURCE）
```

`writer.Ingest` / `Reconcile` 仍是本地目录 / object_id 实体对账。设计见 [`docs/CONNECTORS.md`](../docs/CONNECTORS.md)。出站是 [`../hook/`](../hook/)。

## 谁被创建

| 对象 | 怎么来 | 之后 |
|---|---|---|
| ChangeSet 预览 | `Preview` | 调用方 `Commit`；`empty` 则不要提交 |
| Checkpoint JSON | 调用方自己序列化 `Checkpoint` | kit 不落盘 |

## 文件

| 文件 | 负责 |
|---|---|
| `types.go` | Signal / Unit / Observed / Scope / Plan / Checkpoint |
| `preview.go` | `Preview`、`Envelope`、`CommandID` |
