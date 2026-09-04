# delivery/

**交付链**：③ 已经定位并 hydrate 之后、transport 编码之前，把「知识 ID + Canonical」变成调用方可见内容。不是 ④ 协议层，不是出站 Hook，也不进 `retrieval/` / `index/`。

```text
知识 ID（PinnedKnowledgeRef）+ 已 hydrate 的 Canonical
        → Chain.Apply
        → 可见正文（或屏蔽后的信封）
```

公开类型：`Envelope`（输入/输出）、`Chain`、`Stage` / `StageFunc`、首段 `RepositoryRead`。无 `knowledge.read` 时去掉正文、provenance、observation、units/declarations，保留 ID 与 Address。后续规则按序挂 `Stage`；未选定的隐私化不得实现。本包不读 `allow.json`。

| 文件 | 负责 |
|---|---|
| `chain.go` | `Envelope`、`Chain`、身份冻结 |
| `read.go` | 仓读权屏蔽 |

政策由 `docs/PERMISSIONS.md` §7.2 拥有。应用装配在 `cli/` 注入 `Allowed`。
