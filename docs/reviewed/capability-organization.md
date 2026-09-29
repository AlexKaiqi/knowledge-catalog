# 能力边界组织设计

本设计以能力边界为组织单元重排仓库目录：一个顶层目录 = 一个能力 = 一个概念；对外最小信息（README +
interface）先行，内部实现按三档隐藏。承接 [核心架构](core-architecture.md)（四协议边界与层所有权）、
[工程质量闭环](quality-loop.md)（闸/审分层与 coupling 棘轮）、[基础重构验收合同](refactor-acceptance.md)（红绿执行纪律）。
本设计只动组织，不动行为；所有不变量 ID 沿用 [架构不变量](architecture-invariants.md) 现有编号，不新造编号。

## Goal

以能力边界作为仓库的组织单元：**一个顶层目录 = 一个能力 = 一个概念**。每个能力以
对外最小信息（README + interface）先行，内部实现语言级隐藏。四个协议边界
（Snapshot / Catalog / Knowledge / Retrieval）保持公开形状不动，只做「内部实现完全不暴露」的收尾；
组织优化的主战场是应用层的约 20 个单元。

## Non-Goals

- 不改四个协议边界的公开形状（`snapshot/store.go`、`catalog/README.md`、`knowledge/README.md`、
  `retrieval/README.md` 是已选定合同）；公开消费面不新增对象列举、不新增 PATCH / 跨仓事务。
- 不新增协议层或编号体系；应用能力负责装配与政策执行，不是第五个协议边界。
- 不删除、削弱 conformance / arch 守卫断言；`internal/arch` 层表变更属协议决策，必须同变更改
  `core-architecture.md`。
- 不做 big-bang 重命名；一次迁移一个能力，每步可逆、全绿。
- 不移动非 Go 能力区域（`webui/`、`dsh-plugin/`、`output/`、`docs/graph/`、`.data/`）。
- 不处理 Dataset 消费语义、VFS 能力扩展等产品行为演进（各自 owner 所有）。

## 1. 能力定义与资格

### 1.1 能力的四件套

| # | 构成 | 形式 | 防什么 |
|---|---|---|---|
| 1 | README（人读的最小信息） | 边界声明 + 最小用法 + 依赖方向 + 非目标 | 使用者不知道该知道什么 |
| 2 | interface（编译器可见的最小信息） | 根包导出 API，集中在少数具名文件；窄口用 port 类型 | 契约面发散 |
| 3 | internal（实现隐藏） | 隐藏分三档，按需就位：a. 包内 unexported（单包能力即已隐藏）→ b. `internal/` 语言级强制 → c. `internal/arch` denylist 图级双保险 | 实现细节外泄 |
| 4 | 验证（固化） | conformance（行为）+ arch 层表与 denylist（方向）+ 导出面闭集锚点（形状） | 边界静默漂移 |

README 与 interface 是「外部使用该能力的最小必要信息」；四件套整体才是能力边界——
只有 1+2 是文档，加上 3+4 才是边界。

### 1.2 资格三问（准入与除名）

1. 有独立概念名吗？（`delivery` 是能力，`utils/common/shared` 永远不是。）
2. 有 ≥2 个消费方，或独享一个决策吗？（`delivery` 单消费方但独享 AUTH-03，合格。）
3. README 的「不拥有什么」一节写得出来吗？

三问全过 → 顶层能力目录。第三问答不出 → 降级为相邻能力 `internal/` 或合并；
**缺 README 不是文档欠账，是边界未定的信号，处置是降级或合并，不是补一篇文字交差。**

### 1.3 README 模板（能力的身份证）

~~~~markdown
# <capability>/
<一句话：这个能力拥有什么>

## 边界
拥有：…
不拥有：…（以及为什么归属别处）

## 最小用法
```go
// 可编译的最小例子；这段代码同时是契约面的一部分
```

## 依赖方向
可以依赖：…        # 与 internal/arch 层表 allowlist 一致
禁止 import：…     # 与 internal/arch denylist 一致

## 验证
Conformance: Test…；arch guard: <layer>/<denylist 项>；coupling unit: <name>
~~~~

「依赖方向」一节是 `internal/arch` 规则的人读镜像，两边必须说同一句话。

### 1.4 契约面判定

外部使用者的最小必要信息才留在根包根文件；判断标准是「README 最小用法里出现的名字」。
声明式契约优先（闭集函数如 `cli/surface.go`、`httpsurface.Patterns()`），加一个导出符号就是扩契约。

## 2. 目标目录组织

### 2.1 顶层保持扁平，一层能力 = 一个顶层目录

分组靠 `internal/arch` 层表（graph 级），不做物理嵌套。理由：
a. 依赖不是树形的，物理分组假装它是；b. `scripts/coupling` 以顶层目录为 unit，
「一个能力 = 一个耦合 unit」让棘轮正好对齐；c. Go 对跨组 import 无语言约束，`capabilities/`
这类分组目录只会改坏全部 import path 而不增加任何强制。

### 2.2 重构目标全树（逐能力展开；①README ②契约文件 ③隐藏 ④验证锚点）

