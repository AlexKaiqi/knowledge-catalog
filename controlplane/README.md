# controlplane/

上层维护闭环：`PROPOSAL → Preview → validate → Merge`。它编排 Catalog pin、Writer candidate ref 和 gate 证据，不定义新的知识写面。

| 文件 | 负责 |
|---|---|
| `controlplane.go` | 依赖装配、journal 与 merge gate 配置 |
| `proposal.go` | candidate proposal 创建 |
| `preview.go` | Workspace candidate overlay 与固定 Preview |
| `validation.go` | 结构检查、外部 validation 结果绑定 |
| `merge.go` | basis / evidence / candidate CAS 后合并目标 ref |
| `state.go` | 本机 proposal / preview / validation 状态文件 |
