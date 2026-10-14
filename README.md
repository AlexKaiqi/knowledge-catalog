# Knowledge Catalog — 通用知识底座（Go 参考实现）

为人、应用和 Agent 提供可发现、可获准、可使用并留痕的对象环境。对象可以是材料、数据、
代码、工具或服务；团队独立维护供给，使用者按任务选择并在真实授权下使用、持续修正。
这是 Catalog 协议的 Go 参考实现：Snapshot 仓承担协作、版本与生产发布，Object 层组织
对象内容及元数据，插件在明确作用域内扩展业务能力。当前具体插件方向为声明式索引与
LiveAccess；Affordance 同时说明对象和系统自身怎样使用。大文件、大量文件的存储与版本
能力由底层方案承载，系统整体容量与恢复能力另需验证。

接入方维护自己的内容、模型和来源；消费者采用需要的范围，并保留所用版本与引用依据。
普通文件可先组合交付，需要结构化知识、查询或动态资源时再增加相应能力。
愿景、核心概念与架构见 [`doc/`](doc/README.md)。
申请访问衔接资源拥有者或既有业务流程；通用自主申请与 Wield 等后续扩展仍属设计方向。

## 当前 MVP

当前版本是**预配置与既有授权下可用的单实例 Server/Client 参考实现**：即使部署在一台机器上，`kc`、`kcfs` 也只通过 typed API 进入 KC Server；Store 和 Retrieval provider 可以本机部署，但不能绕过知识、授权、证据和索引语义。核心入口已实现，不代表全程自助、所有 adapter 或宿主已完成验收。共享服务试点有条件可用，多实例生产服务尚未验收；边界见[路线](doc/roadmap.md)，验证结论须对应具体运行范围与源码版本。

完整设计从 [`doc/README.md`](doc/README.md) 开始：愿景、概念、架构、各篇设计与路线。
本机阅读（UTF-8 HTML）：`make docs-serve`，然后打开提示的地址（默认 `http://127.0.0.1:8766/doc/README.md`）。

先按角色进入，不必先读完整设计：

| 角色 | 最短闭环 | 入口 |
|---|---|---|
| 知识接入方 | Client `create` → `attach --repo` → `writer commit --dir`（或 `writer put <对象URN>`）→ `read <对象URN>`；来源核对走 `provenance` | `kc help write` |
| 治理方 | `attach --repo <配置源>` → `dataset define --source <repository>` → `grant add`；serve 追 live 投影，必要时 `operations projection sync --repo` | `kc help compose` |
| 知识消费方 | `连接服务 → show`；已知对象直接 `read <对象URN>`，可加 `--dataset`；未知对象先按仓或 Dataset `search`，再读取命中引用 | `kc help consume` |

Dataset 负责文件范围、组合与版本交付，不是写入源仓的前置条件；普通文件无需知识 Schema，结构校验和声明式检索在知识层提供。

每个部署显式绑定实际 `kr://kc/system` 仓；部署初始化将 `systemrepo/content` 的模型、入口、机制、使用说明和基础 Schema 经 Writer 发布，启动核对，运行时只读实际仓。接入方在自己的
Knowledge Repository 中版本化 Domain Schema；Writer 会校验 Schema 文档、兼容性和引用实例。
`kc deployment system publish --config deployment.yaml` 保留显式发布/核对入口（空仓写入、已占用只校验），不自动覆盖不同版本，也不在启动时挂载内置仓。
`POST /knowledge/v1/schemas:list`（CLI `kc schema list`）可在选择知识集前分页列出一个固定 Repository 已发布的实体。面向
人、IDE 与通用 Agent 文件工具的默认投影是 Semantic YAML view（例如
`object/semantic/metrics/*.yaml`），Canonical 单元信封只属于维护/存储形状。产品设计和
用例见 [`doc/design/object.md`](doc/design/object.md)。

**Knowledge Catalog 语义只有一套**：身份、版本、来源、写边界、知识集组合、维护闭环、联邦读取。不同的是 store adapter。协议分层 ⓪–③（[`doc/architecture.md`](doc/architecture.md)；不要和介质梯子混名）：

```text
③ 检索派生     AccessSpec / RetrievalPlan / CandidateRef / 完整回读
M 访问物化      StateLookup 端口 + 外部 State / Stream runtime（上层产品）
② 知识内容     object_id、Aspect、来源信封、schema/*、Binding handle
① 组合平面     Dataset / 登记空间：来源、文件范围、不可变发布版本与固定 pin
⓪ 存储适配     Snapshot authority（当前生产实现为 lakeFS）
```