```text
kc/
│
│   ━━━ 协议环:四个协议边界公开形状不动,只收尾「内部完全不暴露」 ━━━
│
├── kernel/                 能力:坐标与错误底座 —— 全部即契约,无 internal
│
├── snapshot/               能力⓪:不可变版本与文件树(变化识别是一等能力)
│   ├── README.md                       # ① 拥有版本/条件推进/求差;不拥有知识解释
│   ├── store.go                        # ② Repository 口(唯一协议面)
│   ├── receipt.go                      # ② 协议回执类型(Phase 2 新增,替换 treewriter.Receipt 外泄)
│   ├── conformance_test.go             # ④ RepositoryContract:各介质同一行为合同
│   ├── lakefs/  gitea/                 # ② 注册薄壳:仅 import ③实现体(A-01:只许装配根引)
│   └── internal/
│       └── lakefsimpl/  giteaimpl/  treewriter/  commandlog/  stamp/
│
├── catalog/                能力①:登记与组合
│   ├── README.md                       # ① 拥有登记/Dataset 发布;不拥有 object_id/Aspect
│   ├── catalog.go  dataset.go          # ② 登记口与 ResolvedKnowledgeSet 冻结(KS-01/K-10)
│   ├── conformance_test.go             # ④ 依赖集闭集(现有焦点测试归位)
│   └── internal/worktree/              # ③ 宿主检出实现
│
├── knowledge/              能力②:身份与读写解释
│   ├── README.md                       # ① 拥有身份/Schema/来源/治理;不拥有组合与检索
│   ├── writer/  reader/                # ② 写面(CAS/幂等)与读面(同版本 hydrate 窄口)——本身就是契约
│   ├── serving.go                      # ② 消费组合窄口(注入状态端口,CA-01)
│   ├── conformance_test.go             # ④ I-01/V-01/W-01/W-02/C-01/S-01 反例组
│   └── internal/
│       └── unitcodec/  semanticview/  maintenance/   # ③ 编码/视图/维护扫描(仅显式入口可达)
│
├── retrieval/              能力③合同:检索合同(薄,纯契约)
│   ├── README.md                       # ① 拥有查询计划/候选页/回读纪律;不拥有投影实现
│   ├── search.go  relations.go  view.go   # ② SearchRequest/SearchView/关系合同/页界(IX-01)
│   ├── conformance_test.go             # ④ R-01/R-02/K-25 反例组
│   └── (无 internal;禁 import index —— denylist 钉死)
│
├── index/                  能力③投影:派生投影(读面+写面,可丢派生状态)
│   ├── README.md                       # ① 拥有 READY/对账/代际/物理拓扑;不拥有声明本身/Canonical
│   ├── retriever.go                    # ② 读面:exact-basis 候选源 + READY(R-01/R-02)
│   ├── maintainer.go                   # ② 写面:basis/coverage/lag、代际重建、增量、消费者对账(PC-01/IX-03/04)
│   ├── conformance_test.go             # ④ P-01/K-19/PC-01/IX-02..05 反例组
│   ├── internal/
│   │   ├── planner/                    # ③ 计划器 + 声明编译器(S-01 声明的执行侧)
│   │   ├── consumer/                   # ③ 消费者耐久进度与对账(PC-01)
│   │   └── lanes/                      # ③ 静态 snapshot 基准 / 动态 observation 两条 lane
│   └── opensearch/                     # ② provider 注册薄壳(实现体进 internal/)
│
│
│   ━━━ 应用环:本轮组织重构的主对象 ━━━
│
├── knowledgeapp/           应用能力:应用用例(API-01 唯一业务规则所在层)
│   ├── README.md                       # ① 全部 executor 如实入册:纵切 8 + 治理 5 + 投影 3
│   ├── read.go  search.go  write.go    # ② 纵切 executor 与请求/响应类型(契约面)
│   ├── operations.go  governance.go    # ② 治理 executor(PROPOSAL→Merge 经应用核心)
│   ├── dataset_*.go  traverse.go       # ② Dataset 纵切与邻域闭包
│   ├── surface_anchor_test.go          # ④ 导出面闭集锚点(仿 cli/surface.go;Phase 0 新建)
│   └── (单包;internal/ 为 Phase 3 后评估项——59 个 unexported 已压住实现)
├── home/                   应用能力:部署装配
│   ├── README.md                       # ① 拥有装配/恢复;不拥有业务规则与 provider 选择权(A-01)
│   ├── deployment.go  seams.go         # ② DeploymentConfig/OpenDeployment + 各协议口装配缝签名
│   └── internal/authority_drivers/     # ③ 唯一具体 adapter import 点(A-01)
│
├── server/                 应用能力:服务接入面(新建,Phase 1)
│   ├── README.md                       # ① 拥有 transport/授权强制/遥测;不拥有业务规则(API-01)
│   ├── routes.go                       # ② method+pattern 闭集,与 httpsurface.Patterns 对账
│   ├── routes_anchor_test.go           # ④ 先立闭集再搬实现
│   └── internal/
│       ├── handlers/  auth/            # ③ typed handler 与授权强制
│       └── telemetry/  readiness/      # ③ 观察注入;就绪探测经装配缝,不 import provider
│
├── cli/                    应用能力:客户端面
│   ├── README.md                       # ① 拥有 argv/输出/会话粘合;不拥有业务规则
│   ├── surface.go                      # ② argv 闭集(60 锚点,已有)
│   ├── surface_anchor_test.go          # ④ 已有
│   ├── invocation.go                   # ② 工作区载体 invocation/command/handler(R1 回迁,transport 侧唯一 home 持有点)
│   └── (单包;serve/service_routes/handlers 迁 server/ —— Phase 1;product 输出 unexported)
│
├── invoke/                 应用能力:transport 无关调用词汇表 + 调用观察
│   ├── README.md                       # ① (R1 已补)transport 中立性即存在意义
│   ├── flags.go  json.go  run_result.go  # ② FlagValue 读取器/JSONOut/RunResult
│   ├── http.go  http_write.go  observation.go  # ② WriteJSON/WriteInvoke/观察口
│   ├── invocation.go                   # ② Stage 三档 + BindingScope(纯词汇)
│   └── (单包;Phase 0 落 denylist ↛{cli,httpsurface,client,home}——R1 起四者全可钉)
├── invocation/             应用缝:执行载体+动词操作体(B3 新增;B5 载体/B6 动词体;两 transport 共用,互不 import 的第三方家)
│   ├── README.md                       # ① 持句柄不开句柄;B6 起动词操作体(Verb*)全体在此
│   ├── invocation.go                   # ② Invocation 载体 + Command/Handler
│   └── verbs_*.go  allow.go  stores_observe.go …  # ② cli 动词体 B6 迁入(读/写/索引/资源/目录/观测/授权)

├── client/                 应用能力:调用方身份/凭证/会话
│   ├── README.md                       # ① 三概念:Identity/Authentication/Session
│   ├── identity.go  auth.go  session.go   # ② 三概念分文件(现混排,拆文件不拆包)
│   └── (单包;Phase 0 落 denylist ↛{cli,httpsurface,home};89 导出 Phase 1 复核收窄)
├── controlplane/           应用能力:维护闭环(PROPOSAL→Preview→validate→Merge)
│   ├── README.md                       # ① 拥有闭环编排;不拥有新写面/闸规则(K-07/08/09)
│   ├── proposal.go  preview.go         # ② 操作语义:candidate 提案与固定 Preview
│   ├── validation.go  merge.go         # ② validate 绑定与 merge(basis/evidence/CAS)
│   └── (单包;状态文件走 internal/journal;Phase 0 落 denylist ↛{cli,httpsurface,client,home})
├── gate/                   应用能力:合并证据闸(fail-closed)
│   ├── README.md                       # ① 拥有跃迁前证据检查;不拥有知识/不拨用户系统/不是 hook
│   ├── gate.go  rules.go               # ② 规则与证据绑定同一 previewId;兼容路径语义
│   └── (单包,现仅依赖 kernel+internal/jsonfile;Phase 0 落最严 denylist)
├── connector/              应用能力:外部接入对账合同
│   ├── README.md                       # ① 拥有对账;不连源/不拥有 Writer;Observer/Resource Access 不在此
│   ├── preview.go                      # ② Preview(patch/reconcile 两模式)→ChangeSet
│   └── (单包;已有 denylist)
├── integrationruntime/     应用能力:接入运行宿主
│   ├── README.md                       # ① typed API 即契约;源协议与 Address 映射在接入方 integration repo
│   ├── runtime.go                      # ② build/activate/run/status
│   └── (单包;已有 denylist ↛{home,写面,具体介质})
├── hook/                   应用能力:出站通知
│   ├── README.md                       # ① 拥有 action×phase 绑定;pre fail-closed;post 只发指针进 outbox
│   ├── binding.go  outbox.go           # ② 绑定语义与重试投递(现混排,目标按此归并)
│   └── (单包;已有 denylist ↛{catalog,写面,reader,index,maintenance 面,介质})
├── delivery/               应用能力:交付链
│   ├── README.md                       # ① 拥有可见正文改写;不定位/不索引/不装配 Canonical/不读 allow.json
│   ├── envelope.go  chain.go           # ② Envelope/Chain/Stage/首段 RepositoryRead(AUTH-03)
│   └── (单包;已有 denylist)
├── datasetfs/              应用能力:本机消费视图(只读)
│   ├── README.md                       # ① (Phase 0 补)拥有只读挂载;不拥有写/协议语义
│   ├── plan.go                         # ② 挂载计划与固定 commit 语义
│   ├── mount_linux.go  mount_unsupported.go   # ② 平台面(build tag,同包)
│   └── (单包,当前零 kc 依赖;Phase 0 落 denylist 钉全环)
├── identity/               应用能力:身份绑定
│   ├── README.md                       # ① 拥有用户名↔IdP subject 耐久绑定;不存凭证/授权
│   ├── identity.go                     # ② VerifiedUser/Bind,fail-closed
│   └── (单包,现仅依赖 kernel+internal/jsonfile;Phase 0 落 denylist 同 gate 严集)
├── observability/          应用能力:证据与遥测(非 Canonical)
│   ├── README.md                       # ① 拥有证据/派生统计;不是知识/授权权威(O-01)
│   ├── recorder.go  accesslog.go       # ② Recorder/AccessLog 协议口(耐久 ack 才回 EvidenceID)
│   ├── filestore.go                    # ② 默认介质 adapter(装配注入;其它介质同语义)
│   └── (单包;Phase 0 落 denylist ↛{retrieval,index,maintenance 面,transport}——不可禁 knowledge,serving 反向依赖它;层归属=裁决项 A)
│
│
│   ━━━ 入口与闸:不是能力,只组合/只检查 ━━━
│
├── cmd/                    入口,不是能力:二进制,只组合(kc / kcfs / kc-integration)
├── scripts/                入口,不是能力:工程工具(check-docs / coupling / validation-inventory)
├── internal/
│   ├── arch/                           # 闸:层表 + denylist + 闭集(fail-closed)
│   ├── gitdir/ journal/ jsonfile/ repofile/ treepath/   # infra,低于⓪①,非能力
│   └── telemetry/  testkit/
├── docs/  webui/  dsh-plugin/  output/   # 文档治理 / 宿主集成 / 资产,非能力
└── .data/                                # 草稿与场景数据,非能力
```

