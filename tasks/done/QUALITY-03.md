# QUALITY-03 · Server 求值入口缺空 principal 拒绝守卫（权限梳理发现）

- [ ] 未清（2026-09-24 权限模型梳理）：`cli/allow.go` `PrincipalAllowed` 与 `cli/dataset_consume.go` 消费求值对 `as == ""` 直接放行——这是本地直连 Home 无 `--as` 的 owner 路径（无 Deployment 本地 Home 属另一入口，`doc/design/permissions.md` §7.3 配对合同）。判定：守卫缺口而非行为 bug——部署 Server 路径依赖上游认证保证 principal 非空，求值函数本身无防线；若未来 Server 入口漏带 principal，会静默变成全放行，与 §7.1「不存在 owner bypass」的实现前提只隔一层。处置（未排期）：为 Server typed 求值路径补「principal 为空即拒绝（本地 Home 路径除外）」守卫测试，必要时在 facade 装配层强制。触发条件=触碰认证装配（`cli/flags_identity.go` / `service_invoke.go`）或新增 Server 入口时顺手补齐；期间该双轨以本条为准，不写进设计文档（设计已按 §7.3 声明应然）。

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。转为 `DEBT-EMPTY-PRINCIPAL-01`，代码位置已从 `cli/` 移到 `invocation/`。
