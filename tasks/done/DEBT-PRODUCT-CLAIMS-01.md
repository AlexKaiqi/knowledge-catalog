# DEBT-PRODUCT-CLAIMS-01 · 产品书能力承诺与实现、验收证据对齐

## 债

- 位置：[产品书](../../doc/product.html) 的对象、Agent、自动跟随、可观测与外部取值说明，以及 CLI 工具示例。
- 信号：2026-10-15 按当前工作区核对，正文将若干待实现能力描述为现有体验；部分可执行示例不符合当前公开合同；组件通过、历史记录和真实任务验收没有逐项区分。
- 判定：产品承诺与证据边界不一致，不是否定已有基础能力。相同漂移出现多处，需要可执行的产品示例合同，不能只靠人工维护状态文字。
- 处置：本条承接产品文字、示例和证据对照；已有实现缺口沿用下列任务，不重复立项，不通过收窄设计或放宽断言消除差距。
- 触发条件：再次向接入方发布产品书，或宣称产品书全部能力已验收前。

## 目标

接入方能明确知道哪些能力现在可用、成立的条件与验证边界；产品示例能按当前公开入口执行，尚未兑现的目标不冒充现有行为。

## 依据

- [产品书](../../doc/product.html)、[路线](../../doc/roadmap.md)。
- [测试与验证](../../doc/engineering/testing.md)、[工程质量闭环](../../doc/engineering/quality.md)。
- [Dataset](../../doc/design/dataset.md)、[发现](../../doc/design/discovery.md)、[LiveAccess](../../doc/design/liveaccess.md)。
- [对象](../../doc/design/object.md)、[Affordance](../../doc/design/affordance.md)、[服务](../../doc/design/service.md)。

## 审计发现

| 产品承诺 | 2026-10-15 核对结果与证据 | 实现缺口去向 |
|---|---|---|
| 来源更新后 Dataset 默认自动跟随 | 正文多次作为现有行为，状态表却标近期；来源推进自动出修订、失败保留现版仍待做。当前默认读取最新已发布修订，不等于自动发布新修订。 | `DATASET-REVISION-01` |
| 索引声明选进哪个 Dataset 就在哪生效 | 全文、过滤、关系和语义检索的基础实现已有；当前声明仍依赖 Schema access，新声明、能力清单及按修订检索空间未完成。语义查询测试存在，不等于真实任务的语义质量验收。 | `INDEX-DECLARATION-01`、`INDEX-SEARCH-SPACE-01` |
| 库存按当前身份列出可消费 Dataset | [库存入口](../../snapshot/service/inventory_http.go) 检查 inventory.read 后直接返回全部公开 Dataset 摘要，未按每份交付的消费权限筛选；登记可见不等于可消费。 | `DEBT-INVENTORY-CONSUMPTION-01` 实现，`FINAL-ACCEPTANCE-01` 验收 |
| 外部取值描述 identity，以及只有 shared 值进入共享索引 | [描述解析器](../../object/resource_descriptor.go) 的字段闭集尚不接受 identity；产品中的声明不能作为当前可执行输入。现有身份透传与外部取值不证明新 identity 规则已完成。 | `LIVEACCESS-DECLARATION-01`、`LIVEACCESS-READ-01`、`INDEX-SEARCH-SPACE-01` |
| 私有数据“数据不离开你的系统” | [LiveAccess 合同](../../liveaccess/README.md) 明确可靠保存实际采用的外部值或失败，支持源变化、离线与重启后的复核。可说原始数据无需整体迁入，不能承诺读取结果不进入平台。 | 本条修正文案与数据留存说明 |
| 每一次访问都有记录 | 初次核对发现文件回调被访问账白名单静默忽略、多仓静态对象漏记来源；后续修复已通过正式 HTTP 读取再按 trace/request 查询的回归，记录原始仓路径、固定 commit、采用来源及错误，可靠留证前不交付成功字节。语义合成文件保留来源快照，不伪造物理文件路径。完整验证范围见相应任务，不能外推所有访问形态已验收。 | `DEBT-FILE-ACCESS-EVIDENCE-01`、`DEBT-COMPOSED-ACCESS-EVIDENCE-01` |
| 任意陌生 Agent 仅靠公开入口就能用 | 真实试跑完成独立消费与修订，但首次供给依赖提取二进制内嵌资料；系统身份说明矛盾；合法根映射触发空指针。单一模型的单次旅程也不能证明任意 Agent 的可靠成功率。 | `ZK-TRIAL-01`、`DEBT-OBJECT-WRITE-GUIDE-01`、`DEBT-OBJECT-IDENTITY-PUBLICATION-01`、`DEBT-DATASET-ROOT-RECIPE-01` |
| Agent 自行安装 CLI、连接 MCP、安装 Skill 并完成任务 | 对象登记与文件交付已有；[示例旅程](../../cli/example_world_test.go) 检查 order-cli 文件字节，没有执行安装到业务查询全链。当前示例脚本没有 help 分支；无真实订单服务，不构成产品书所述订单查询验收。MCP 与 Skill 全链也未找到运行证据。 | 本条建立示例合同与逐形态证据，真实 Agent 效果另行验收 |
| 所有发布都校验，没有绕过检查的写法 | 对象 Writer 路径有校验；对象设计仍将底层直接提交的同等发布前检查标为未到。不能把 Writer 保证外推为底层仓所有写入都受该检查。 | 本条限定现有入口的保证；设计目标不变 |
| 无权方面明确标为无权 | [持有模型](../../object/holding.go) 的 withheld 当前列的是来源仓，不是逐方面名称；安全拒绝已有，定位粒度与文字不同。 | 本条准确描述返回粒度 |
| 大文件和大量文件都能承载 | 文件与分块读取已有；已有历史小阶梯实测，不是当前容量资格。最大文件、总量、历史深度、并发与恢复边界没有完整结论。 | 路线中的规模与生产资格；本条不外推容量 |