挂用户 git 停在 ⓪+①（链接 + 读授权，不拿走正文）。Aspect 从 ② 才感知。

## 核心理念

> 别把 authority 已经会的版本图重新发明成协议。⓪ 是 Snapshot 坐标与 CAS。① 是组合平面，不是文件仓、不是知识协议。协议在 ② 补身份、来源、写边界与外部访问声明；③ 是可丢派生。

- **身份**（RESOLVE，②）：`ObjectIdentity ≠ path`，身份在文件内容（frontmatter），Address = `object_id` + aspect + member。
- **来源**（GET_PROVENANCE，②）：精确 commit 坐标 + 各单元信封；不是 git log。
- **写**：`COMMIT`/`PROPOSAL` → Snapshot；State/Stream 是 Aspect Binding 的观察面，不是 Writer Surface。
- **目标 store**：当前通过 `snapshot` 根包接入 lakeFS，具体后端在 `snapshot/internal/lakefsimpl`；Dataset、权限、HTTP 与完整控制台分别归具名一级模块。Gitea/filegit/Dolt 存储 adapter 已退役删除；独立身份认证不受影响。`declarativeindex/opensearch` 提供可重建 Snapshot/State 派生。未配置检索时仍提供 Snapshot 精确 READ/VFS，但 SEARCH/RELATIONS 明确缺能力；Bound State READ 与动态字段 SEARCH 通过独立 runtime 服务。见 [`doc/architecture.md`](doc/architecture.md)。

### 概念与动词

| 概念 | 动词 | 输入 → 输出 |
|---|---|---|
| Object / Address | RESOLVE / READ / PUT / REMOVE | 见 Reader / Writer |
| 来源信封 | GET_PROVENANCE | 对象+commit → `ProvenanceTrace.chain` |
| 当前态/事件流/时序观测 | Aspect State/Stream Binding | 消费 READ 可 hydrate State；State 字段可进入独立动态投影并返回双 basis；Stream window、TTL/retention、cursor/checkpoint 由上层产品治理 |
| Dataset（公开 Go 类型 KnowledgeSet） | dataset define / ResolveKnowledgeSet | 发布时冻结文件范围和 commit；消费时固定一个已发布版本 |
| Object 历史 | LOG / DIFF | 对象+commit → `ObjectRevision[]`；两 commit → `ObjectDiff` |
| Schema 内省 | DESCRIBE_SCHEMA | 只暴露字段 `text/filter/sort` 逻辑访问语义 |
| 检索计划 | RetrievalPlan | ResolvedKnowledgeSet + AccessSpec + provider capabilities → 每请求路由 |
| 工作投影 | SEARCH / describe-index | 仅 OpenSearch；本地未配置时 SEARCH 明确缺能力，Snapshot 精确 READ/VFS 不受影响 |
| 外部资源 | ResourceDescriptor | Agent 读取自包含访问句柄，再走全系统统一访问；默认不沉淀 |
| Proposal | propose / validateStructure / MERGE | 候选 Ref；结构检查后记录 PASSED/FAILED |
| 外部套件 | recordValidation | 只绑定传入的 PASSED/FAILED，不跑测试 |

## 结构

