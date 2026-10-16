# ZK-TRIAL-01 · Snapshot 与 Object 零知识试跑

## 目标

跑通隔离环境中的通用 Agent 试跑，取得陌生 Agent 认识系统、发布对象、消费文件交付及修订采用的真实记录。
只评价 Snapshot、Object 与公开帮助；结论区分试跑基础设施、任务完成和产品缺口。

## 依据

- [Affordance](../../doc/design/affordance.md)、[对象](../../doc/design/object.md)、[Snapshot](../../doc/design/snapshot.md)、[Dataset](../../doc/design/dataset.md)。
- [权限](../../doc/design/permissions.md)、[测试与验证](../../doc/engineering/testing.md)。

## 验收

- Agent 看不到源码、封存答案、其他身份凭证或模型密钥；KC 网关固定本次主体。
- 只用真实 lakeFS 的独立权威与当前源码二进制，保存环境、完整工具记录、最终回答与独立状态核对。
- 认识环境、接入方发布和独立消费方各有真实运行记录；修订与旧依据复核按实际完成范围报告。
- 相关回归、场景结构、文档和质量门禁通过，不把模型结束或命令退出零当作任务通过。

## 前置

复用现有试跑材料及已批准模型端点；用户目录 `.env` 的 `VOLCENGINE_API_KEY` 仅由运行侧读取。

## 不做

不验证索引、LiveAccess 或 MySQL，不调整在途插件重构，不提交代码，不重置已有部署和权威数据。
库存发现、Dataset 内搜索、外部取值与业务任务的零知识试跑由 `ZK-TRIAL-02` 承接。

## 交付与实际验证

已完成隔离世界、固定身份与模型凭证网关、可续问运行器、五组原话、证据与评审输出；
沿用 pi 1.2.0 和 DeepSeek 模型，没有另接 DSH。五个独立会话加一次概念续问已真实运行，
独立经 Server 核对首次交付、消费、同身份修订、旧规则保留和源仓权限边界。
报告及逐次记录位于仓库外 `/Users/kaiqidong/kc-zk-runs/20261015-snapshot-object/report.md`。

本轮回归 18 项、场景 Python 62 项、三项场景结构检查、脚本语法与 `make check-docs` 通过；
测试镜像构建时的定向 Object/Snapshot Go 测试与质量门禁通过。当前最终 `make quality` 因
并行索引/LiveAccess 接口重构无法编译，日志在同目录 `operator/final-quality.log`。
因此本任务保留 doing，不把固定镜像试跑完成写成当前工作区全仓验收通过。

首次供给完成但依赖提取二进制内嵌资料，不能宣称公开帮助已足以完成所有零知识供给。
修订中两次越范围 search 均在权限层被拒；历史对象由 operator 按旧提交核对，消费者自主复核的是旧交付文件。
发现的问题分别登记，不在本轮调整产品行为：

- `DEBT-OBJECT-WRITE-GUIDE-01`：目录发布缺输入样例。
- `DEBT-OBJECT-IDENTITY-PUBLICATION-01`：系统对象身份说明矛盾。
- `DEBT-DATASET-ROOT-RECIPE-01`：根映射发布空指针。

## 2026-10-15 接入方复测

按用户要求，使用 Docker 内 pi 1.2.0 与用户目录 `.env` 中的 Volcengine 凭据，显式选择该文件列出的
`glm-5.3`，重新构建当前源码并创建独立真实 lakeFS 世界。供给材料是表结构、十份 SQL 与口径说明，
共 12 个原始文件；没有给预制对象、仓库源码或封存答案。结果目录为
`/Users/kaiqidong/kc-zk-runs/20261015-184057-pi-supplier/`。

运行器新增可选 `--materials`，保留默认单份资料行为；目录输入拒绝符号链接与特殊文件，记录实际输入
的相对路径及 SHA256。先红后绿，24 项运行器回归、`make check-docs`、`make quality` 通过。

三次独立 pi 会话已结束：环境认识、接入方发布、消费方理解。接入方用 78 次工具调用、约 8 分钟发布
6 张表、2 项作业、10 份查询与 1 份口径，并发布 `qinghe-shared` 第 2 版、授予同事消费权限。
操作员经正式 Server，以同事身份读取全部 27 个文件及 19 个业务对象，核对十份 SQL 和口径正文，
最终草稿的 19 个对象正文与 Server 回读完全一致；同事直接读来源仓返回 `FORBIDDEN`。