## 本次查到的验证证据

- 当前组件：Object、Writer、Reader、identity、snapshot/access、observability、hook 共七包重新执行通过；文件读取/帮助定向 27 个顶层测试通过，无 fail/skip；datasetfs 与 snapshot/console 两包通过。均不代替真实部署验收。
- 当前质量：本次完整 `make quality`（包含 `make check-docs`）通过；早先试跑日志中的在途重构编译失败不能当作当前状态。
- 历史场景：`.validation/runs/20261015T072728Z-cdbbb656aee5/report.json` 的 lakeFS 夹具场景通过、源码未在运行中变化；只执行两组场景入口，20 个测试事件，非全量合同，也不是当前源码的整体验收。
- 真实 Agent：`/Users/kaiqidong/kc-zk-runs/20261015-snapshot-object/report.md` 保存五个独立会话、一次概念续问及独立 Server 核对；使用固定镜像与真实 lakeFS，未部署搜索、LiveAccess 或业务 MySQL。旧对象由操作员复核，消费者自主复核的是旧交付文件。
- 规模：`.data/scale/runs/ladder-20260928T034638Z/report.json` 为 1/10/100 文件的小阶梯，报告明确不作资格判定。
- `FINAL-ACCEPTANCE-01` 仍待做；历史任务处于 done 可能是未验收关闭，不能仅凭所在目录判定通过。

## 验收

- 产品书逐项对照实现和证据，清楚区分已实现、待实现、组件验证、历史部署验证及真实任务验证；当前能力与状态表不互相矛盾。
- 产品示例形成自动执行合同：至少覆盖外部取值描述的公开解析/发布，以及工具说明、下载、help 和查询。测试执行实际示例并检查内容与版本，不只检查文字或命令退出成功。
- 尚未兑现的能力在产品书中明确标为待实现并指向所属任务，不冒充现有行为，也不删去承诺。
- 文件读取留证、根映射发布和公开说明的既有问题分别由原任务验收，未完成前保持明确边界。
- 需要真实模型、外部服务或规模环境的验证保存独立记录、候选源码/镜像、身份、输入、输出与判定；未执行项明确标未验证。
- 文档重读、`make check-docs`、`make quality` 通过；本条不以这些门禁代替产品行为验收。

