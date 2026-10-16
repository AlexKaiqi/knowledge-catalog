# OBJECT-INTERFACES-01 · 对象能力扩展与字段索引声明（2026-10-10，本轮继续认领）

- [ ] 在既有知识身份、Aspect/Member、Schema、Relation 和 Writer 合同上，实现可独立注册的对象 Interface、LiveAccess 以及唯一的 Schema 字段索引声明，同步 owner 设计与系统出版物，受影响测试及契约全绿后验收。
- Goal：对象按声明实现共同能力，消费者经系统固定版本解析和分派；查询声明统一写在 Schema 字段上，结构与查询合同分别校验，并沿用声明 → 校验/能力匹配 → 执行/同依据回读的接缝。Owner：[对象 Interface](../../doc/design/affordance.md)、[知识读写](../../doc/design/object.md)、[资源访问](../../doc/design/liveaccess.md)、[声明式索引](../../doc/design/discovery.md)、[术语](../../doc/concepts.md)。
- 本轮裁决：仅保留字段 access，撤回独立索引对象、第二写法和覆盖/兼容规则；移除对应解析、定位与公开 API，system 查询声明移回字段。结构不变而 access 收缩或移除应允许发布，旧 pin 按旧字段声明解释；AccessSpec 变化仍由索引流程单独判断。不影响对象 Interface 与 LiveAccess，不改物理配置、授权、写代数和无关前置工作区修改。
- Non-Goals：不新增写入代数、跨仓事务、源系统客户端或默认授权；不把运行观察当 Snapshot；不要求所有对象拥有统一 definition 字段；不新增通用资源描述符、流处理能力或物理索引配置；不改动无关的已有工作区修改，不提交。
- 不变量：沿用 I-01（身份）、V-01（固定依据）、REL-01（关系来源）；对象能力遵循新 owner 的 OI-01～05（身份独立、固定与有界分派、授权与实际依据、独立扩展、实际依赖维护），索引遵循其 owner 的字段声明唯一来源、类型语义与历史固定解释约束；知识版本、运行观察与处理实现依据分别保留。
- 选定：普通对象/Aspect/Member 承载能力声明，固定版本解析，注册处理器按合同和实现结构分派；LiveAccess 保留旧 State/ResourceDescriptor 解释；Schema 字段 access 统一编译到 AccessSpec，结构兼容性和查询影响分别判断并保持旧 pin 语义。否决：独立索引对象、第二种索引写法、覆盖和回退规则、核心业务类型 switch、隐式全仓扫描、向只读 system 仓动态追加扩展、Schema 副本跨仓隐式解析、公开读取挂 Hook、借索引声明扩大授权。
- 接口落点：knowledge 的公开对象能力/索引声明类型，object/reader 的固定版本解析，liveaccess 的处理器接缝，retrieval.AccessSpec 与 index 的能力兑现；精确公开形状、错误和一致性证据归包 README/Go API/Conformance，不写入设计 Markdown。
- 实现范围：普通 Interface 合同与命名实现 Member、固定版本解析、独立处理器注册、Schema 校验及授权；首个内置 LiveAccess 与历史 ResourceDescriptor 解释，公共 CLI/HTTP 调用保留持久访问与审计依据；字段查询声明经 Writer 发布与有界 Schema 定位编译到 AccessSpec，查询能力变更驱动重建且旧 pin 保持原解释。system 出版物、导航、Schema、查询声明与相关 owner/文档图同步；移除独立索引 Schema、对象、定位元数据与 API。
- 增量质量闭环：已修正文档对 required/additionalProperties 的过时限制；回调可变值隔离、共享注册表并发装配、调用证据失败关闭均有回归。字段 access 收缩不再误判结构破坏，字段删除、类型改变与其它值结构保护保留；普通 discovery/projection/* 对象不能改变查询合同。审查发现处理器匹配混入查询声明摘要，分开结构支持摘要与完整知识依据并增加回归。系统 Schema 数量/身份与材料漂移继续由自动守卫保护，没有削弱原断言。
- 验证结果：字段查询变化重建、正文增量、能力移除、旧 pin 解释、值结构保护和 Interface 结构分派回归通过，受影响核心包与公开调用的 race 回归通过。固定源码副本（指纹 `45d532f498283d005233601ddb677e0f9e5618f0eff8a6839df4f2cf94021372`）的 `make quality` 通过；`make test` 运行 `20261010T091328Z-1acd8a633186` 为 passed，21 个测试终态事件，fail / skip 均为 0；`make test-contracts` 运行 `20261010T091328Z-6d5dc878d218` 为 passed-with-skips，1,963 个通过事件、fail 0、既有外部 live skip 20，60/60 公开命令满足成功及风险边界要求。两次正式运行均退出码 0、`sourceChanged=false`；完整混合契约不写成无 skip 或真实部署验收。
- 同源证据边界：运行期间其它任务更新了 11 个文件，包含 TASK、CLI 旧命令夹具、验证说明、Git 内部工具及维护守卫；本轮知识、Interface、索引实现和 system/owner 文件与固定副本逐字节一致。当前工作区另补跑受影响定向回归和 `make quality` 通过，记录回写后再次复核文档与质量门禁；固定副本的完整契约不外推为并行改动后整树同源验收。证据保留在 `.validation/schema-field-frozen-copy.json`、`.validation/schema-field-current-differences.json`、相应 runs 与日志中。
- 过程失败保留：原工作区完整契约运行 `20261010T084731Z-027ea6dc0caf` 的断言虽无失败，但并行修改触发 `source-changed`，整体退出码 2，不计通过；改用与当时工作区逐字节一致的固定副本完成上述正式复跑。前一独立索引对象方案的验证记录不作为本轮裁决的验收证据。
- 验收边界：本轮验证本地夹具、真实 OpenSearch、组件/架构和公开 CLI 契约；真实 lakeFS/Gitea/运行时/身份服务/模型的部署验收为独立入口。保留本项 `[ ]`，不将带 skip 的混合套件记为本文件要求的全绿且无 skip 完整验收，也不以本轮结果覆盖既有部署待办。


**现在的位置：基础协议与可恢复的服务骨架已经建立；普通用户自助修复已有历史功能验证记录。** 统一用户名、持久登录、同名托管供给、管理地址、自有 Gitea 连接、首次准入与受限分享已实现，既有范围由 WALK-01 记录；不把这些记录当成本轮重新验收。动态 State 部署、真实任务收益与规模资格仍是独立待办。

| 阶段 | 已具备的能力 | 当前状态与下一道门槛 |
|---|---|---|
| M1 基础协议与固定版本消费 | 稳定身份、Writer 单仓发布、CAS/幂等、Schema、来源、Dataset、固定任务依据、检索后同版本回读、权限交付链、typed Client | **已交付参考实现**；继续由共享合同和正式 CLI/HTTP 旅程回归保护 |
| M2 可恢复单实例与仓分配机制 | 独立 Catalog Snapshot 权威、耐久状态与可丢缓存、已有源只读 attach、服务凭证创建托管仓、创建者策略授权、实例替换后续用 | **已补齐同名用户供给**；Gitea/LakeFS 托管供给与管理地址进入耐久恢复路径，正式验收见 WALK-01 |
| M3 普通用户全程自助 | 获权后可发布、修改、评审、移除、复核和采用更新；单文件手册已有完整内容 | **已有修复与验证记录**；SELF-01～04 的既有范围由 WALK-01 汇总，真实外部身份服务与更广部署资格单独记录；手册与价值路径归 GUIDE-01 |
| M4 动态 State 完整部署 | State 精确 READ、独立动态投影、声明/观察双 basis、notice-and-pull、同版本 hydrate 已有 | **局部能力已有，可信消费闭环待补齐**；DOC-20 收口设计与用例，DYN-01 承接真实 Observer/runtime/lakeFS/KC/OpenSearch 与重启恢复；默认时效待 REVIEW-04 |
| M5 数据规模与生产资格 | 规模档位、压测场景树用例和基础输入生成器已有；原生点读、增量机制与部分遥测已有；LAKEFS-04 bulk-ingest 写通路已实现并通过合同/回归/闸层 | **尚无有效容量结论**；SCALE-01 补执行器、小规模基线、逐档扩量、历史与恢复证据（含 staging/bulk 双路径对照），DOC-15 补观测，未来 native provider 的能力隔离沿现行 Provider 合同；代际方案待 REVIEW-01 |

验证贯穿每个阶段，不集中到最后。M1/M2 的“已交付”保留既有实现与历史验收结论；并不表示本次整理重新跑过完整产品套件。

DOC-20 的既有 State 消费设计与用例见其完成记录；剩余缺口包括范围级覆盖/时效、
后台恢复、观察保留、旧 Dataset 声明维护与源授权。DYN-01 承接 lakeFS 上的部署证据；单 key
增量成本纳入 SCALE-01，不能以 lookup 次数代替整个维护路径成本。Stream 窗口与持续订阅后续
分别推进；自助产品已有结果由 WALK-01 保留，手册与其它规模任务仍按原条目跟踪。

产品缺口按 [MVP 的优先完善表](../../doc/engineering/testing.md) 收口：服务版采用、可信动态
消费、真实任务效果与修订循环、规模与恢复资格。本轮 GUIDE-01 对齐文档与现行入口；
之后各项仍沿既有 owner 与验收合同推进，不因价值表述更新改变当前容量或部署承诺。

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。Interface 已退役为 LiveAccess 内部机制（`e79e3fa`），字段索引声明改为 `index` 方面（`INDEX-DECLARATION-01`）。原范围不再继续。
