# kernel/

无依赖底座，只拥有三类所有层都可安全使用的概念：统一错误信封、canonical digest、Snapshot 的 `RepositoryID / CommitID` 坐标。

`kernel` 不是共享类型桶。`ObjectID / Address / ObjectRef / schema_ref / provenance` 属于 `object/`；原始 `FileRef` 属于 `snapshot/`。`internal/arch` 会阻止这些声明回流。

| 文件 | 负责 |
|---|---|
| `errors.go`、`write_outcome.go` | 错误码、错误信封、可证明的写入结果与边界归一化 |
| `digest.go` | 无领域含义的 canonical JSON digest |
| `json.go` | 无损 JSON 数字解码及 canonical 数值表示；不解释领域字段 |
| `identity.go` | Repository / Commit 坐标与 Digest 类型 |

`UnmarshalJSON` / `DecodeJSON` 保留 JSON 数字的十进制值；能经标准 JSON 编码无损往返的数字
继续使用 `float64`，其余使用 `json.Number`。`DecodeJSON` 保留调用者设置的未知字段策略，
输入大小限制和单值检查仍由调用者负责。Canonical digest 对数学等价的十进制数字使用
同一表示，区分相邻大整数；大指数保持紧凑表示，不展开任意次幂。

错误信封保留 `code` 与 `message`，可选 `writeOutcome: "NOT_APPLIED"` 只表达提供方已证明
目标未发布。错误归一化沿包装链保留该证明，不修改可能被复用的原错误。缺失或未知取值
不表达任何发布结论，不能按临时错误码、HTTP 状态或消息文字推断；该标记不是自动重试指令。