## 前置

无。产品表述与证据边界核对、示例合同不依赖能力实现任务。

## 不做

不在审计中修改产品行为、不重置部署、不提交代码，不把本次定向测试写成整体验收。能力兑现后的产品旅程复核
归 `FINAL-ACCEPTANCE-01`；Dataset 搜索与消费说明随 `PUBLICATION-SYNC-01` 对齐。

## 早期复核（2026-10-15）

本轮只核对产品承诺、当前代码与证据，不修改 `doc/product.html`、公开代码或他人在途文件。
实际阅读依据为本任务列出的路线、测试与质量、Dataset、发现、LiveAccess、对象、Affordance 和服务。
完整的逐项状态、精确文案修改与最小自动合同方案保存在
`.validation/runs/20261015T120939Z-product-claims/audit.txt`，原产品书副本在该目录的 `baseline/`。

- 上方“审计发现”的 ResourceDescriptor 旧判断已过时：公开解析器已接受 `identity`，缺省为 `per-caller`；现有 Object、Writer、LiveAccess 对应定向回归通过。产品原样 JSON 仍返回 `USAGE_INVALID`，本轮确定原因是把信封 `object_id` 混入封闭的描述正文；仅去掉该字段后 `per-caller` 正常。证据：`resource-descriptor-probe.log`、`descriptor-components.log`。后续修正文案与从 HTML 提取原输入的发布合同，不放宽描述字段闭集。
- 库存缺口仍在：只授予一份 Dataset 的消费权、另保留无权 Dataset，两条公开列举入口仍都返回后者。证据目录 overlay 中的临时合同对 `/snapshot/v1/inventory` 与 `/snapshot/v1/datasets` 均红，见 `inventory-contract-red.log`；没有在工作区加入永久红测试。发现设计已明确要求按消费权列举，无需设计裁决；行为实现已独立登记到 `DEBT-INVENTORY-CONSUMPTION-01`，产品审计只核对交付和证据。
- 工具证据分开成立：现有公开例子与独立图片下载回归通过，证明固定文件交付和单独授权；直接执行示例脚本时，无来源的 `--help` 退出 3，有来源的 `--help` 退出 2，尚无成功帮助入口；进程内模拟订单服务上的 `get O-1042` 返回精确模拟结果。见 `product-existing-contracts.log`、`tool-probe.log`。这不证明真实订单系统、任意 Agent 安装、MCP 连接或 Skill 安装。
- 当前 `TestProductDocumentationUsesCurrentCommands` 只枚举 `.md`，未覆盖产品 HTML。后续扩展同一验证器读取 HTML 的 `pre/code`，不再复制一套命令表或验证器。工具全链合同应复用既有例子发布夹具，执行固定 revision 下载后的真实脚本、离线帮助和进程内查询；外部取值合同从 HTML 的实际输入出发解析、发布并固定版本回读。
- 产品正文的自动跟随、共享外部值索引、成员使用汇总仍分别等 `DATASET-REVISION-01`、`INDEX-SEARCH-SPACE-01`、`INDEX-USAGE-01`；LiveAccess 的身份声明、跨仓依赖、共用请求时限及精确信任范围已经有代码和组件证据，不能继续笼统写成“identity 尚未实现”，也不能由此声称部署全验收。其余精确文字修正包括外部值留存、Writer 保证的入口范围、无权来源仓粒度、容量资格和任意 Agent 成功率。
- 验证范围仅为上述定向回归、解析与进程内脚本探针；未跑全量门禁或部署、未占用 OpenSearch、未安装依赖、未提交。产品书在本轮前后与保存副本一致。实现任务和最终证据尚未齐备，本任务继续保留在 doing。

