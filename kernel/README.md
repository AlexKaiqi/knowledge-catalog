# kernel/

无依赖底座，只拥有三类所有层都可安全使用的概念：统一错误信封、canonical digest、Snapshot 的 `RepositoryID / CommitID` 坐标。

`kernel` 不是共享类型桶。`ObjectID / Address / KnowledgeRef / schema_ref / provenance` 属于 `knowledge/`；原始 `FileRef` 属于 `snapshot/`。`internal/arch` 会阻止这些声明回流。

| 文件 | 负责 |
|---|---|
| `errors.go` | 错误码、错误信封与边界归一化 |
| `digest.go` | 无领域含义的 canonical JSON digest |
| `identity.go` | Repository / Commit 坐标与 Digest 类型 |