```text
kernel/             # 无依赖底座：错误、canonical digest、Repository/Commit 坐标
snapshot/           # ⓪ Store / TreeStore / ref / CAS / Advanced
├── dataset/        # 唯一仓登记、Dataset 组合与发布历史
├── access/         # 服务、Repository 与 Dataset 授权
├── service/        # 原始文件与 Dataset HTTP 服务
├── console/        # 完整控制台与 Dataset 页面
├── client/ cli/    # 所属 typed SDK 与可组合任务交互
└── internal/       # lakeFS 与底层私有实现
object/             # 业务对象基本合同、固定读写与唯一组装
├── reader/ writer/ service/         # 固定解释与任务流程
├── client/ cli/                    # 所属 typed SDK 与任务交互
├── format/ semanticview/            # 文件编码及消费表示
└── cache/ maintenance/              # Hydrator 与独立派生维护
interfaces/         # 普通能力合同、实现解析、登记和调用
└── service/        # 固定范围、授权、隔离调用与结果核验
liveaccess/         # 当前态访问的独立实现及来源端口
├── service/ contract/
├── client/ cli/    # 当前态访问、调用与插件说明入口
└── discovery/      # 向动态投影供给已核验观察
discovery/          # Schema 字段 access、逻辑查询及补判
├── service/ datasets/               # 发现任务及可搜索 Dataset 流程
├── client/ cli/                    # 所属 typed SDK 与任务交互
├── projection/                     # 派生编译、固定依据与就绪
└── llmhttp/                        # 可选模型处理提供方
declarativeindex/   # 声明式索引插件、具体投影与维护
├── projection/ control/            # 固定/动态投影与后台恢复
├── opensearch/                     # 私有物理驱动的公开装配入口
└── client/ cli/ plugin/             # typed 运维任务与真实插件登记
systemrepo/         # 完整平台出版物的显式发布与核对
controlplane/       # PROPOSAL → Preview → validate → Merge
gate/               # merge 证据清单
hook/               # CLI 出站 pre/post
observability/      # access→retrieval→refine→feedback 证据链、trace、派生 hitmap/training
datasetfs/         # Linux go-fuse 宿主投影；只消费固定的应用层文件计划
cli/  cmd/kc/       # 全局上下文、五模块 CLI 装配、部署与服务进程入口
server/ invocation/ # typed HTTP 接入、共享执行载体与所属服务适配
      cmd/kcfs/     # 本机多目录 mount 进程；不暴露为 HTTP 动词
internal/
├── gitdir/         # 不带领域语义的 Git 目录 plumbing
├── journal/        # 本机过程账
├── jsonfile/       # 原子 JSON 落盘
├── testkit/        # T12 / Writer 契约与测试装置
└── arch/           # 分层守卫：把架构总览的 import 规则跑成测试
doc/
├── README.md         # 文档入口与阅读顺序
├── vision.md concepts.md architecture.md roadmap.md
├── design/           # 按能力的设计篇，含用例与进度
└── engineering/      # 测试、质量闭环、测试目录与架构不变量
```

生产 `kc` 的业务请求经各模块 SDK 进入 Server；handler/stage 的内部登记、Invocation 遥测
和 kcfs 的本机夹具装配仅在根 CLI 的 `_test.go` 中编译。模块参数、输出及故障矩阵随所属
CLI/服务测试保存，根保留全局 argv、认证上下文、跨模块旅程与实际 Server 集成验证。

## Dataset 登记与组合

`snapshot/dataset` 管理服务唯一登记空间与 Dataset 发布历史，不解释对象内容。客户端选择服务后无需再选择空间。

- **KnowledgeSet** — 命名 Dataset 的不可变发布版本：来源、冻结 commit、文件范围；当前版本指针与历史发布记录分开保存。源分支推进不改变已发布版本
- **ResolvedKnowledgeSet** — 固定一次消费的 Dataset 版本、文件清单和 `{仓 → commit}`；历史 pin 由服务端发布记录恢复，权限仍按当前状态检查。动态 observation cut 由上层 Retrieval/Materialization 持有
- **发布与索引** — Dataset 发布与来源提交分别完成。启用检索时，正式发布编排先准备成员固定版本投影，再接受新 Dataset 版本；准备失败保留旧服务版。未配置检索的文件发布不要求索引就绪，也不自动获得搜索能力。默认消费最新已发布版，已打开的固定视图不随更新移动；完整部署恢复和动态覆盖仍按 MVP 范围验收
- 消费读 / `object_id` 在 `reader.Serving`，不在 登记空间。没有公开全量枚举或宿主直写式 snapshot export；未来若提供导出，必须是显式 typed streaming API，且不是消费 fallback
- Linux 上用 `kcfs mount --dataset <id> --root <现有项目>` 把已发布 Dataset 的消费 YAML 投影挂入用户工作区，让本地 Agent 像读普通文件一样访问。当前只读，仅适合本机容量能容纳的数据集，不用于超大 Dataset 的本地挂载。入口与身份沿用当前 Client 会话，kcfs 在挂载时内部冻结版本。目录和文件经 typed Knowledge Set File Gateway 读取，客户端不持有 Repository 机器凭证

Writer 幂等日志在配置 `stateDir` 下的 `writer.db`。登记空间 当前态看 `kc show`，权威历史看 `kc registry audit`。`system.jsonl` / `audit.jsonl` 过程账与访问、反馈、检索原始证据同属耐久状态；hitmap 和检索投影可以重建。配置、登记空间 Snapshot 权威、知识 Snapshot 与服务状态都需要独立恢复来源；客户端登录态、配方和 pin 由客户端保存。文件职责见 [`home/README.md`](home/README.md)、[`snapshot/dataset/README.md`](snapshot/dataset/README.md) 与 [`doc/design/service.md`](doc/design/service.md)。