### 2.3 能力目录的内部形状：深度跟着隐藏需求走，不跟着地位走

应用环画成单目录不是低一等。单包能力同样过四件套，只是第 3 件由 **unexported 标识符**承担：
Go 的包内私有已经是语言级隐藏，`internal/` 只在实现需要跨包、或有实现体必须挡住其他包 import 时才出现。
四种真实形状：

| 形状 | 第 2 件（interface） | 第 3 件（隐藏） | 现例 |
|---|---|---|---|
| 单包能力 | 根包导出文件 | unexported 标识符 | `delivery` `gate` `identity` `invoke` `connector` `datasetfs` `controlplane` `kernel` |
| 契约多面 | 根上读/写两面或 port 集 | `internal/` 实现包 | `index`（retriever/maintainer）、`snapshot`（store）、`knowledge`（writer/reader 在根，codec/视图藏） |
| 装配能力 | 装配缝 + 配置合同 | `internal/` 唯一 adapter import 点 | `home` |
| 闭集契约 | 闭集函数 + 锚点测试 | 面后实现 | `cli`（surface.go）、`server`（routes.go） |

单包能力的展开样貌（四件套齐备，深度为零）：

```text
gate/
├── README.md                    # 1 拥有合并证据闸;不拥有知识、不拨用户系统、不是 hook
├── gate.go  rules.go            # 2 契约面:导出符号即契约,锚点测试钉形状
├── gate_test.go                 # 4 行为验证
└── state.go  …（unexported）    # 3 隐藏:包内小写即实现,外部不可见
```

拆出 `internal/` 的时机（满足其一，且 README「依赖方向」与 denylist 同步加条目）：
1. 实现确需多包（持久化介质抽象、provider 接入、编译器/规划器分离）；
2. 出现必须只经注册缝触碰的实现体（adapter/驱动）；
3. 根包导出面开始膨胀——先藏实现，再谈加符号。

判断信号用现成棘轮，不靠目录深度:`scripts/coupling` 单列的 exp/kL 与 prodLOC——
单包能力这两列暴涨就是该拆 internal 的信号；目录里多几个文件本身不是。

