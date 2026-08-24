# connector/

Collector 侧的 Address 对账 helper。具体外部系统实现、运行宿主和源客户端在业务共建的 integration repo 或场景侧；本包不连源，也不拥有 Writer。

```text
外部当前态 → Collector → connector.Preview → ChangeSet → Writer COMMIT
```

`connector.Preview` 保留两种模式：

- `patch` 不推断删除；
- `reconcile` 只在 `Observed ∩ Scope` 内产生 REMOVE。

预览为空则跳过，否则调用方经 Writer `Commit`。Descriptor 访问句柄及 integration repo / runtime 边界见 [`docs/CONNECTORS.md`](../docs/CONNECTORS.md)。

| 文件 | 负责 |
|---|---|
| `types.go` | Signal / Unit / Observed / Scope / Plan / Checkpoint |
| `preview.go` | `Preview`、`Envelope`、`CommandID`；只生成 ChangeSet |
