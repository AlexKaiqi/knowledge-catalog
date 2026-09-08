# kernel/

无依赖底座，只拥有三类所有层都可安全使用的概念：统一错误信封、canonical digest、Snapshot 的 `RepositoryID / CommitID` 坐标。

`kernel` 不是共享类型桶。`ObjectID / Address / KnowledgeRef / schema_ref / provenance` 属于 `knowledge/`；原始 `FileRef` 属于 `snapshot/`。`internal/arch` 会阻止这些声明回流。

| 文件 | 负责 |
|---|---|
| `errors.go` | 错误码、错误信封与边界归一化 |
| `digest.go` | 无领域含义的 canonical JSON digest |
| `json.go` | 无损 JSON 数字解码及 canonical 数值表示；不解释领域字段 |
| `identity.go` | Repository / Commit 坐标与 Digest 类型 |

`UnmarshalJSON` / `DecodeJSON` 保留 JSON 数字的十进制值；能经标准 JSON 编码无损往返的数字
继续使用 `float64`，其余使用 `json.Number`。`DecodeJSON` 保留调用者设置的未知字段策略，
输入大小限制和单值检查仍由调用者负责。Canonical digest 对数学等价的十进制数字使用
同一表示，区分相邻大整数；大指数保持紧凑表示，不展开任意次幂。
