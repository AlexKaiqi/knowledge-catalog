# DOC-REWRITE-01 · 新建 doc/ 重写全部文档并退役 docs/（2026-10-14）

- [x] 按用户要求新建 `doc/`，按愿景重写，不设权威层级：README、vision、concepts、architecture、roadmap，`design/` 十篇（affordance、dataset、discovery、governance、liveaccess、object、permissions、plugins、service、snapshot），`engineering/` 四篇（quality、testing、test-catalog、architecture-invariants；后两篇由旧篇原样搬入）。场景 U 条目进入各设计篇“用例”一节。旧 `docs/`（含 `docs/graph/`、product.html）整体删除，开发阶段不留兼容说法。
- 已确认模型：KC 当前只做 discover 与 access；对象 URN 不感知 repo，object_id 为 `<namespace>/<name>`；同一 ID 跨仓视为同一对象的不同方面，同一范围同一方面冲突报错；用“同一对象”还是“关系”由建模者自选；插件声明在类型与 Schema 上，仓可关闭；Dataset 只管可见范围、版本固定与消费权限。
- 守卫改造：`scripts/check-docs` 改为检查 `doc/` 可达、“# ”标题、无 frontmatter、根目录 Markdown 仅 README／AGENTS／TASK、仓内链接有效；`scripts/docs-serve` 只放行 README 与 `doc/` 的 Markdown 和图片，入口 `/doc/README.md`；arch 分层／术语守卫、check-surface、maintenance、validation-inventory 夹具和 agent-signals owner 的目录名同步迁移，断言未削弱、无 skip。全仓引用（代码注释、包 README、`.data/scenes`、`.data/scale`、AGENTS、README、本文件）映射到新篇，去掉旧章节号引用。
- 耦合基线收紧：`scripts/check-docs` 不再依赖 object，不稳定度 1.00→0.00；显式 `go run ./scripts/coupling -depth=2 -write-baseline`，传播成本 15.36%→15.27%，其余为单元行数微调。
- 验证：make check-docs（19 篇）、make quality、make test 均退出 0（日志 `/tmp/kc-doc-rewrite-quality.log`、`/tmp/kc-doc-rewrite-test.log`）；另跑 scripts／internal/arch／systemrepo 定向 go test、maintenance_test、场景树三项检查与 `_tests`、perf_tree --check、check-surface、check-observability-signals、go vet。未跑 make test-contracts、make deploy-local-scenes；未提交、未推送。
- 登记的债（位置／信号／判定／处置／触发）：
  - `scripts/assemble-doc.sh` 默认输出仍为已退役的 `docs/KNOWLEDGE_CATALOG_DESIGN.md`；沿 CLEANUP-01 既有登记，确认仓外用途后退役或改输出。
  - `scripts/check-surface.sh` 与 maintenance_test 中的 `*REFACTOR.md` glob／夹具为旧文档残留；无害但无对象，下次触碰这两个脚本时删除。
  - 本文件历史条目仍有 `docs/graph`、`docs/reviewed` 纯文本提及；历史记录，不作当前合同，不追存量改写。
- 待用户裁决：httpsurface A／B／C、controlplane 去留、connector 是否迁出、object／plugins 篇中“待确认”项（多环境、Fork、插件默认范围与关闭粒度）。
