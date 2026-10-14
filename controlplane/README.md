# controlplane/

上层维护闭环：`PROPOSAL → Preview → validate → Merge`。它编排 Dataset 服务 pin、Writer candidate ref 和 gate 证据，不定义新的对象写面。

| 文件 | 负责 |
|---|---|
| `controlplane.go` | 依赖装配、journal 与 merge gate 配置 |
| `proposal.go` | candidate proposal 创建 |
| `preview.go` | Workspace candidate overlay 与固定 Preview |
| `validation.go` | 结构检查、外部 validation 结果绑定 |
| `merge.go` | basis / evidence / candidate CAS 后合并目标 ref |
| `state.go` | 本机 proposal / preview / validation 状态文件 |

服务装配绑定唯一登记权威后恢复治理状态；旧分组账仅采用身份匹配的单份状态，空旧全局键沿原迁移合同恢复。外来、多份或混合旧状态明确拒绝，启动不写账。权威身份是恢复核对依据，不是调用方的选择参数。
