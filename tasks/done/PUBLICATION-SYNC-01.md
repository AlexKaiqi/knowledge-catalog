# PUBLICATION-SYNC-01 · 系统出版物跟上新设计

## 目标

系统仓 `systemrepo/content` 里的说明与实际行为一致：Schema 只描述结构；索引声明是选进 Dataset 的
`index` 方面；外部取值只在方面或成员上、引用取值描述；搜索只在 Dataset 内；插件按形状认领。

## 依据

- [Affordance](../../doc/design/affordance.md)、[插件](../../doc/design/plugins.md)、[发现](../../doc/design/discovery.md)、[LiveAccess](../../doc/design/liveaccess.md)。

## 验收

- 至少改完：`concepts/index-declaration/definition.md`、`concepts/schema/definition.md`、`readme.md`、
  `mechanisms/projection/definition.md`、`capabilities/search-object/usage.md`、
  `mechanisms/dynamic-access/definition.md`、`capabilities/resource-access/`、
  `guides/use-dynamic-values/guide.md`，以及随出版物发布的基础 Schema 中的 `access`。
- 出版物、二进制版本与 `kc help` 之间的自动检查通过。
- `make check-docs`、`make quality` 通过。

## 前置

`INDEX-SEARCH-SPACE-01`、`LIVEACCESS-READ-01`：
说明要和代码同一批改，先改说明会让 Agent 读到与行为不一致的内容。

## 不做

不重写与本轮设计无关的章节。

## 并入（2026-10-16）

`DEBT-OBJECT-PACKAGE-EXAMPLES-01` 并入本任务：都是让公开说明跟上新设计，一批改、一起核对。原债文本保留在 `done/`。

- 位置：`object/format/README.md` 的 `kc ingest --dir`；`object/writer/README.md` 的 Schema 发布和索引声明示例。
- 信号：2026-10-15 公开帮助走查发现，前者指向已不在公开命令表中的 ingest；后者仍用 `writer put --object`、Schema 顶层 origin 与字段 access。
- 处置：按所属公开命令改写包说明，索引与取值采用各自声明，补可执行示例验证。
- 追加验收：说明引用的发布命令可实际执行，Schema 不混入索引与取值字段；对应测试通过。
- 追加不做：不恢复旧命令，不改变对象协议或缩减索引、取值能力。

## 本轮实现（2026-10-16）

- 随 Dataset 检索实现同步系统出版物：从库存发现、选择 Dataset 修订、读取声明能力、搜索与同版本回读；Schema 只提供结构，检索能力来自选入的声明成员，取值描述由单元引用。投影、关系、动态读取及初次使用指南一并校对。
- 更新公开包 README 的 Writer 示例，删除失效的 ingest 与 Schema access 写法；`TestWriterPackageExamplesPublishStructureAndIndexSeparately` 从说明提取实际命令，经正式 Server 发布并回读 Schema 与索引成员。
- 更新出版依据并重新生成 CLI entrypoints；系统仓自动守卫拒绝旧仓级搜索、Schema access 和隐式采用声明的说明，正反例保留。
- 增量复核发现投影说明及插件帮助错误承诺 Dataset 自动恢复，Schema 能力和 Reader 包说明仍有字段访问声明旧概念。已改为当前的显式同步恢复、结构与独立索引声明，并补出版物/插件帮助反例守卫；新守卫先红后绿，证据 `/tmp/kc-publication-limits-{red,green}.log`。共享与按调用者观察的成员隔离尚未就绪，公开说明明确当前准备限制，不提前宣称完整任务完成。
- 已有定向证据：系统出版物测试、可执行包示例、架构边界及说明生成通过；不将这些结果写成真实部署或整项验收通过。最终统一门禁和真实 Dataset 发现/搜索/固定修订回读待本轮源码冻结后执行。未提交。

- 统一契约复核同时修正 `cli/README.md` 中仍要求 `object.search` 与按仓正文屏蔽的旧说明：Dataset 消费按 `file.read` 准入、同修订回读，来源仓直读仍独立授权。最终门禁随下一次冻结源码统一执行。

## MVP 说明收尾（2026-10-16，待统一验证）

- 当前源码的 typed 守卫已经要求 `DatasetSearchExecutor`，I-21 已引用实际存在的通知退役与 Dataset 刷新测试，
  检索帮助也已移除运行时业务材料示例；历史三项失败不再作为尚未实现事项，最终以同候选门禁为准。
- Reader 包说明仍有来源仓进入对象身份、旧分列 read、按仓 relations 及 Schema 字段 access 的残留。
  已按对象、发现、Affordance 与 LiveAccess 的现有合同同步，补同修订 Dataset 回读示例；仍有效的
  `binding show`、provenance/log/resolve 入口按公开代码保留。修改后已重读该篇。
- 复用公开说明命令守卫检查 Reader 指南，增加旧 read/relations 反例；typed 执行器反例补跨文件搜索委派、
  未调用 helper、外国 import 与遮蔽，保留实际执行而非仅构造执行器的检查。
- 本轮未逐条运行验证，改动完成后由统一入口覆盖 CLI 文档命令合同、完整架构、`make check-docs` 与
  `make quality`。本节只记录实现范围，未达到该任务完整验收，保持 doing。

## 交付

2026-10-16 收口：系统出版物、生成的命令入口及包说明已按当前公开合同同步；Schema 只描述结构，
索引声明选入 Dataset 后生效，读取与检索保留修订依据。Writer 包示例使用正式发布入口；Reader 指南
修正仓限定身份、分列 read、按仓 relations 和 Schema access 旧说明，并保留仍有效的声明诊断入口。

已验证范围：旧稳定来源的完整合同报告（`.validation/runs/20261016-mvp-unified-043602Z/gates/20261016T044027Z-d94f92fb680a/report.json`）
包含系统出版物一致性、导航、退役说明守卫及可执行 Writer 包示例的通过记录。
最新 Reader 指南、CLI 命令守卫及 typed 执行器委派由63 项专项报告（`.validation/runs/20261016-mvp-priority-closeout/gates/20261016T045451Z-a5016df50e0b/report.json`）
实际验证，0 fail / 0 skip；相关三份文件与部署候选 `8667c2f7…` 字节相同。
同批质量门禁（`.validation/runs/20261016-mvp-priority-closeout/gates/20261016T045451Z-9e0b9c81b729/report.json`）
通过，包含 `make check-docs`，各次运行源码稳定；前文旧门禁阻断不再作为本任务未完成项。

本任务交付说明同步与自动合同。完整 INDEX 能力、陌生 Agent 的发现消费旅程及整体验收分别由原实现任务、
`ZK-TRIAL-02` 与 `FINAL-ACCEPTANCE-01` 承接；完整合同中的其它条件 live skip 不记为本任务验证通过。
未提交。任务移动与归档后的文档、质量门禁由统一收口执行。