## 运行

```bash
export PATH="$HOME/.local/go/bin:$PATH"   # 若系统 go < 1.23
make test                 # 原有 lakeFS HTTP 夹具的普通/索引场景；复用或自动启动真实 OpenSearch
make deploy-local-scenes  # 独立真实 lakeFS 部署场景；需先显式准备 local 测试栈
make test-contracts       # 原组件/架构/application/HTTP 合同组合，显式运行
make quality              # gofmt/tidy/vet/staticcheck + 复杂度/文件体积/重复门禁
make test-state-runtime-e2e # 独立 Docker runtime + OpenSearch；HTTP index-sync/search 动态旅程
make test-server-web      # Server 浏览器登录与 Web 入口测试
make test-all             # 夹具、真实 lakeFS 部署、混合合同、Server Web 与其它 adapter/FUSE；需 local 测试栈
go run ./cmd/kc -- help
go run ./cmd/kc -- deployment init --config deployment.yaml # 首次初始化；登记空间 Snapshot 与状态独立持久
go run ./cmd/kc -- deployment status --config deployment.yaml
go run ./cmd/kc -- serve --config deployment.yaml         # 恢复既有部署，不创建 Snapshot
go run ./cmd/kcfs -- plan --dataset agent --root "$PWD"
./scripts/e2e-kcfs-docker.sh                   # Docker 内真实 Linux/FUSE 验收
# Linux + fuse3: 将 plan 改成 mount，进程存活期间提供多个只读宿主挂载
```

`deployment.yaml` 是部署配置，不是运行缓存。配置类型与完整校验在 `home.DeploymentConfig`；例如独立的 登记空间 Snapshot 权威，以及平台管理的存储池：

```yaml
version: 1
stateDir: /var/lib/kc/state
cacheDir: /var/cache/kc
bootstrapPrincipal: alice
auth: lakefs
authURL: https://lakefs.example
listen: 127.0.0.1:7380
registry:
  id: kr://acme/registry
  driver: lakefs
  dsn: https://lakefs.example/kc-registry
repositories:
  - id: kr://kc/system
    driver: lakefs
    dsn: https://lakefs.example/kc-system
managedRepositories:
  driver: lakefs
  dsn: https://lakefs.example
  root: s3://kc-authority
  creatorActions: [writer.preview, writer.commit, knowledge.read, knowledge.provenance, knowledge.schema.read]
```

Snapshot 机器凭据由部署环境注入；配置不含 token。配置、登记空间 Snapshot 权威、知识 Snapshot、`stateDir` 中的 grants/gates/receipts/治理过程/原始证据都必须持久保存。`cacheDir` 可以删除重建；缺失耐久状态时启动失败，不重新创建一个空 登记空间。平台仓通过显式 `kc create --name` 申请；自有仓通过 `kc create --url --credential-file` 连接；两者都在随后显式 `kc attach --repo` 时才进入当前 登记空间。绑定与创建结果持久保存，无需逐仓改配置；外部仓不存在时失败。creatorActions 不提供默认权限，调用者只获得部署明确选择的窄动作；重放与重启不补回已撤销 grant。

按角色进入可先用 `kc help consume`、`kc help write`、`kc help compose`；
三个角色帮助先给出同一套 登记空间/Repository/Dataset 心智模型，再给最短
操作路径；`kc help` 保留完整协议表。

Agent 使用同一分组 `kc` CLI 和普通 shell/文件工具，经 Server 与显式身份消费知识。
未知对象使用 `kc search`，字段合同使用
`kc schema describe`，已知对象直接 `kc read`。不存在公开知识
枚举或 SEARCH→LIST 降级；可选只读文件挂载由 `kcfs` 提供，不存在 KC 服务端 Session Store。
本仓库 DSH 宿主集成及专属模型验收运行器已退役。通用 Agent 的概念解释、入口选择、恢复与
任务效果仍需独立的真实模型证据，当前未执行；验收边界见
[测试与验证](doc/engineering/testing.md)。