## 本轮包说明复核补充

位置：`object/README.md` 的 Binding/Schema origin、`object/reader/README.md` 的旧访问声明说明。信号：索引声明收尾阅读时仍出现已退役的 Schema 级取值来源描述。判定：属于本任务的产品表述漂移，不改变 LiveAccess 协议。处置：非搜索段已对齐公开 `ValueSource`、ResourceDescriptor 和 LiveAccess 固定声明解析合同，并同步 `object/reader/doc.go`、`liveaccess/state_lookup.go` 的同类注释。仍未改的 `object/reader/README.md` 消费/搜索/旧命令段（身份带来源 Repo、Schema text/filter/sort、binding show 与旧 --object 例子）交 `PUBLICATION-SYNC-01` 在主线完成后统一核对，不作为本轮交付。触发条件：`PUBLICATION-SYNC-01` 开工或本任务关闭前。

## 本轮最终统一门禁（2026-10-15）

统一证据与命令见 门禁索引（`.validation/runs/20261015T120241Z-task-orchestration/final-gates.json`）。最新 `make quality`（含 check-docs）通过；完整架构 648 pass/1 fail/0 skip（术语，归 DEBT-TERMINOLOGY-TRIAL-01）；`make test` 18 pass/2 fail/0 skip（同一 Dataset 文本能力场景及父级，归 INDEX）；`make test-contracts` 的 component 为 2349 pass/67 fail/6 skip，boundary/e2e 未启动。上述四次均源码稳定；合同失败分组见 报告（`.validation/runs/20261015T135236Z-03378168af4a/findings.md`），分别归 INDEX、库存消费债及回环 HTTP 资源调查，不由本任务替其它线修复。

最终相关定向结果及真实部署边界分别保留在任务专项证据与门禁索引：定向通过不替代完整入口，现有 local 栈 readyz 503，117 项真实验收均 error/not_started。全量门禁尚未全绿，依本轮约定保留 doing；未提交或安装依赖，未降低检查阈值或重置已有环境。


## 并入

`DEBT-PRODUCT-FLOW-01`（原位置为产品书的架构、首次使用及权限段和愿景插件概述）与本任务修改同一批产品文档，按债归并规则并入。其信号是五层图与 Dataset 文件底座错位、原始文件与对象发布路径混用、首次发布依赖隐含输入、角色与成功条件未衔接。既有文案、HTML 命令守卫和文档连续阅读修复保留；原始实现与验证见 `.validation/runs/20261016-product-flow/summary.json`。剩余验收包括新版完整质量门禁、工具固定修订下载/离线帮助/进程内查询合同，以及各能力所属任务完成后的产品旅程复核。Dataset 搜索与消费方式待主线完成后随 `PUBLICATION-SYNC-01` 对齐，不在本轮改动。

## 2026-10-16 Agent 入门补充

产品书增加“AI 友好”价值卡和独立入门章节：已有 Agent 从 CLI 帮助开始，沿库存、指南与对象说明理解内容，再按范围和版本读取；接入现有首次读取与发布指南。首次引导和可选 Skill 分开，Skill 用于复用已验证流程与团队约定，不作为首次使用前提，也不承诺任意 Agent 自动成功。文档目录同步阅读顺序，未修改 CLI 行为或安装 Skill。

一名未继承对话的 Agent 按约定只执行 8 次离线帮助读取，能说明用途、固定版本消费路径、后续帮助入口与环境前提。该结果只支持一次帮助可理解性观察；没有执行真实服务读取、业务处置或 Skill 安装，不替代零知识部署验收。

文档检查、HTML 链接/锚点/章节序号与 SVG 语法、受影响的 CLI 示例及帮助合同通过，无 fail/skip。`make quality` 再次在 `discovery/service/dataset_search.go` 的 709 行超过 700 行门禁处失败，沿用 `DEBT-DISCOVERY-SEARCH-SIZE-01`，未改该文件或门禁。证据见 本轮摘要（`.validation/runs/20261016-product-agent-entry/summary.json`）。本任务继续保留 doing。