### 2.4 修正记录（相对上一轮讨论）

- `index/` 保持顶层能力目录，**不**收进 `retrieval/internal/`：应用核心直接装配 Retriever
  （`kc/index` 现被 `knowledgeapp`/`cli`/`home`/`retrieval/opensearch` import），埋入 internal
  等于把应用面契约藏起来。
- `gate/` 保持独立能力，**不**内部化：事实是 `cli`/`controlplane`/`home` 三方消费，
  上一轮「只有 merge 消费」的说法有误。
- `invoke/`、`datasetfs/` 缺 README（已核实），是现有能力里仅有的材料缺口，Phase 0 补齐。
- R1 修正（2026-09-28，施工首日）：S3' 把工作区载体 `Invocation`（含 `WS *home.Home`，143 处动词体消费）
  一并搬进了 invoke，触发既有守卫 `TestApplicationPackageBoundaries`（只有 transport 可 import home），
  main 带红。修复：`invocation`/`command`/`handler` 移回 cli（`cli/invocation.go`），invoke 回归纯
  词汇表（掉 home/knowledge/knowledge/serving/snapshot 四个 import，instability 0.88→0.80，
  propagation 38.22%→37.86%，基线显式收紧）；`TestCLIAndHTTPUseSameTypedApplicationExecutor` 的
  `run:`→`Run:` 守卫字面量追平（等价字面量，断言不弱化）。