业务交付可用不等于严格零知识验收通过：接入方使用二进制字符串取得 Schema 文件路径线索后再公开读取，
公开入口充分性仍未通过。消费方答对口径、表连接与 SQL 差异，但将声明可包含的订单状态说成实际汇总内容，
并把作业的 `DISABLE` 混称为表被禁用；没有数据库观察支持这些断言。两次搜索尝试也与其开头的过程描述不完全一致。
供给说明把全部查询统称为“口径下可执行”，完整事件 DDL 未保留，亦不能声称所有原文逐字一致。
这些模型产物和证据表述问题保存在独立内容审计、消费评审与每轮 `record.md`；公开输入说明问题补入
`DEBT-OBJECT-WRITE-GUIDE-01`，不通过人工教会命令来改写本轮结果。

本轮没有修改产品行为，没有执行数据库查询、索引/LiveAccess 验收、修订旅程或全量部署场景，未提交。
任务继续保留 doing，实际交付与严格验收结论分开记录。

## 2026-10-15 公开用例 Dataset 与帮助优化复验

按用户澄清，接入方合法学习材料是系统公开的用例 Dataset，来源 Repository 保持独立权限边界。
实际触碰 Affordance、对象和 Dataset 设计对应的帮助、系统指南、用例交付与对象读取范围；设计本身
未改。根帮助和发布旅程连接到公开 Dataset，目录帮助补完整头部样例、原值与描述输出的区别、
固定依据和回执回读。三个用例 Dataset 选择现有 Schema 文件，复用已有业务材料。

准备公开用例时，发现 Dataset 未选中的同名持有也参与冲突规划。已在既有 Reader 规划路径按所选
单元过滤，不新增公开接口；真实重复方面仍冲突，未选中对象不能被读取，部分选择仍保留完整性要求。
新增真实指南发布合同、30 个公开 Schema 的交付回归和读取边界回归；代码评审还补回范围外非法引用
的入口校验。运行器支持独立接入仓名，Agent 内 kc 可执行但不可读取二进制。

新世界使用真实 lakeFS、三个公开示例 Dataset、两个空接入仓；只给 pi 12 份原始材料和原任务。
pi 1.2.0 / Volcengine `glm-5.3` 在 2026-10-15 19:32–19:43（UTC+8）完成 74 次工具调用。
它经帮助、系统指南、Dataset 列表与公开原文件学到输入，自主提交、发布 `qinghe-shared` 第 1 版并授权
同事；没有二进制提取、身份切换或操作员中途教命令。受测镜像之后只同步了生成的 Schema 入口描述，
并补了内部规划器对非法范围外引用的校验；这两项不宣称由该模型会话覆盖。

独立操作员以同事身份固定依据下载 300 个文件，其中 105 个正文/Schema 文件、195 个派生定位文件；
101 次正文引用读取成功，4 次因两个重复 Schema 身份冲突失败。来源仓直接读取均被拒绝。
10 条 SQL 原文与 4 条 GMV 规则完整保留，未被公开例子的 UNPAID 口径污染。严格业务验收未通过：
两个 EVENT 未交付，DDL 的外键、非空、精度等信息丢失；币种与生效日没有原始依据；语义模型未完整
表达原口径；87 个旧定位项残留，抽查两个旧身份均以定位不符拒绝，未错误返回新身份。
操作员只审计，没有修正模型产物后冒充自主成功。

证据根为 `/Users/kaiqidong/kc-zk-runs/20261015-public-examples-pi-supplier/`，汇总见
`operator/assessment.json`、`operator/business-review.md`、`operator/public-entrypoint-audit.json`。
本轮定向指南、帮助、Schema 交付、选择范围回归、25 项运行器测试及 `make check-docs` 通过。
完整受影响包测试仍有索引声明迁移后的旧 access 断言、系统 Schema 数量和插件配置失败；索引验收
还缺 OpenSearch 配置。`make quality` 的代码静态项通过，但耦合守卫报 declarativeindex/cli 漂移及
传播成本 15.30% 超过 15.14%，没有调宽基线。未完成当前工作区全绿，任务继续 doing，未提交。
最终补测三项场景结构检查通过，仅证明声明一致；独立测试栈已停止，卷与原始证据保留。

新发现的失效定位项登记为 `DEBT-OBJECT-PATH-IDENTITY-01`；范围外旧包说明登记为
`DEBT-OBJECT-PACKAGE-EXAMPLES-01`。帮助修复进展归 `DEBT-OBJECT-WRITE-GUIDE-01`，
并行索引迁移的门禁问题归既有 `INDEX-DECLARATION-01` / `PUBLICATION-SYNC-01` 范围。

## 2026-10-15 按信息充分性收窄归因

用户明确只处理我们信息本身的问题，不为 Agent 在资料足够时的漏读、漏做或推断失误增加产品行为。
原始业务审计作为该次运行事实保留，不再把“模型没有完整完成业务整理”当作公开入口不足的证据。