```bash
# 运维：预先准备 登记空间 Snapshot 权威与托管存储池，配置可恢复的部署输入及显式创建者策略。
# 只有首次初始化执行 init；替换进程后直接 serve，保留 stateDir 与远端权威。
kc deployment init --config deployment.yaml
kc serve --config deployment.yaml           # 终端 A

# 终端 B。接入方登录已有 Server；此前已获目标 登记空间 的 repositories.create 准入。
export KC_SERVER_URL=http://127.0.0.1:7380
kc login
REPOSITORY="$(kc create --name public-core | jq -r '.repositoryId')"
kc attach --repo "$REPOSITORY"

# 接入方：构造对象引用，提交完整内容，再用同一引用读回。
REFERENCE_REPO="$(printf '%s' "$REPOSITORY" | jq -sRr @uri)"
SCHEMA_REFERENCE="urn:kc:object:${REFERENCE_REPO}:schema%2Frunbook.body"
RUNBOOK_REFERENCE="urn:kc:object:${REFERENCE_REPO}:runbook%2Fpayment-oncall"
kc writer put "$SCHEMA_REFERENCE" --command-id schema-1 \
  --value '{"entity":"Runbook","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}'
kc writer put "$RUNBOOK_REFERENCE" --command-id sync-1 --schema-ref schema/runbook.body \
  --value '{"body":"切换支付流量前先检查冻结窗口"}' \
  --origin-kind SOURCE --source-ref file:///source/runbooks/payment-oncall.md
kc read "$RUNBOOK_REFERENCE"

# 治理方：命名知识集、发权。serve 追 live 投影；sync 用于历史 EnsureAt / 强制重建 / 排障。
kc dataset define agent --revision 1 --source "$REPOSITORY"
kc operations projection sync --repo "$REPOSITORY"
kc registry audit

# 消费方：同一对象引用限定到已发布 Dataset；精确历史读取保留返回的 commit。
kc show
kc show
kc search --dataset agent --query 冻结窗口
kc read "$RUNBOOK_REFERENCE" --dataset agent
kc provenance --dataset agent --object runbook/payment-oncall
kc log --dataset agent --object runbook/payment-oncall
# 共享服务可验证 Gitea 登录；调用方带 Authorization，主体变为稳定的 gitea:<user-id>
kc serve --config deployment.yaml # auth: gitea / authURL / bootstrapPrincipal 在持久配置中
```

`--dataset` 跟已发布配方（`KS-01`）。精确 READ 在已有 object id 时仍可用；SEARCH
返回 `CAPABILITY_UNSATISFIED` 不是零命中。检索投影由治理方维护，不是消费命令。

## Conformance

自动化入口是 `scripts/testsuite.sh`，Make target 只是稳定别名：

| 组 | 命令 | 边界 |
|---|---|---|
| lakefs（默认；local 为同义入口） | `make test` / `make test-lakefs` | 原有普通/索引场景；lakeFS adapter + 进程内 HTTP 假服务 + 真实 OpenSearch |
| 真实 lakeFS 部署 | `make deploy-local-scenes` | 独立部署入口；真 Graveler / MinIO / PostgreSQL / OpenSearch，需已有 local 测试栈 |
| contracts | `make test-contracts` | 原组件 + 架构 + 应用/transport 合同组合 |
| component | `make test-component` | 各 Go 组件单元测试、本地合同；live adapter 在 short 模式跳过 |
| boundary | `make test-boundary` | ⓪–③ import、类型归属、术语与 provider 边界 |
| e2e | `make test-e2e` | 共享应用语义与 typed Client/HTTP/登记空间 边界；结束时对账全部产品 `kc` 命令 |
| adapters | `make test-adapters` | 真实 Gitea、OpenSearch |
| state-runtime | `make test-state-runtime-e2e` | scene `_materials/accessor` 的 Resource Access 容器 + OpenSearch；动态候选与 Snapshot 不变性 |
| docker | `make test-docker` | adapters + State runtime + Docker Linux/FUSE |
| all | `make test-all` | 场景、真实 lakeFS 部署、contracts、docker 与插件；需准备 local 测试栈，不包含付费 Agent、Taihu 或规模资格 |

默认 `make test` 运行 `TestProductScenes` 与 `TestMetricPermissionScenes`，不需要准备整套 lakeFS 部署。已设置 `KC_TEST_OPENSEARCH_URL` 时复用该服务，否则自动启动并清理一次性 OpenSearch。真实部署使用独立的 `make deploy-local-scenes`；组件/架构/逐命令合同用 `make test-contracts`。场景夹具不执行独立 Go evidence、动态 State 和走查叶，不能把某一组通过当作全部通过，也不能用含隐式 skip 的 `go test ./...` 冒充完整外部适配器验收。