- R3 修正（2026-09-28，B5 循环分析）：service 闭包需要带 `WS *home.Home` 的执行载体，而 cli 与
  server 互不可 import、invoke 按设计禁 home——载体必须有第三个家。落地为顶层 `invocation/`
  缝合包（`internal/telemetry` 同类共享缝；按耦合单元模型立顶层单元——internal/* 聚合桶会把
  度量拉出 +16.5pp 伪增值）。invoke 保持纯词汇（R1 结论不变）；cli 经类型别名垫片，143 处动词
  体站点零改动；§3.4 denylist 增 `invocation ↛ {cli, server, httpsurface, client, knowledgeapp}`
  与 home 豁免（持句柄、不开句柄）。B5 内部分层重排：B5a 载体（本条）→ B5b 授权/hooks/证据
  族分层迁出 → B5c 派发器 → B6 service 面。棘轮 34.83%→34.62%（26 单元，基线显式收紧）。
- R3 落地修订（2026-09-29，B6）：denylist 收窄为 `invocation ↛ {cli, server, httpsurface,
  client}`——`knowledgeapp` 解禁（B6 起动词操作体消费用例核心，缝是用例核心的编排层，与
  §3.3 的「cli/server 瘦身为纯前端」一致）；`server → home` 解禁（transport 打开 Home 装配
  执行载体）；新增合法边 `cli → server`（serve verb 是二进制组合根，cmd/kc 只 import cli；
  B8 复核是否收拢）。cli 动词体全部迁入 `invocation/verbs_*.go`，cli 经 `Run: inv.VerbX`
  注册表引用。行为语义变化一处：ResourceLookup 无 state runtime 时返回
  `TEMPORARY_UNAVAILABLE`（原实现经 `NewHTTPStateLookup(nil)` 得到同一码，合同不变）。
  棘轮 34.62%→38.62%（26 单元；缝成为更大枢纽 + server 获得传入边，B6 设计代价，基线
  显式重写并登记 TASK.md）。
- B7 落地（2026-09-29，Phase 2 就绪装配）：`serve_readiness` 迁 `server/internal/readiness`
  （Result/Cache/Probe/Deployment/EvidenceWriteProbe + 指纹缓存）；就绪探测改装配缝注入——
  `Providers.SearchBackend` 端口由 `home.NewSearchBackendProbe` 绑定具体索引 adapter
  （`retrieval/opensearch.Check`），readiness 包与 server 均不 import 具体 provider；A-01
  守卫落地为 `TestProviderAdaptersImportedOnlyAtAssembly`（provider 集 {snapshot/lakefs,
  snapshot/gitea, retrieval/opensearch, retrieval/llmhttp, retrieval/cache}；snapshot/commandlog、
  snapshot/treewriter 属能力树不属 provider；注册例外 server ↛→ retrieval/llmhttp 的
  reranker/embedder 装配，B9 吸收）。wire-shape 裁决（B6 债项收口）：`client` 为管理面
  公开 wire 契约家（负载类型 + typed 方法 + transport/身份/会话），typed service 路由形状
  继续由 server 自持；`server → client` 为守卫 why-text 记录的既决例外，双方不得单方迁移
  形状。棘轮 38.00% 零漂移（就绪迁移耦合中性）。
- B8 落地（2026-09-29，Phase 1/2 收尾）：cli 导出面按收尾 mandate 收窄 43→24——product
  输出文本（Help/ConsumeHelp/WriteHelp/ComposeHelp）与全部零外部消费者的迁移垫片
  （FlagString 族/RequireFlag 族/WriteAllow/ConfigDir/NormalizeLoginServer/
  PrincipalAllowed/ResourceLookup/HTTPHandler/RemoteServerURL）unexport，三个死垫片
  （resourceLookup/remoteServerURL/principalAllowed）删除；cmd/kc、cmd/kcfs、
  cmd/kc-integration、integrationruntime 所用符号（Run/RunWithTelemetry/ParseArgs/
  ParsedArgs/RunKnowledgeSetFS/ClientServerURL/NewSessionClient + 类型别名）全部保留，
  「公开形状只增不删」按对外合同不收缩执行。消费方收口：`TestCapabilityConsumerSets`
  钉住 delivery/gate/hook/identity/observability 的消费方集合（新消费方须显式扩表）；
  delivery README 消费方更正为 invocation 缝注入。B6 债项双收口：配对夹具改判设计使然
  （两替身行为不同）；`cli → server` 组合边判定保留（serve verb 即二进制组合根，
  HTTPServerOptionsFromFlags/HTTPHandlerFromConfig 即本地装配缝）。阶段门：make test +
  make test-contracts + make test-race + tag `capability-phase-2`。
- B9 落地（2026-09-29，Phase 2 协议回执）：`snapshot/receipt.go` 新增协议自有回执
  `snapshot.Receipt`/`ReceiptResult`（JSON 形状与 writer 历史输出一致；**新增，不替换**
  `treewriter.Receipt`，公开形状不收缩）；`treewriter.Receipt.Protocol()` 提供转换
  （子包→能力根依赖本已存在，无环）。invocation 两文件改用：`WorkspaceCommitResult.Receipt`
  字段改协议类型（产品输出 JSON 不变，信封不再暴露 treewriter 实现类型，verbs_write 的
  treewriter import 随之消失）；`fillHookResult` 增 `snapshot.Receipt` 分支（producer 类型
  分支保留）。Phase 2 A-01 项至此全部吸收（B7 就绪端口 + B9 回执）。
- B10 落地（2026-09-29，Phase 3 收拢·裁决 C=实施）：`snapshot/lakefs`、`snapshot/gitea`
  收拢为公开注册薄壳（类型别名 + 函数变量再导出，公开 API 与调用面不变，home 不改
  import），实现整体迁 `snapshot/internal/{lakefsimpl,giteaimpl}`（含 package 内部测试；
  外部测试 `package lakefs_test`/`gitea_test` 留薄壳处继续测公开合同，RepositoryContract
  组 `TestLakeFSMatchesTreeProviderByOperationStep` 绿）。A-01 守卫升级：adapter 集改记
  internal impl 路径，仅对应 shim 可 import（外加 snapshot 树根豁免）；层表增
  `snapshot/internal/{lakefsimpl,giteaimpl}`。裁决 A=observability 上提 app 横切能力、
  B=gate 保持独立能力（B11/B13 执行）。耦合 38.00% 零漂移。
- B11 落地（2026-09-29，Phase 3 收拢）：`knowledge/unitcodec` → `knowledge/internal/unitcodecimpl`
  （消费者 `internal/repofile` 在 knowledge 树外，按 Go internal 可见性走薄壳
  `knowledge/unitcodec`）；`knowledge/semanticview`、`knowledge/maintenance`、
  `catalog/worktree` 同样收拢为薄壳 + `internal/{semanticviewimpl,maintenanceimpl,worktreeimpl}`
  （跨树消费者 index/invocation/testkit 保留原 import 路径=薄壳面；行为整体迁 impl，
  package 内部测试随迁，外部合同测试留薄壳处）。Go internal 规则补上语言级强制：
  knowledge 树外代码从此只能经薄壳面消费这些能力。层表增四个 impl；deny 规则为
  `catalog/internal/worktreeimpl` 复制层①语义；TestProtocolLayersDoNotDependOnClient
  覆盖全部 impl（强化）；TestCatalogStaysOffTheKnowledgeProtocol worktree 依赖闭包
  += `catalog/internal/worktreeimpl`。knowledge/catalog README 更新薄壳注记。

## 3. 能力规格

### 3.1 协议环

| 能力 | 契约（对外最小信息 → 公开类型） | 隐藏（internal 化） | 允许依赖 | 关键不变量 | 验证入口 |
|---|---|---|---|---|---|
| `kernel` | 坐标 / 错误信封 / canonical digest（`kernel/README.md` 四文件） | 无（全部即契约） | 无 | E-01 | kernel 单测 |
| `snapshot` ⓪ | Repository 口：不可变版本、条件推进、变化识别（`snapshot/store.go`） | `lakefs`/`gitea` 实现体、`treewriter`/`commandlog`/`stamp` | kernel, internal infra | W-01, W-02, K-02, K-23, A-01, CA-02 | RepositoryContract（Gitea/LakeFS）、A-01 守卫 |
| `catalog` ① | 登记与 Dataset 发布配方（`catalog/README.md`） | `worktree` | kernel, snapshot | KS-01, K-03, K-10…13 | `TestCatalogStaysOffTheKnowledgeProtocol` 依赖集闭集 |
| `knowledge` ② | Writer CAS/幂等、Reader 同版本 hydrate 窄口、Schema 声明 | `unitcodec`/`semanticview`/`maintenance` | kernel, snapshot | I-01, V-01, W-01/02, C-01, S-01, D-01, CA-01 | knowledge conformance |
| `retrieval` ③合同 | 查询计划、候选页、回读纪律、失败关闭、页界（SearchRequest/SearchView） | 无（纯合同） | kernel, snapshot, knowledge | R-01, R-02, K-25, C-01, IX-01 | retrieval conformance；禁 import index |
| `index` ③投影 | 读面 Retriever（exact-basis + READY）+ 写面 ProjectionMaintainer（basis/coverage/lag、代际、增量、对账） | planner/编译器/消费者对账/两条 lane；opensearch 实现体 | kernel, snapshot, knowledge, retrieval | P-01, PC-01, IX-02…05, K-19 | 投影 conformance；PC-01 测试组 |

声明式索引的三分映射（`core-architecture.md`「协议边界之内还要分清组件责任」）：
**声明**属于 Knowledge ②（S-01：Schema 只声明逻辑访问语义）；**对声明的编译与执行**属于
`index`（计划器、物理 digest、代际）；**请求正确性**属于 `retrieval` 合同 + `index` 读面。
`index/README` 的「不拥有」一节写明：不拥有声明本身、不拥有 Canonical、不决定正文是否发布。

### 3.2 应用环

| 能力 | 契约 | 隐藏 | 现实消费方（实测） | 关键不变量 |
|---|---|---|---|---|
| `knowledgeapp` | 全部 typed executor 入册：纵切 8 + 治理 5 + 投影 3（Read/Search/Commit、Dataset publish·read·search·relations·traverse、PROPOSAL→Merge、Projection describe/sync/notice；以导出面锚点为准） | 编排细节 | server（拆分前为 cli） | API-01, CA-01…03, KS-01/02, AUTH-01…03 |
| `home` | `DeploymentConfig` / `OpenDeployment` + 协议口装配缝 | `authority_drivers`（唯一 adapter import 点） | server, cli, invoke, scripts/fixture-deployment | A-01 |
| `server`（新） | `httpsurface` method+pattern 闭集 + handler 注册表 | mux、授权强制、遥测、就绪探测 | cmd/kc serve；cli 前端经 client | API-01, IX-01, AUTH-01…03, E-01 |
| `cli` | argv 闭集（`cli/surface.go`）+ client 会话 | product 输出编码 | 人；cmd/kc | QUALITY-05、check-surface 60 锚点 |
| `invoke` | FlagValue/ParseArgs/JSONOut + ObservationTelemetry | — | server, cli（拆分前 cli 单消费） | — |
| `client` | Identity / Authentication / Session | 会话持久化 | cli, cmd/kc-integration, integrationruntime | 协议层永不依赖 client（守卫已有） |
| `controlplane` | PROPOSAL→Preview→validate→Merge 操作语义 | journal、状态文件 | server, cli, invoke, knowledgeapp | K-07, K-08, K-09 |
| `gate` | 规则/证据绑定同一 previewId、fail-closed | gates.json 读写 | server, controlplane, home | K-08 |
| `connector` | `connector.Preview`（patch/reconcile）→ ChangeSet | 无源访问、无 Writer | integrationruntime | W-01 |
| `integrationruntime` | build/activate/run/status typed API | 运行宿主 | cmd/kc-integration | — |
| `hook` | action×phase binding、pre fail-closed、post outbox | hooks.json、投递实现 | server, home, invoke | K-21 |
| `delivery` | Envelope / Chain / Stage；无 `knowledge.read` 去正文 | 屏蔽规则实现 | server（拆分前 cli） | AUTH-03 |
| `datasetfs` | 只读挂载点语义 | mount 实现 | cmd/kcfs 经 cli | KS-01 |
| `identity` | VerifiedUser / Bind，fail-closed | identities.json、IdP 细节 | server, home | — |
| `observability` | Recorder / AccessLog 协议口 | FileStore JSONL、日志分区 | server, client, home, knowledge/serving | O-01 |

⚠ `observability` 因 `knowledge/serving` import 它而被钉在 knowledge 层——横切证据能力混进协议层。
裁决项 A（见「选定方案 / 被否决方案」）：要么 serving 经窄口消费、把它上提到 app/横切层，要么承认它是 ② 的一部分。

### 3.3 依赖方向规则（app 层内新增；层间规则不变）

- 层表仍为 `base / infra / snapshot / catalog / knowledge / retrieval / app` 七层
  （`internal/arch/layers_test.go::architectureLayer`）；本设计只在 app 层内加方向约束。
- `server → { knowledgeapp, home, delivery, gate, hook, identity, observability, controlplane, invoke }`
- `cli → { client, invoke, home }`（cli 前端不直接碰应用用例，一律经 typed client）
- 协议环（含 `index`）永不 import `server` / `cli` / `client`（守卫已有，随 `server/` 建包把 denylist 同步加上）。
- 每个能力的 README「依赖方向」一节与 denylist 同句（§1.3）。

### 3.4 应用环四件套现状审计（2026-09-28 实测）

「内部简单」不豁免标准——简单缩的是**形状**（单包、unexported），不是**有无**。实测应用环现有 14 个能力（不含新建的 `server`）：

| 能力 | ① README | ② 导出面 | ③ 隐藏 | ④ 自有 denylist 条目 | 缺口与处置 |
|---|---|---|---|---|---|
| `knowledgeapp` | ⚠ 落后于面：README「当前纵切」列 8 个 executor，代码导出 16+（治理 5 + 投影 3 未入册） | 95 导出（应用环最大单包，2087 行） | 单包，59 个 unexported 在压 | ✓ | **无形状锚点**（新增导出无人拦）；Phase 0：导出面闭集锚点 + README 对齐；§2.3 时机三已触发，Phase 3 后评估 internal/ |
| `home` | ✓ | 109（装配缝） | internal/authority_drivers | 焦点测试（A-01） | 无 |
| `cli` | ✓ | surface 闭集 + 60 锚点 | 单包+闭集 | 多条中被钉 | Phase 1 拆分 |
| `invoke` | ✗ | 24 | 单包 | ✗ | Phase 0：补 README + 条目 |
| `client` | ✓ | **89（偏大，观察项）** | 单包 | 仅为他人 denied 目标 | Phase 0：补条目；Phase 1 复核面 |
| `controlplane` | ✓ | 23 | 单包 | ✗ | Phase 0：补条目 |
| `gate` | ✓ | 25 | 单包 | ✗ | Phase 0：补条目（现仅依赖 kernel+jsonfile，可钉最严） |
| `connector` | ✓ | 28 | 单包 | ✓ | 无 |
| `integrationruntime` | ✓ | 23 | 单包 | ✓ | 无 |
| `hook` | ✓ | 38 | 单包 | ✓ | 无 |
| `delivery` | ✓ | 15 | 单包 | ✓ | 无 |
| `datasetfs` | ✗ | 16 | 单包 | ✗（当前零 kc 依赖） | Phase 0：补 README + 条目 |
| `identity` | ✓ | 12 | 单包 | ✗ | Phase 0：补条目（同 gate 可钉严） |
| `observability` | ✓ | 55 | 单包 | ✗ | Phase 0：补条目；层归属=裁决项 A |

结论：①③ 基本齐（缺 README 两个），**真正成片的缺口是 ④**——7 个能力没有自己的 denylist
条目，它们今天的隔离靠「恰好没人 import 它们」和零散焦点测试，不是 fail-closed。
四件套对小能力的**成本**≈一个 README + 一条 denylist + 少量锚点测试，所以越简单越没有理由不达标。

## 4. 分阶段执行计划（一批一提交：重构 → 验证 → 再重构）

节奏与 git 策略：
- **一批 = 一个 commit**；每批结束必须全绿才进下一批；红了先修本批，修不动 `git revert` 本批——批间不做交叉依赖，保证可单独回滚。
- 分支 `refactor/capability-boundaries`，`main` 不动；每阶段完成打 tag（`capability-phase-N`）。
- 每批同变更：`internal/arch` 守卫更新（层表/denylist）+ `scripts/coupling` baseline 复录（`-write-baseline` + TASK.md 注记）+ 交付说明列影响的不变量 ID。
- 禁 skip；公开形状只增不删；层表/denylist 变更属协议决策，同变更改 `core-architecture.md`。

通用门（每批都跑）：`go build ./...`、`make quality`、`go test ./internal/arch/`。
阶段末重门：`make test`（场景）+ `make test-contracts` + `make test-race`；阶段二末追加一次真实部署验收
（独立入口 `make deploy-local-scenes`，按各自合同单独进行，不混入重构提交）。

| 批 | 阶段 | 内容（详见下方 Phase 说明） | 验证门（通用门之外） | 提交点 |
|---|---|---|---|---|
| B0 | 基线 | 工作区清理（✓ 前轮遗留 4 文件 = `2ce24b6`）；设计升格 `docs/reviewed/` + `docs/graph/` 登记 + TASK.md 立项；建分支与 `pre-capability-refactor` tag；裁决 A/B/C | `make check-docs`（五段合同）+ status clean | 2 个 commit |
| B1 | 一·材料 | `invoke`/`datasetfs` 补 README；`knowledgeapp` README 与导出面对齐（16+ executor 入册） | 人工评审 README | 1 commit |
| B2 | 一·材料 | §3.4 的 7 条 denylist 落 `internal/arch`（proposed 集已按现 import 核过，加上即绿） | `go test ./internal/arch/ -run TestForbiddenDependencies` | 1 commit |
| B3 | 一·闸 | arch 新守卫：顶层生产目录 README 存在性；`knowledgeapp` 导出面闭集锚点 | `go test ./internal/arch/` + `go test ./knowledgeapp/` | 1 commit |
| B4 | 二·契约 | 新建 `server/`：`routes.go` 闭集 + `routes_anchor_test.go` + 层表注册 | `make check-surface` + `go test ./server/` | 1 commit |
| B5 | 二·搬家 | `service_routes`/handlers 迁 `server/internal/handlers`；cli 调用点改指 | `make test` + `TestCLIAndHTTPUseSameTypedApplicationExecutor` + `make test-contracts` | 1 commit |
| B6 | 二·搬家 | 授权强制、遥测迁 `server/internal/{auth,telemetry}`；denylist 补 `server↛{cli,client}`、`knowledgeapp↛server`、`cli↛knowledgeapp`（前端一律经 typed client）；`delivery`/`gate`/`hook`/`identity`/`observability` README 消费方改写 | `make test` + `make quality` | 1 commit |
| B7 | 二·搬家 | `serve_readiness` 迁 `server/internal/readiness`；就绪探测改装配缝注入（吸收 Phase 2 的 `opensearch.Check` 项） | `make test` + `make test-contracts` | 1 commit |
| B8 | 二·收尾 | cli 收尾（product 输出 unexported）；`delivery`/`gate`/`hook`/`identity`/`observability` 消费方为 server 的守卫/文档收口 | 阶段末重门 + `make test-race` | 1 commit + tag `capability-phase-2` |
| B9 | 三·协议回执 | `snapshot/receipt.go` 新增；`cli` 两文件改用（**新增，不替换**，不收窄公开形状） | `make test` + A-01 组 | 1 commit |
| B10 | 三·收拢 | snapshot internal 化（裁决 C：薄壳 + `internal/{lakefsimpl,giteaimpl}`；不做则仅 denylist 更新） | RepositoryContract 组（`TestLakeFSMatchesTreeProviderByOperationStep` 等） | 1 commit |
| B11 | 三·收拢 | knowledge internal 化 + `catalog/worktree` internal 化（更新 `TestCatalogStaysOffTheKnowledgeProtocol` 依赖集） | `make test` + `make test-contracts` | 1 commit |
| B12 | 三·收拢 | opensearch 归属 ③投影（裁决 C 同批）；A-01 守卫扩展定稿（生产包 import 具体 adapter 仅限 `home` 与能力薄壳）；`index/internal/` 拆分仅按 §2.3 时机评估，无实益不做 | `make test` + coupling 终值对比 | 1 commit + tag `capability-phase-3` |
| B13 | 四·裁决 | 裁决项 A（observability 层归属）/ B（gate 独立性）落地 | 同变更改 `core-architecture.md` + `make test-boundary` | 1 commit |
| B14 | 四·验收 | §5 全门 + coupling 终值 vs 38.22% + 交付说明（实现范围 vs 已验证范围）；设计文档回写实际落点 | §5 全部门 | 1 commit |

各 Phase 的具体改动内容如下（批次表只定顺序、提交点与门，不重复内容）：

**Phase 0 · 材料补齐（无代码移动）＝ B1–B3**
- `invoke/`、`datasetfs/` 按模板补 README（四件套检查表逐项过）。
- 为 §3.4 表中 7 个无自有条目的能力补 denylist（proposed 集已按当前 import 逐个核过，加上即绿：
  `invoke ↛ {cli, httpsurface, client, home}`（transport 中立性——它存在的意义；R1 后 invoke 已零
  home/cli/client 依赖）；
  `client ↛ {cli, httpsurface, home}`；`controlplane ↛ {cli, httpsurface, client, home}`；
  `gate ↛ {snapshot, catalog, knowledge, retrieval, index, hook, controlplane, cli, httpsurface, client, home}`；
  `identity ↛` 同 gate 严集；`datasetfs ↛` 全协议环+全部 transport/装配（当前零 kc 依赖）；
  `observability ↛ {retrieval, index, controlplane, hook, gate, cli, httpsurface, client, home}`
  ——不得 deny knowledge，`knowledge/serving` 反向依赖它）。
- `knowledgeapp` 导出面闭集锚点（仿 `cli/surface.go` 60 锚点做法，把全部 16+ executor 与请求/响应类型入集，
  新增导出符号须显式改集）；README「当前纵切」与导出面对齐（如实登记治理与投影执行器，不收窄不隐藏）。
- 验证：`go test ./internal/arch/ -run TestForbiddenDependencies` 仍绿（条目均按现 import 可加）+ §5 新守卫（README 存在性）。

**Phase 1 · cli 拆分（承接 QUALITY-05，本轮唯一结构性大刀）＝ B4–B8**
1. 先立 `server/routes.go` 闭集 + 锚点测试（先契约后实现，禁止两份实现并存）。
2. 搬 `cli/serve*`、`service_routes.go`、handlers、授权强制、遥测、readiness 进 `server/`
   （实现放 `server/internal/`）；`cli/` 保留 `surface.go` 与 product 输出。
3. 同变更：arch 层表加 `server`；denylist 补 `server ↛ cli/client`、`knowledgeapp ↛ server`、
   `cli ↛ knowledgeapp`；
   `delivery`/`gate`/`hook`/`identity`/`observability` 的 README 消费方一节同步改写。
- 验证：`TestCLIAndHTTPUseSameTypedApplicationExecutor`、`TestFormalServiceNamespacesAreExplicitAndRetiredRoutesStayMissing`、
  `make check-surface`、`make test-contracts`、`make quality`。

**Phase 2 · adapter 泄漏修复（能力边界包住装配与就绪）＝ B7 + B9**
- `cli/dataset_commit.go`、`cli/home_hookrun.go` 中的 `treewriter.Receipt`：在 snapshot 能力口
  增加协议自有回执类型（**新增，不替换**，不收窄公开形状），CLI 信封改用新类型。
- `server/serve_readiness.go` 的 `opensearch.Check(...)`：改为装配根注入的 readiness 端口
  （checker 在 `home` 装配时绑定具体 provider），server 只见端口。
- 验证：A-01 守卫扩展（生产包 import 具体 adapter 仅限 `home` 与能力根 shim）。

**Phase 3 · internal 收拢（一次一个能力，可逆）＝ B10–B12**
- `snapshot/lakefs`、`snapshot/gitea` → 公开注册 shim + `snapshot/internal/{lakefsimpl,giteaimpl}`；
  shim 保持原导出 API，`home` 不改 import。
- `retrieval/opensearch` → `index/opensearch` shim + `index/internal/opensearchimpl`（provider 归属
  ③投影能力）。
- `knowledge/unitcodec|semanticview|maintenance` → `knowledge/internal/…`；
  `catalog/worktree` → `catalog/internal/worktree`（同步更新 `TestCatalogStaysOffTheKnowledgeProtocol` 依赖集）。
- 验证：每批 `make test` + `make test-contracts` + `make quality`；coupling baseline 重录。

**Phase 4 · 层归属裁决（需明确决策，不随 Phase 3 顺手做）＝ B13**
- 裁决项 A：`observability` 层归属（§3.2 ⚠）。
- 裁决项 B：`gate` 保持独立能力（倾向，三方消费）还是收进 `controlplane/internal/`（需 server 经
  controlplane 转出 gate 操作）。

## 5. 验收与棘轮

- `scripts/coupling -strict`：propagation cost ≤ 现基线 38.22%，且随批次推进应单调下降；
  instability drift ≤ 0.05；baseline 重录必须留 TASK.md 注记。
- `make quality` / `make test-boundary` / `make test-contracts` / `make check-surface` /
  `make check-docs` 全绿，无 skip。
- **升级为自动检查**（本轮信号三出口中的「升级守卫」）：`internal/arch` 新增守卫——
  顶层生产目录必须存在 `README.md` 且其「依赖方向」节与 denylist 覆盖一致（存在性 = B3，一致性 = B12）。
  新顶层目录未过四件套检查即 fail-closed，与 L-01 的包层归属检查同构。

## 选定方案 / 被否决方案

**选定**
- 顶层扁平 + 层表分组；否决 `capabilities/` 物理嵌套（依赖非树形、coupling unit 错位、import path 噪声、无强制力）。
- 能力四件套为准入条件；否决「README 可后补」——写不出「不拥有什么」即降级或合并。
- `index` 为顶层能力、双导出面（retriever/maintainer）；否决收进 `retrieval/internal/`（应用核心直接装配 Retriever）。
- adapter 采用「公开薄 shim + internal 实现」；否决纯 denylist 约束（少一档语言级强制）。
- cli 拆分先立 routes 闭集再搬实现；否决实现先行（QUALITY-05 已证无零倒置切面，契约先行才有缝）。

**被否决**
- 把应用能力升格为第五协议边界（core-architecture Non-Goals）。
- 为迁就目录物理分组而合并接入面与消费面、或合并 hook 与 connector（权限模型与流量模型不同）。
- big-bang 一次性重排全部目录。

### 待裁决（立项前必须裁决）
- A：`observability` 层归属（knowledge 层现值 vs 上提为 app 横切能力）。
- B：`gate` 独立能力（倾向）vs `controlplane/internal/`。
- C：adapter shim 模式是否本轮实施（可选强化；不做则维持 A-01 denylist 现状）。

## 硬性约束 / Invariants

本设计触碰且不得削弱的不变量：A-01、L-01、API-01、AUTH-03、P-01、O-01、CA-01、KS-01、IX-01…05
（可证伪属性与证据入口见 [架构不变量](architecture-invariants.md)）。

- import path 变更面大：一次一个能力、每步全绿、可回退；`git mv` 不改内容，守卫更新与移动同 commit。
- 不收窄公开形状：新增类型（如协议回执）不替换旧导出；删改导出符号属协议决策。
- README 不复制设计：README 持形状与最小用法，`docs/reviewed/` 持设计；某能力需要设计 owner
  文档时走 `docs/graph/` 登记，不在 README 里长出第二份设计正文。
- 层表 / denylist 变更 = 协议决策：同变更改 `core-architecture.md`，并在交付说明列明影响的不变量 ID。

## 接口契约 / 状态机

状态机不复制进本文：接入「未提交或形成提案」与消费「先固定坐标再鉴权」的请求语义由各边界
Conformance 持有。

- 协议口：`snapshot/store.go`、`catalog/README.md`、`knowledge/README.md`、`retrieval/README.md`、
  `index/README.md`（读面/写面两文件名在本设计落地时定稿）。
- 应用口：`knowledgeapp` executor 集（`docs/reviewed/service.md` §1.2）、`cli/surface.go`、
  `httpsurface.Patterns()`、`home/README.md` 装配合同。
- 闸：`internal/arch/layers_test.go`、`provider_boundary_test.go`、`semantic_test.go`；
  `scripts/coupling`（baseline 与 -strict）。
- 迁移承接：`TASK.md` QUALITY-05（cli 内部分层，形状确认 2026-09-24 已完成）。
