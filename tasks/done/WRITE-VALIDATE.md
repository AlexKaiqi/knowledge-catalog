# WRITE-VALIDATE · 进 published 的知识发布过同一套 Writer 校验

- [ ] 未认领：人可以不经过 `kc writer` CLI 改自有仓；成为已发布知识的那次提交必须过与 COMMIT 相同的 Schema / CAS 基点 / 同仓 `schema_ref`（保护分支或 CI 代发 / 只验不写）。owner 为 [`doc/design/dataset.md`](../../doc/design/dataset.md) §3.5、[`doc/design/governance.md`](../../doc/design/governance.md)、[`object/writer/README.md`](../../object/writer/README.md)。

**Goal：** Writer 代数可在不推进 ref 时复用；CI 有公开检查口。

**Non-Goals：** 不把校验塞进 ingestion control；不把外部 CI 做成每次 COMMIT 的 Gate；不抢走业务 git；不改 Connector 必须走 Writer。

**不变量：** `K-21`；Gate 只拦 proposal → published。

**选定与否决：** 选定目录写入走 `writer commit --dir`（对照当前版本求差是 commit 内部动作）。选定可选 `kc diff` 看同一对照。否决产品 CLI 另设 pack / status / plan；否决把 ChangeSet 文件当日常 CLI 操作数；否决最短旅程必须先 diff 再提交；否决面 3 当写面。

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。方向保留在 [路线](../../doc/roadmap.md) 的“未到”：直接改仓时的发布前检查。