逐条核对实际工具输出：原始 DDL、EVENT 与 SQL 可读；unit 可省略；effectiveFrom 虽是所选用例
Schema 的必填字段，但帮助已给自定义 Schema 和说明对象路径，Agent 本轮也确实创建了自己的
Schema，不能解释为平台要求猜生效日期。跨仓同方面冲突、子目录与单文件选择均已实际读到；
正确 Dataset 配方被 Agent 自己截短，属于未应用可用信息。身份自检有明确登录主体，不能用本地
目录名代替消费者身份。以上不再追加产品提示、业务抽取器或模型成功率优化。

确实需要修的是系统公开对象、地址、Repository 三篇概念中的仓限定身份旧说法。已与现有设计和
公开合同同步，补出版物守卫并验证 URN 示例不随来源仓变化；先红后绿，系统出版物全包及 Object
引用/holding 回归通过。旧定位残留仍是独立 Writer 一致性债：Agent 漏删不能免除成功提交保持
其它对象不变的责任，归 `DEBT-OBJECT-PATH-IDENTITY-01`，本次不扩展 Writer 行为。

本次 `make quality` 停在并行迁移的未使用 hasHints 测试辅助函数，未削弱检查或改索引代码。
信息归因记录见同一证据根的 `operator/information-attribution.json`；未重跑 pi 或启动服务，未提交。

本轮稳定候选门禁：`make check-docs`通过；`make quality`在 `declarativeindex/plugin/plugin_control_recovery_fixture_test.go` 的gofmt检查失败，归仍在途的 `INDEX-DECLARATION-01`，未替其它线格式化。证据 `.validation/runs/20261015T123258Z-42a42700fe5d/report.json`，`sourceChanged=false`。本任务继续doing；尚未达到统一门禁验收。

## 本轮最终统一门禁（2026-10-15）

统一证据与命令见 门禁索引（`.validation/runs/20261015T120241Z-task-orchestration/final-gates.json`）。最新 `make quality`（含 check-docs）通过；完整架构 648 pass/1 fail/0 skip（术语，归 DEBT-TERMINOLOGY-TRIAL-01）；`make test` 18 pass/2 fail/0 skip（同一 Dataset 文本能力场景及父级，归 INDEX）；`make test-contracts` 的 component 为 2349 pass/67 fail/6 skip，boundary/e2e 未启动。上述四次均源码稳定；合同失败分组见 报告（`.validation/runs/20261015T135236Z-03378168af4a/findings.md`），分别归 INDEX、库存消费债及回环 HTTP 资源调查，不由本任务替其它线修复。

最终相关定向结果及真实部署边界分别保留在任务专项证据与门禁索引：定向通过不替代完整入口，现有 local 栈 readyz 503，117 项真实验收均 error/not_started。全量门禁尚未全绿，依本轮约定保留 doing；未提交或安装依赖，未降低检查阈值或重置已有环境。

## 交付

2026-10-16 收口：已交付隔离试跑世界、固定主体与模型凭证网关、可续问运行器、原始任务材料、完整工具记录、
最终回答和独立 Server 状态核对。五个独立会话及一次概念续问覆盖认识环境、供给、独立消费、修订与旧交付复核；
后续接入方复测已从公开示例 Dataset 学习并自主发布，按信息充分性逐项区分产品缺口与模型遗漏。
真实记录的主报告副本（`.validation/runs/20261016-mvp-task-closeout/evidence/zk-trial-report.md`）和
后续归因副本（`.validation/runs/20261016-mvp-task-closeout/evidence/zk-information-attribution.json`）
按原字节保留，复制清单（`.validation/runs/20261016-mvp-task-closeout/evidence-copies.json`）登记原路径与 SHA-256。

已验证范围：真实 lakeFS 上独立核对首次交付、同身份修订、旧规则保留及来源仓权限边界；运行器、公开指南与
选择范围的相关回归、场景结构检查已有前文所列通过记录。试跑材料 35 份在完整合同来源 `374f90a4…` 与部署候选
`8667c2f7…` 之间相同；后续完整合同报告（`.validation/runs/20261016-mvp-unified-043602Z/gates/20261016T044027Z-d94f92fb680a/report.json`）
及质量门禁（`.validation/runs/20261016-mvp-priority-closeout/gates/20261016T045451Z-9e0b9c81b729/report.json`）
已补齐原在途门禁阻断，质量入口包含 `make check-docs`，运行源码稳定。

结论为试跑记录、状态证据和缺口归因已交付。业务整理遗漏、历史对象由操作员复核的范围及首次供给旧缺口均按
原记录保留；没有据单一模型旅程推断任意 Agent 成功率。搜索、LiveAccess、MySQL 和发现消费试跑归 `ZK-TRIAL-02`；
其它条件 live 未验范围继续单独登记。收口未重跑模型或部署、未安装依赖、未提交；任务移动及文档、质量复核统一执行。
