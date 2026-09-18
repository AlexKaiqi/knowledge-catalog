# connector/

Collector 侧的 Address 对账 helper。具体外部系统实现、运行宿主和源客户端在业务共建的 integration repo；本包不连源，也不拥有 Writer。Observer（change notice）和 Resource Access（origin 访问地址）不在本包。

```text
外部当前态 → Collector 对账 → connector.Preview → ChangeSet → Writer COMMIT
```

`connector.Preview` 保留两种模式：

- `patch` 不推断删除；
- `reconcile` 只在 `Observed ∩ Scope` 内产生 REMOVE。

`Unit` 可同时声明 `schemaRef` 与 `valueSource`。Checkpoint 的 `Observed`
必须保存值 digest 和 declaration digest；这样 Binding 或 Schema 声明变化即使
业务值仍为 `null`，也会产生 PUT。旧的 value-only observation 只兼容没有
Schema/Binding 声明的单元。

预览为空则跳过，否则调用方经 Writer `Commit`。Descriptor 访问句柄及 integration repo / runtime 边界见 [`docs/CONNECTORS.md`](../docs/CONNECTORS.md)。

| 文件 | 负责 |
|---|---|
| `types.go` | Signal / Unit / Observed / Scope / Plan / Checkpoint |
| `preview.go` | `Preview`、`Envelope`、`CommandID`；只生成 ChangeSet |

对账前经 `kc writer head --repo …` 取得目标固定 `baseCommit`。人写 YAML 用 `kc writer commit --dir`；采集器用 `connector.Preview` 内部求差，`--command-id` 提交。CLI 参数以 [`cli/SURFACE.md`](../cli/SURFACE.md) 和帮助文本为准。
