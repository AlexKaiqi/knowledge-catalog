# CATREG-01 · Catalog 成员合并与登记格式校验

- [x] 已完成（2026-10-10，Catalog 增量范围）：成员名单写入 `catalog.yaml`，Dataset 当前定义与不可变 release 保持独立文件；初始化、增删成员、发布/退役/归档、只读恢复与按 Dataset 审计已验证。
- Goal：按已证实的访问需求组织登记状态，恢复拒绝损坏或歧义的格式。Owner：[Dataset](../../doc/design/dataset.md) 的登记/发布与历史复核，[架构总览](../../doc/architecture.md) 的独立 Snapshot 权威，[服务边界](../../doc/design/service.md) 的显式初始化与恢复。Non-Goals：不改知识协议、授权语义、源仓历史、公开命令或跨仓事务；不去重 Dataset 当前定义与 release，不新增知识 Schema。
- 不变量：`L-01`、`V-01`、`W-02`、`KS-01`、`KS-02`；接口依据 `CatalogState`、`KnowledgeSet`、`KnowledgeSetSource`、`DatasetItem`、`Registry.Save` 与 Snapshot tree/CAS。
- 选定：新写出合并成员；旧布局兼容只读打开，明确变更时按新布局提交；结构与文件身份统一校验，持久 Items 如存在须与冻结 Sources 相符；保留退役/移除后的历史记录。否决：恢复自动推进 HEAD、混合两份成员权威、按旧历史授予当前权限、因格式调整覆盖不可变 release。
- 审查闭环：路径历史把无关 HEAD/父提交归为 Dataset 变更、后端页上限截断早期记录，均已先红后绿修复并升级为回归守卫（`registry_audit_test.go`，含 ChangeStore 与树比较两种路径）；YAML 浮点/时间/二进制类型隐式转换已先证伪再收紧解码，整数编码中转浮点丢精度也已修复并以 `2^53+1` 可恢复回归守卫。`catalog/log.go` 历史读取失败转空列表为存量错误传播缺口，本项不扩大公开签名；处置登记为债，触发条件为完善 Catalog 审计错误/分页合同。
- 已实现：新初始化明确写空成员列表，明确登记变更合并旧成员文件；只读恢复与幂等登记不迁移，不推进 HEAD。新增格式、文件身份/命名冲突、冻结 Sources/Items、current/release 一致性、退役/归档恢复与审计回归。
- 验证口径：共享工作区另有 System 出版物改动在途，整树检查曾被其编译/材料状态、文档关系环与旧 Schema 数量断言阻断；以基线 `2aa827f8` 加本项 13 个 Catalog 文件在 `/tmp/kc-catalog-validation-20261010` 隔离复跑，最终文件逐字节与工作区一致。最终 Catalog 全包、`make quality`（含文档）、`make test`、真实 `make deploy-local-scenes`、完整 `make test-contracts` 全绿，公开命令 60/60。System 场景断言同步后，当前共享工作区的 `make quality`、`make test`、真实 `make deploy-local-scenes` 与审计/发布恢复 CLI 定向合同也已绿；不把隔离完整合同扩称为 System 增量的完整合同验收。真实 DFS 20 个通过事件（含根）与两个走查专用节点按原入口不执行，本项未新增 skip，不把 DFS 结果扩称为那两个节点通过。最终证据：`/tmp/kc-catalog-final-package.log`、`/tmp/kc-catalog-final-quality.log`、`/tmp/kc-catalog-scenes.log`、`/tmp/kc-catalog-final-live.jsonl`、`/tmp/kc-catalog-shared-targeted.log`、`/tmp/kc-catalog-isolated-final-contracts.log`。

下列条目各自独立可认领。它们分别落在重构目标形态的哪一面、彼此的依赖顺序，以及「目标」与「现状」的逐条差距，见
重构目标形态与执行序归根本 TASK 的迁移记录（标注三态：已固化 / 已选定未闭环 / 待裁决），不裁决任何 `REVIEW-*`，也不新增 owner；原 REFACTOR_TOPOLOGY 与 PROVIDER_REFACTOR_GUIDE 已退出文档图。
