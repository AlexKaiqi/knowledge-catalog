# CLEANUP-04 · 删除 connector 链，Interface 降为插件内部机制（2026-10-14）

- [x] 按用户决定删除 `connector/`、`integrationruntime/`、`cmd/kc-integration` 及唯一跨包旅程 `cli/integration_runtime_journey_test.go`；架构守卫只去掉这三个已删除包的条目与专属规则（其余包规则不变）；场景 `repository-attached` 去掉随之消失的 `collector.preview` 能力与证据；test-catalog 去掉 N 节。场景素材里的 Python 采集样例走普通 Writer，与本包无关，保留。
- 用户决定 Interface 降级：对外只保留“插件”概念，处理器合同是 LiveAccess 内部机制；文档已同步（concepts／vision／architecture／affordance／plugins／object／roadmap）。代码迁移 `interfaces/`→`liveaccess/` 未做，列为路线近期项。
- 耦合基线收紧：删包后传播成本 15.27%→14.95%；`client` 不稳定度 0.69→0.82 是其消费者被删的预期结果。显式 `-write-baseline`。
- 验证：make check-docs、make quality、make test 退出 0（`/tmp/kc-cleanup-quality.log`、`/tmp/kc-cleanup-test.log`），场景树三项与 `_tests`、check-surface 通过。未跑 test-contracts 与真实部署验收；未提交。
- 债：`kernel.ErrScopeDenied`（`SCOPE_DENIED`）已无生产者，仅 telemetry 词表与 `interfaces/dispatch_test.go` 引用；公开错误码，随错误码清理一并定去留。`.data/design/capability-organization.md` 是旧草稿，仍写 connector／knowledge 等旧形状；目录重组时删除或重写。
