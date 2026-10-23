# DEBT-INDEX-AUTH-VOCAB-01 · 对外词汇统一为 index 后，授权词仍是 projection

## 债

- 位置：`invocation/allow.go:40` 的动作映射（`describe-index` → `projection.read`、`index-sync` →
  `projection.manage`）、`invocation/observation_context.go:164-166`、`home/config/access.go:30`、
  `home/config/managed.go:83`、`server/repository_readiness.go:52`。
- 信号：用户能读到的命令、HTTP 路径、响应字段都改成 index 后，授权词仍是 `projection.read` 与
  `projection.manage`。同一个能力对外有两种叫法，排障时要在两套词之间换算。
- 判定：改授权词是破坏性变更，所有已有 allow 文件、`home` 配置与场景授权都要迁移，拒绝信息里的动作名
  也会变。与词汇统一同批做会把一次改名扩大成一次授权迁移。
- 处置：等确实要统一授权词时一次做完，方向为直接切换，不留双词。
- 触发条件：任何要求"对外只有一套词"的验收，或新增面向用户的授权说明时。

## 关闭

2026-10-22 并入 `INDEX-SURFACE-01`，未按本文件单独验收。

并入理由：本条与 `INDEX-SURFACE-01` 改同一批文件——`invocation/allow.go` 的动作映射表、`home/config/`
的访问配置、`server/repository_readiness.go` 的检索状态判定。原债自己的"前置"也写着要等
`INDEX-SURFACE-01` 让命令与路由词稳定后再做，而该任务尚未开工，所以并入不改变任何时序，只是不再让它
在 `todo/` 里排第二次队。

承接内容与变化：

- 原债给出的两个方向（新增 `index.read`/`index.manage` 并接受旧词一个过渡期、或直接切换一次改净）
  保留为 `INDEX-SURFACE-01` 的验收要求，仍规定必须一次改净、不留长期双词。
- 原债"不保留永久双词；若走过渡期必须写明结束条件与触发人"这一条保留在 `INDEX-SURFACE-01` 的
  "不做"里。
- 原债"不改 `projection.read` 的语义范围，本债只处理词"这一条保留：授权词换名字，`file.read` 与
  Dataset 发现范围的判定不变，`server/repository_readiness.go` 的检索状态不得因改词而放宽或收紧。