## 2026-10-16 本轮实现与验证

实现范围：保留已有产品书架构、首次使用、愿景插件作用域及 HTML pre/code 同一验证器；完成 FLOW 债归并，补固定修订下载后的 demo 工具离线帮助、公开 start/查询全链合同，修正文档交接锚点并增加导航一致性守卫。对象及 Reader 非搜索说明、两处同源注释已对齐单元 ValueSource 与固定 ResourceDescriptor，未改协议形状、Dataset 搜索或消费方式。

已验证范围：原下载脚本离线 help 先红；交换查询接口、写错公开启动变量、破坏编号或锚点的反例均红，恢复后产品公开合同 137 pass、连续阅读合同 86 pass、最终 HTML 导航/命令合同 7 pass，均 0 fail / 0 skip。重读产品书与材料说明，两篇包说明全文及同源注释已复核；`make check-docs` 通过，稳定快照 `make quality` 仍被主线既有 709 行文件门禁阻断（`DEBT-DISCOVERY-SEARCH-SIZE-01`），任务保留 doing。全部路径、源码指纹、命令与红绿证据见 L4 验证索引（`.validation/runs/20261016T020346Z-l4-product-claims/index.md`）。

剩余范围：Dataset 搜索/消费、包消费/搜索/旧命令段及能力出版随 `PUBLICATION-SYNC-01` 和原实现任务对齐；真实部署、陌生 Agent、MCP 与 Skill 验收未执行。未安装依赖或提交，未占用共享索引或重置部署。

## 拆分（2026-10-16）

原验收含“在索引、身份规则、自动跟随的所属任务交付时，对应产品旅程以原预期验收”，等于整条路线走完才能关闭，
与 `FINAL-ACCEPTANCE-01` 重叠。该条及“并入”一节剩余验收中的“各能力所属任务完成后的产品旅程复核”转入
`FINAL-ACCEPTANCE-01` 的“产品书”一条；本任务只承接产品文字与证据边界修正、产品示例的可执行合同。

## 交付

2026-10-16 收口：产品书已区分能力目标、当前实现条件与已有验证范围，修正外部值留存、Writer 保证范围、
来源仓无权粒度、规模资格及 Agent 效果的表述；补齐首次使用交接、工具离线帮助、固定修订下载后的查询合同，
复用同一命令验证器检查 HTML，导航与反例守卫随改动保留。Dataset 搜索消费及 Reader 旧命令说明由
`PUBLICATION-SYNC-01` 同批完成，能力兑现后的产品旅程复核已明确转入 `FINAL-ACCEPTANCE-01`。

已验证范围：专项索引（`.validation/runs/20261016T020346Z-l4-product-claims/index.md`）保留原始输入、反例红绿及
产品公开合同 137 pass、连续阅读合同 86 pass、HTML 导航命令合同 7 pass，均无 fail / skip。
后续完整合同报告（`.validation/runs/20261016-mvp-unified-043602Z/gates/20261016T044027Z-d94f92fb680a/report.json`）
实际包含 HTML 命令、产品导航及交付工具离线帮助、缺来源拒绝、公开查询的通过事件；相关产品书、导航与工具
合同文件在来源 `374f90a4…` 与候选 `8667c2f7…` 之间相同。
质量门禁（`.validation/runs/20261016-mvp-priority-closeout/gates/20261016T045451Z-9e0b9c81b729/report.json`）
已通过，包含 `make check-docs`，不再受前文 709 行文件门禁阻断。

本次完成产品文字、证据边界和示例合同；真实业务服务、陌生 Agent 效果、MCP、Skill、规模资格及尚未实现能力的
整体验收继续保留原任务边界，未将其它条件 live skip 记作通过。收口未操作部署、安装依赖或提交；任务移动及
归档后的文档、质量复核统一执行。
