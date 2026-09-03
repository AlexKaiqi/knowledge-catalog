# TASK

执行接力棒，不是设计权威。一次交互只认领一条 `[ ]`。`[x]` 仅当对应契约全绿且无 skip/删断言。

权威映射见 `docs/README.md`（documentation-governance）。设计类文档合同标题由 `make check-docs` 强制。

## 用 ai-native-project-maintenance 梳理文档

- [x] 给 `class` 为 foundation / decision / runtime / evolution 的设计 Markdown 补齐 Goal / Non-Goals / 硬性约束 / 选定与否决 / 接口契约 五段（从现有正文抽出，不另造宪法），并用 `make check-docs` 强制这些标题。
- [x] 设计文档写应然：不得用当前实现收窄 Non-Goals；实现偏差点名到 owner 或 `MVP_ACCEPTANCE.md`。见 `docs/README.md` 应然/实然分层。
- [x] 对照设计应然，审计剩余文档问题与实现偏差；把仍缺的缺口写入 `MVP_ACCEPTANCE.md`，不在本回合改协议代码。
- [x] 按篇删除设计 Markdown 里可复制的协议 Schema、错误码表和命令穷尽清单，改为指向包 README、公开 Go API 与 Conformance；先从 `ASPECT_ACCESS.md`、`OBSERVABILITY.md`、`SERVICE_ARCHITECTURE.md` 开始。
- [ ] 在不搬迁目录树的前提下，把 `KNOWLEDGE_CATALOG_DESIGN.md` §9.2 / §9.4 的 ADR 与明确拒绝收成可引用的决策块，专题文档只 `refines`、不再复述另一套否决表。 *(进行中)*
- [ ] 把仍按「问题 / 第一性原理 / 决策」展开的专题正文接到文首五段之下：文首是合同，后文只保留调研证据和推导，删除与文首重复的原则编号。