| 测试 | 不变量 |
|---|---|
| T1 Path Move | 文件移动后 ObjectIdentity / KnowledgeRef 不变 |
| T2 Commit CAS | 过期 expected target commit 被拒绝 |
| T3 Atomicity | 任一操作失败无部分提交 |
| T4 Command Idempotency | 精确重试返回原 Receipt；异内容冲突 |
| T5 | 已退役：底座没有 APPEND/Stream surface；state/stream 通过 Aspect Binding 声明 |
| T6 Authority Store | lakeFS 的版本身份、CAS、pinned read 与 provider-neutral conformance；夹具与真实服务证据分开 |
| T7 Ingestion/Grounding | ingest 扫描、reconcile 对账、groundingCitation |
| T8 Retrieval Projection | `discovery/projection/` 通用执行合同与 `declarativeindex/projection/` 可重建投影定位 + Canonical 回读；非权威、basis/lag；`AspectSelector` 只裁显式 READ |
| T9 Maintenance Loop | 完整多 Repository Preview、validateStructure、Validation basis；Merge 更新源仓，重新发布 Dataset 后新请求可见 |
| T10 Refine | SEM_FILTER 三值；SEM_RERANK 单次 listwise Provider、Ref-preserving RankGroup、fixed-basis/lane evidence、输入字节预算；Responses-compatible Luna 可选实测 |
| T11 登记空间 | Dataset Registry、不可变发布记录、故障传播、来源不覆盖、固定已发布版本 |
| T12 Snapshot + Knowledge composition | Snapshot 身份/CAS/历史 + 上层 Reader/Writer 的 LOG/DIFF/REMOVE、幂等、schema_ref、PROPOSAL |
| Hook / Gate | pre 非 0 无 commit；REPLAYED 不打 hook；post 只含指针；缺 suite 不能 merge；Preview 变了旧 PASSED 作废 |
| Collector helper | `patch` 不误删；`reconcile` 只在 Observed∩Scope 上 REMOVE；超 Scope 拒绝；预览可 COMMIT |
| End-to-end journey | `cli/mvp_acceptance_test.go` 固定接入方/消费方最短闭环；`cli/user_journey_test.go` 再覆盖 HTTP 读写、proposal、权限和生命周期 |
| Layering | `internal/arch`：[`doc/architecture.md`](doc/architecture.md) 的 import 与类型归属跑成断言。`snapshot/dataset → snapshot`；②不得依赖③；Snapshot adapter 不得依赖 Object/format/Discovery；ObjectID/Address/Provenance 由 `object` 声明 |
| CLI surface | 各模块 CLI 合同与根 `cli/command_test.go`：Help、输入与命令表对齐；退役动词仍报替代品；嵌入测试的 stage 归属保持；公开参数边界与 Server 集成分别验证 |

## 文档

- [`doc/README.md`](doc/README.md)：文档入口，给出阅读顺序和全部设计篇。本机 `make docs-serve` 渲染为 UTF-8 HTML。
- [`doc/vision.md`](doc/vision.md) → [`doc/concepts.md`](doc/concepts.md) → [`doc/architecture.md`](doc/architecture.md)：系统设计主干。
- `doc/design/`：按能力的设计篇，每篇含用例与进度，目录见 `doc/README.md` 的“设计”表。
- [`doc/engineering/testing.md`](doc/engineering/testing.md) 与 [`doc/engineering/test-catalog.md`](doc/engineering/test-catalog.md)：验证入口、用例与证据。

改文档后运行 `make check-docs`：`doc/` 下每篇以标题开头、不带 frontmatter、能从 `doc/README.md` 经链接走到，仓内 Markdown 链接不悬空。包级具体契约放在对应目录的 README，不复制到设计篇。执行接力棒是根目录 `TASK.md`。

## Store 扩展

权威 Adapter 实现 Snapshot capability，并与上层 Reader/Writer 组合验证同一 conformance；当前生产实现为 lakeFS，夹具与真实部署的通过范围分别以运行证据为准。具体 adapter 只在 `home/authority_drivers.go` 装配。检索引擎实现 `Retriever` / `ProjectionMaintainer`，Relation 候选同样只能来自 exact-basis Retriever，再按候选 ID 回读 Canonical。见 [`doc/architecture.md`](doc/architecture.md)。
