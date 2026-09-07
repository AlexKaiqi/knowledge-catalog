# Knowledge Catalog MVP 验收

本页只回答一件事：当前实现是否足以让知识接入方发布可验证知识，并让知识消费方在固定版本上发现、检索、读取和溯源。

## 结论与边界

| 形态 | 当前结论 | 承诺边界 |
|---|---|---|
| 单实例 Server/Client 参考实现 | **预配置与既有授权下可用** | 本机部署也由 Client 经 typed API 进入 Server；Dolt/Gitea 都提供 Snapshot authority 与精确 Knowledge 回读；SEARCH/RELATIONS 只经 exact-basis Retriever 发现候选，再按同一 commit 回读 Canonical。未配置对应能力时明确失败，不扫描降级 |
| 接入方、消费方全程自助 | **尚未完成** | 目标是交付客户端封装唯一服务入口，用户自行申请平台仓或连接自有仓、登记、按策略取得权限并完成发布维护和消费；平台仓创建与创建者显式授权按下述正式旅程验收；自有仓连接、登录封装和完整首次准入/分享闭环仍有缺口，见已知缺口 |
| 共享服务试点 | **有条件可用** | 需部署方提供 TLS、可信认证器、备份与单实例写入约束；Gitea 认证和远程 authority 可用，但不是完整生产平台 |
| 多实例生产服务 | **尚未验收** | 跨进程幂等/租约、独立 Catalog/Knowledge 服务部署、SDK/MCP、容量与故障演练仍不在当前保证内 |

已有协议、发布与固定版本消费能力不等于全程自助产品已验收，也不表示每个 adapter 或宿主
都已生产就绪。产品目标由 `KNOWLEDGE_PRODUCT_AND_SCHEMA.md` 拥有：部署方一次性建立
运行能力与显式策略，后续日常任务不要求其逐仓配置或代执行。验收应覆盖两条完整旅程：

1. **接入方**通过交付客户端登录，自助申请平台 Snapshot 或连接自己的 Snapshot，完成验证、登记与策略授权，再发布、修改、移除和恢复带身份、Schema 与来源的知识。
2. **消费方**通过同一客户端登录，浏览可见源并自主临时选源或采用已有命名知识集，固定本次任务，在同一 pin 上 SEARCH、READ 和 GET_PROVENANCE；显式建立新 pin 才采用上游更新。

## 当前可执行旅程：已有部署与显式准入

以下展示参考实现当前能执行的路径，不能当作全程自助验收。部署方仍须先持久保存声明配置、
远端 Catalog Git 和独立服务状态，再显式初始化；外部 Snapshot 接入仍须先建立来源连接；平台仓由已授权接入方显式申请。
配置形状和示例见仓库 README。已有部署只恢复并启动，不必重放首次搭建。这里的 local
身份仅用于本机测试配置，不是普通用户的产品登录：

```bash
kc deployment init --config deployment.yaml   # 首次：建立 Catalog 和首个管理主体
kc deployment status --config deployment.yaml
kc serve --config deployment.yaml            # 终端 A：只恢复

export KC_SERVER_URL=http://127.0.0.1:7380      # 终端 B
kc login --mode local --as user:local-admin    # 示例配置显式使用 auth: local
kc catalog repo attach --repo kr://acme/public/core
```

Catalog 接入在 Server 内只读验证已配置的 Repository，随后原子登记成员；不创建业务 Snapshot，
也不因登记授予维护或消费权限。当前新增连接仍须调整部署配置，不能称为接入方自助连接。
`local` 命令和独立 `catalog repo register` 已退役。

### 平台仓申请与重部署续用

部署配置声明托管存储池及 creatorActions，普通接入方已获 Catalog 创建准入后，可通过 `kc catalog repo create --catalog <id> --repo <id> --command-id <id>` 申请新仓。成功后直接 PUT 或 pack/commit，并按回执 commit READ/PROVENANCE；无需逐仓改静态配置，也不要求全局授权管理权限。实例替换从耐久创建账恢复连接与原结果，创建重放不补回已撤销权限。

正式证据均已通过：`TestManagedRepositoryProviderCreatesPublishesAndResumes` 从无目标绑定的已有部署和仅有创建准入的主体开始，在真实 Dolt 上验证发布、来源、缓存替换、create/Writer 幂等重放、CAS 维护与撤权；`TestManagedRepositoryProviderOnLiveGitea` 另在真实远端 Gitea 上验证创建、发布、缓存替换后的重放与回读。这些证据不代表自有仓连接、客户端交付和消费分享已完整实现。

### 知识接入方

已登记且具备仓维护和回读权限时，接入方可通过 Client 自行完成 `kc pack → kc writer commit → kc knowledge read --repo`。Workspace 面向消费组合，不是写入前置条件。接入方只提交自己的知识源 id 和草稿，不必命名 Snapshot ref。`pack --out` 把 ChangeSet 写到文件；stdout 只报告 files/diagnostics，不发布。

```bash
kc pack --repo kr://acme/public/core --dir ./drafts --out changeset.json
kc writer commit --command-id source-1 --changeset changeset.json
kc writer head --repo kr://acme/public/core
kc knowledge read --repo kr://acme/public/core --object runbook/payment-oncall
kc knowledge provenance --repo kr://acme/public/core --object runbook/payment-oncall
```

等价的单条 PUT 仍可用。需要 SEARCH 时先发布带 `text` AccessHints 的 `schema/*`。批量文件或外部源仍只有一条写边界：`pack` / `connector.Preview` 生成 ChangeSet，人工或系统检查后由 `commit --command-id` 提交。采集器、源凭证和业务映射留在底座之外。

日常修订保持同一 Address，重新打包并用新的命令身份发布；重试同一笔提交保留命令身份与
内容，结果不确定时查询原 Receipt。并发冲突后读取最新版本、比较合并并重新预览，不能
取消前置条件覆盖别人。显式移除与历史内容恢复同样经 Writer；恢复是新的发布，不倒退
权威历史。完整维护语义见 `KNOWLEDGE_PRODUCT_AND_SCHEMA.md` §5。

### 当前授权准备与可选共享知识集

托管仓创建者能力来自部署明确配置的窄动作策略，并形成可撤销的仓级规则。一般接入方/消费方的额外授权仍由具有授权管理权限的主体管理；完整首次准入与分享策略闭环尚未替代这些准备工作。命名知识集是可选的共享配方，不是消费方自行选源的前置条件。下列为
授权管理者的操作示例，消费方不运行这些命令：

```bash
kc admin grant add --principal user:consumer --action catalog.read,workspace.resolve,workspace.consume --catalog acme/catalog
kc admin grant add --principal user:consumer --action knowledge.read,knowledge.search,knowledge.schema.read --repo kr://acme/public/core
kc workspace define --workspace oncall --revision 1 --source kr://acme/public/core  # 可选共享配方
```

长寿命 `kc serve` 会把 live 检索投影追到各源 published HEAD；`projection sync` 只用于历史
commit、强制重建和排障，不是每次发布或消费的步骤。精确 READ 不依赖检索投影。

### 知识消费方

已有 Catalog 使用与成员仓消费授权时，用户可浏览可见源，自行选择并形成临时 Workspace；
也可以使用已有命名知识集。调用方不必预知 Catalog/Workspace id，不要跨多条命令各自追随
`latest`。`kc catalog show` 的 `repositories` 带源说明（title/summary 或明示无说明），不含
宿主路径或 Snapshot selector。检索投影由服务维护，不是消费命令。

```bash
kc catalog list                         # 发现可见 Catalog，不必先知道 catalog id
kc catalog show                         # 发现知识源与已有知识集
kc knowledge schema list --repo <发现的源>
kc workspace pin --source <发现的源> --out pin.json
kc knowledge search --pin pin.json --query 冻结窗口
kc knowledge read --pin pin.json --object <search 命中的 object-id>
kc knowledge provenance --pin pin.json --object <search 命中的 object-id>
```

可重复 `--source` 选择多个源，或用 `--workspace <发现的知识集>` 固定已有命名配方。临时
pin 保存定义、Catalog 与固定版本，不写 Catalog；它仍要求 Catalog 范围的组合消费权限与
成员仓当前权限，仅有某个命名知识集的授权不能任意临时选源。

上游更新不改变已有任务基点。需要采用更新时显式重新 pin 并保存为新文件；旧 pin 保持原
版本，权限撤销仍在下一请求生效。SEARCH 命中必须从同一 basis 回读 Canonical。`partial`
必须附带 claims；能力不足返回 `CAPABILITY_UNSATISFIED`，不能伪装成零命中。精确 READ
无法诚实表达缺失成员时 fail closed。

## 产品 MVP 必须满足

### 接入方

| ID | 用户结果 | 机器可判定条件 |
|---|---|---|
| P1 | Repository 能独立接入 | 接入方经客户端自行申请平台 Snapshot 或连接自己的 Snapshot，按显式策略完成验证、登记与维护授权；登记不初始化 Snapshot、不改 HEAD、不复制正文、不隐式发权。平台仓创建由正式同主体旅程验收；既有仓验证登记保留，自有仓动态连接待完成 |
| P2 | 身份不依赖路径 | 文件移动后 `object_id` 和 KnowledgeRef 不变 |
| P3 | 写入可安全重试 | 同 `command_id` + 同 digest 返回原 Receipt；异 digest 返回 `IDEMPOTENCY_CONFLICT` |
| P4 | 并发写不静默覆盖 | 过期 `expectedTargetCommit` 返回 `NON_FAST_FORWARD`；失败无部分提交 |
| P5 | Schema 与正文同仓解析 | 带 `schema_ref` 的 PUT 只能引用 target Repository 可解析的 `schema/*` |
| P5a | System Schema 信任根 | 显式初始化登记 `kr://kc/system`，后续部署恢复保留该成员；Meta Schema digest 与二进制一致，普通已认证用户只读 |
| P5b | Schema/实例校验 | Domain Schema 先过 Meta Schema；同批或既有 Schema 精确校验实例 Address、必填、类型与未知字段，失败不推进 HEAD；PUT 省略 `schema_ref` 而继承既有声明时同样校验 |
| P5c | Schema 兼容性 | 同一 Schema object ID 的 breaking 变化返回 `SCHEMA_INCOMPATIBLE`；兼容的非必填字段扩展可继续版本化 |
| P5d | Schema 反向依赖 | 更新 Schema 时按有界原生反向索引校验固定 basis 上全部受影响实例，失配返回 `SCHEMA_INSTANCE_INVALID`；删除 Schema 仍有引用者返回 `SCHEMA_INCOMPATIBLE`；provider 无该索引时失败关闭，不退化为全仓扫描 |
| P6 | 来源可验收 | READ 返回固定 commit 的值；GET_PROVENANCE 返回各知识单元的来源信封 |
| P7 | 批量接入先预览 | `pack` / `connector.Preview` 不写仓；只有 Writer COMMIT / PROPOSAL 改 Snapshot |

### 消费方

| ID | 用户结果 | 机器可判定条件 |
|---|---|---|
| C1 | 能发现消费入口 | `kc catalog list` 返回可见 Catalog ID（不含宿主路径）；`kc catalog show` / `catalog repo list` 的 `repositories` 为 `{id, profile, title?, summary?, schemaCount?}`（`profile` 为 present/missing/unsupported）；`workspace list|show` 仍返回成员源 id（不含 selector 或宿主路径）；单 Catalog 部署可省略 `--catalog` |
| C2 | 一次任务版本一致 | 消费方可临时选源或使用已有命名配方，无需新建共享知识集；`kc workspace pin` 无 `--out` 时 stdout 为 `{repo → commit}` 与 `pinId`，`--out` 时 pin 文档进文件、stdout 为 receipt。所有消费命令接受同一 `--pin`；上游更新不改变旧 pin，重新 pin 才按所选配方解析版本 |
| C3 | 多仓读取不覆盖 | 同 `object_id` 的成员结果并集返回，public/group/personal 不互相覆盖 |
| C4 | 搜索结果可信 | Provider 只给 CandidateRef；公开 hit 在 SearchView basis 回读 Canonical，并带 version/evidence/completeness |
| C5 | 能区分空、缺能力和部分结果 | 零命中、`CAPABILITY_UNSATISFIED`、`partial + claims` 形状不同 |
| C6 | 可追溯 | READ、SEARCH、LOG 和 GET_PROVENANCE 都绑定明确 Repository commit |
| C7 | 授权不越层 | `read-workspace` 不隐式授予成员 Repository 的 `read`；裸 READ 不返回假完整结果 |
| C8 | Agent 能解释并选入口 | 自然语言询问核心概念、接入边界或 SEARCH 缺能力时，Agent 只加载随包 Skill 即给出可判定的正确答案和最小下一步 |

### 服务与运维

| ID | 用户结果 | 机器可判定条件 |
|---|---|---|
| S1 | Transport 唯一 | 业务 CLI 是 typed Client，即使本机部署也不打开 Home；HTTP route 调用共享应用服务，不依赖 CLI parser/command table |
| S2 | 身份来源可信 | 部署配置显式声明认证；local 模式只接受 `X-Kc-As`，taihu/gitea 模式只接受已验证 `Authorization`；错配失败关闭。新部署由显式 init 建立首个管理主体，后续授权经 Server，重部署恢复既有授权 |
| S3 | 可判断存活与就绪 | `/livez`、分 surface `/readyz`、`/metrics` 不依赖知识响应正文 |
| S4 | 权威与派生可区分 | Snapshot 保存知识，远端 Catalog Git 保存组合；配置、授权、gate、幂等/Receipt、控制面与审计证据独立持久保存。工作 clone 与索引可丢可重建，缺失耐久状态失败关闭 |
| S4a | 更换实例仍能继续任务 | 删除整个 cache 并换进程后，Catalog、既有 pin、授权、Receipt 重放与治理状态仍可恢复；启动不补建空授权或业务 Snapshot |
| S5 | 分层可执行 | `internal/arch` 阻止 Catalog 感知知识协议、Writer 依赖 Retrieval 等反向依赖 |

## 自动化证据

```bash
make test          # 临时 OpenSearch + component + boundary + 应用/transport 合同
make test-cover    # short suite、公开动词覆盖和 statement coverage 门禁
make test-race     # 并发敏感包的 race detector
make test-plugin   # DSH MountController、Skill、只读人用浏览、构建与包内容
make test-kcfs-e2e # Docker Linux /dev/fuse：kcfs + DSH MountController 真实生命周期
make test-agent-e2e # 真实模型六角色；需要 dsh + 模型凭证，禁止 host/filesystem 旁路
make test-agent-metric-e2e # KC-AGENT-01：`.data/scenes/` 状态目录里的 Agent as 任务块；需要 OpenSearch
make test-agent-ux-e2e # 真实模型概念问答；检查 Skill trace、语义组和零旁路
make test-service-e2e # 真实 Gitea + OpenSearch、双身份 HTTP 旅程
make test-all      # 再验收真实 Gitea / Dolt / OpenSearch / Linux FUSE
```

关键证据入口：

- `cli/mvp_acceptance_test.go`：通过测试夹具固定两条最短角色旅程；
- `cli/deployment_recovery_test.go`：`TestDeploymentSurvivesInstanceReplacement` 与 `TestDeploymentMissingDurableStateFailsClosed` 验证正式 Run/HTTP 的实例替换与缺失状态边界；`cli/deployment_contract_test.go` 守卫退役命令和显式配置入口；
- `cli/temporary_knowledge_test.go`：`TestProductTemporaryPinConsumesFrozenKnowledgeAndCurrentPermissions` 验证临时 pin 跨命令消费、上游更新隔离与当前权限；`cli/remote_knowledge_basis_test.go` 验证所有 Knowledge DTO 传递同一任务定义和 pin；
- `cli/server_client_only_test.go`：`TestRemoteProviderReadBackAndConsumerDiscovery` 按部署 → 接入方发布 → 治理方 compose/grant → 消费方发现 的顺序，用产品 `--server` Client 走 pack/commit/read 与 list/show/schema list/pin/search/read；`TestServeProjectionWorkerCatchesCommitWithoutSync` 证明长寿命 serve 在无手工 `projection sync` 时仍能追上 published HEAD；角色命令与库存 JSON 不得出现 `--home`、宿主路径或 Snapshot selector，显式任务 pin 另承载固定版本；消费 SEARCH 失败不得教运维命令；
- `cli/service_roles_live_test.go`：真实 Gitea 认证、Dolt/OpenSearch 上的 provider/consumer 独立身份、固定 pin 与更新隔离；
- `knowledge/writer/*_test.go`：P2–P7；
- `snapshot/commandlog/*_test.go`：跨写面的 command-id claim、重放和冲突；
- `catalog/*_test.go`、`cli/consume_flow_test.go`：C1–C7；
- `index/*_test.go`、`retrieval/*_test.go`：候选回读、basis、能力与 continuation；
- `cli/user_journey_test.go`：通过测试专用 embedded seam 验证共享应用语义；`cli/serve*_test.go` 和 remote CLI 测试验证产品 Server/Client 边界；
- `cli/command_evidence_test.go`：以生产 `cliSurface` 为分母的逐命令成功与风险分级边界报告；
- `cli/http_contract_inventory_internal_test.go`、`cli/http_surface_coverage_test.go`：以生产 route registry
  为分母的 67 条 HTTP 路由所有权、method、namespace 与 HTTP/Client 成功语义；
- `dsh-plugin/scripts/agent-scenarios.json`：真实 Agent 验收的机器可读分母，登记六个核心角色、
  四个首次使用/概念问答、`DW-AGENT-01` 数仓 companion 和 `KC-AGENT-01` metric 权限 companion；
  runner 与清单漂移立即失败；
- `dsh-plugin/scripts/e2e_agent_roles.py`：真实 Agent 分别完成 source 发布、Workspace 治理检查、
  固定 pin 读取、audit/log/provenance 审计、坐标冲突恢复与越权写拒绝；每个角色保存回答和
  Skill/shell trace，最终状态 oracle 同时证明合法写入生效、越权写入未污染权威状态；
- `dsh-plugin/scripts/e2e_agent_questions.py`：真实 Agent 回答消费者心智模型、提供方
  接入边界和缺能力恢复问题；每题保存回答、Skill-only trace 和确定性语义 oracle；
- `internal/arch`：分层与术语守卫。

`make test` 通过证明共享应用语义、分层和 typed transport 合同；`make test-service-e2e` 提供
预配置与已授权角色的 Server/Client live 证据；平台仓创建另由具名正式旅程验证，不能据此声称自有仓连接与完整准入能力已交付。
依赖外部服务或 Linux FUSE 的能力，只有对应 live 测试真实通过才可对外宣称；SKIP 不是 PASS。

## 当前已知缺口

这些是**实然落后于应然**，不是把设计改小。对外宣称时不得假装已经具备。

- **客户端唯一入口与普通用户登录**：产品目标是交付客户端封装服务地址，用户只登录并复用连接。当前原生 CLI 仍要求 `--server` 或 `KC_SERVER_URL`，已有登录文件不自动提供后续命令的服务地址（`cli/remote.go`、`cli/run.go`）；Taihu 授权码交换仍在客户端要求部署应用秘密 `KC_SERVICE_CLIENT_SECRET`（`cli/remote_login.go`），Gitea 登录依赖用户已有 token。这些不是普通使用者应承担的部署步骤；需要完成客户端发行配置、连接持久化和不暴露服务端应用秘密的登录闭环。
- **Snapshot 自助供给、连接与凭证**：当前 `catalog repo attach` 只有仓身份输入，必须命中静态 binding 或已经供给的托管仓耐久 binding；未知来源失败，CLI 拒绝在 attach 传递连接位置（`client/management.go`、`home/deployment_runtime.go`、`cli/deployment_contract_test.go`）。平台仓已新增显式 create 与独立耐久管理路径，验收见前文；自有仓动态连接与凭证维护仍未暴露；Gitea authority 统一读取服务进程 `KC_GITEA_TOKEN`，不能满足接入方分别维护自有仓连接授权。目标是在服务管理面持久保存和验证连接，与 Catalog 成员登记分开；不得把连接、秘密塞进 Catalog 协议，也不得把“先请部署方改配置”写成已完成自助接入。
- **首次准入与仓维护者分享**：当前认证不自动产生 Catalog 使用权，仓登记不产生维护权；授权仍由持有 `admin.grants.manage` 的主体经 Server 显式管理（`cli/surface.go`、`cli/allow.go`）。平台仓 create 按显式 creatorActions 建立该仓可撤销的维护授权；仍缺完整新用户准入，以及仓维护者在策略范围内管理消费分享的产品入口。自动发权须有独立可追溯策略，不能以登记成功或源系统账号替代 KC 授权；临时选源也不能越过当前仓读权。
- **Gitea Knowledge READ**：tree 仓的精确读已经走 Writer 写入的 `.kc/knowledge-units.index`（`treeManifestLocator`），不是 ListFiles 扫全树。`TestT12GiteaContract` 证明 Gitea 上 Reader/Writer 合同成立。缺的是 Gitea 原生 ② 表（规模 profile，见 `SCALE_ARCHITECTURE.md`），以及 SEARCH 仍依赖 exact-basis 检索投影（`R-01`/`R-02`），不是「Gitea 仓不能成为 Knowledge Repository」。
- **源说明热状态与 discovery 关闸**：`KNOWLEDGE_PRODUCT_AND_SCHEMA.md` U6 / §3.5。`kc catalog show` / `repository list` 已在应用层 READ 保留源说明并填 `repositories[]`（title/summary 或 `profile: missing`）。缺的是投影 READY/lag claims，以及声称进 discovery 却无说明时的失败关闭。`RETRIEVAL.md` 延期的是 SEARCH 的 Facet/total count，不能用来取消 BROWSE，也不能把 BROWSE 改回对象 LIST。
- **Catalog 范围 SEARCH 语法糖**：`PERMISSIONS.md` 接口表与 `SERVICE_ARCHITECTURE.md` §2.5。应然：`kc knowledge search --catalog` 解析 `discoveryWorkspaceId`，准入是 `catalog.read`，不另要 discovery 的 `workspace.consume`。实然：参考实现尚未暴露该糖与 Catalog 配置字段；当前支持临时选源/保存的 pin、命名 `--workspace` 与单仓 `--repo` 搜索。消费方可以在既有授权下自行选源，不依赖管理员创建命名知识集；但不能把所有登记仓自动当成已经发布的 discovery 成员。help / SURFACE / Walkthrough / `SERVICE_ARCHITECTURE.md` §5.3 不得把该糖写成已提供。命名知识集与 `--repo` SEARCH 的搜宽读严、consume 不隐含 `knowledge.*`、交付链独立层已由 `AUTH-01` / `AUTH-02` / `AUTH-03` 固化。不改变精确 READ / VFS 的 fail-closed。交付链首段之后的隐私化未选定（`PERMISSIONS.md` Non-Goal），不是本条待做项，禁止实现。
- **Connector registry/runtime**：采集与访问正交、写回只走 Writer，见 `CONNECTORS.md`。底座目前只有 Preview helper（ModePatch/Reconcile 是对账模式，**不是** Writer PATCH Surface）。墙外 runtime 未接入不是「产品没有 Connector」。
- **State 投影控制收口进度**：change notice 入站合同是 `index.ChangeNotice`（仓/ref/可选 Address/可选 sourceRevision hint，拒绝正文）。`Controller.Notify` / `CatchUp` 与 Snapshot Desire 分钥；冷启动全量 `RefreshState`，notice 走 `RefreshStateObjects`。公开入口是 `kc operations projection notice` 与 `POST /operations/v1/projections:notice`。消费 SEARCH 仍不得 `RefreshState`。尚未收口的是 `PROJECTION_CONTROLLER.md` §11.3 Docker 首版（真实 observer、Gitea、KC 重启）。`index-sync` 仍可用于 Snapshot EnsureAt、历史 pin、强制重建和排障，不再是动态 live 的唯一入口。
- **Stream projection / RetrievalPlan**：Aspect 可声明 Stream Binding。普通 READ 对 Stream 已失败关闭（`TestOrdinaryReadRejectsStreamBinding`）。缺的是 window/query 面与投影；Binding 里的 `protocol: mcp` 只是 ResourceDescriptor 字段，不是 MCP Gateway。
- **多实例 / MCP Gateway / 多语言 SDK**：`SERVICE_ARCHITECTURE.md` 的规模化拆分与 MCP 网关是方向；未落地记在这里，不是 §12 否决。公开 `append`/`stream` 命令与退役 HTTP 路由已保持 404（`TestAppendAndStreamSurfacesStayAbsent`）。
- **Tree Writer 写放大**：file-backed COMMIT 在 `knowledge/writer/treecodec.go` 仍 `ListFiles` 整棵知识树再写 manifest。Dolt 走 `ChangeStore` 增量。这是规模缺口，不否定 Gitea 精确 READ。
- **公开入口仍叫 Loom**：`TERMINOLOGY.md` 禁止把 Loom 当产品名；实现里 `dsh-loom`、`dsh --profile dsh-loom`、`/api/loom/vfs` 仍在用。协议层不要跟着改回 Loom。
- **Schema 原位 breaking 迁移**：同一 Schema ID 的兼容演进已有；带迁移证据的 breaking 更新仍未实现，仍属 U5 生命周期，不是禁止 breaking。
- State Binding 已有独立动态投影和双 basis（精确 READ、同 revision SEARCH hydrate）。change notice 与控制器第二条输入已收口（见上条）。尚未验收的是 `PROJECTION_CONTROLLER.md` §11.3 Docker 首版、Stream 窗口与多实例生命周期。
- `kc serve` 已按正式 namespace 形成模块化单体；进一步拆成独立进程是部署选择，不是新协议层。
- command-id 能覆盖当前进程/共享日志的知识写面重试；多实例协调、分布式租约、灾难恢复，以及 attach/grant 等管理写入的统一重放，尚未形成生产验收。
- 已有 Go typed client。Catalog/命名知识集发现走 `/catalog/v1` 与 `kc catalog list`，单仓 Schema 发现走固定 basis 的 `/knowledge/v1/schemas:list` 与 `kc knowledge schema list`；维护读回走 `kc knowledge read --repo`；临时知识集走 `kc workspace pin --source <id>`。
- Gitea adapter 为原子 ref CAS 使用短生命周期 `kc-wip/*` branch；Gitea 1.26 的异步 action notifier 可能在清理后记录“ref 不存在”，不影响 commit/ref 结果，但生产日志治理仍需改用无临时 branch 的底层 commit API。
- Linux 宿主 VFS 是可选文件体验，不是接入或消费协议成立的前提。VFS 不是 Writer；只读是选定的宿主投影合同。
- Gitea/Dolt 等 authority 需要 `make test-all` 的真实环境证据；`make test` 已用临时 OpenSearch 验收检索语义，但不能替代生产容量、备份、升级和故障演练。

## Linux VFS 子验收

VFS 的目标是把 Workspace 的多个 Repository 子树投影到已有项目目录。它不增加知识语义，也不把编辑器写文件升级为新的 Write Surface。

```text
现有项目根
├── 用户原有文件
├── vendor/policy/       <- repo A: policy/
├── docs/catalog/        <- repo A: docs/
└── schemas/shared/      <- repo B: schema/
```

每个 `kcfs` 进程只解析一次 Workspace selector；所有 mount 共用该 pin，进程退出前不跟随 Repository HEAD。

| ID | 结果 | 机器可判定条件 |
|---|---|---|
| V1 | 附着任意已有项目 | 项目根只要求是可访问目录；非 mount 内容的 inode/bytes 不变 |
| V2 | 多目录组合 | 每个 `WorkspaceSource.Path` 是独立 mountpoint；可来自不同 Repository |
| V3 | 同仓多子树 | 同一 Repository 可投影多个不重叠的 `SubPath`；pin 中仍只有一个 commit |
| V4 | 一致视图 | `cat`、`rg`、IDE 和 Agent 读到相同 bytes；所有 mount 使用同一 pin |
| V5 | 固定版本 | mount 期间上游 ref 推进不改变 bytes；重启 `kcfs` 后才解析新 pin |
| V6 | 只读 | create/write/truncate/rename/remove 均失败，Repository ref 和原文件不变 |
| V7 | 授权 | 先检查 `read-workspace`，再逐 Repository 检查 `read`；无权成员不进入 plan/mount |
| V8 | 消费语义视图 | `--view semantic` 不要求 Repository mount path；固定 pin 生成 `knowledge/<source>/<entity-plural>/*.yaml`，保留 `_kc` 坐标且不暴露 Canonical 单元信封 |
| V9 | 显示开关语义 | 插件开关缺省关闭且只显示/隐藏已挂载文件；不负责连接、挂载、发权或改变 Agent 访问 |
| V10 | 无 Agent 专用 VFS | DSH 不替换标准 filesystem/search 工具，不导出第二套 `loom-fs` / `loom-search` |
| V11 | 发现后添加 | 未配置 `KC_WORKSPACE` 也能展示可见 Catalog、Repository、Schema 和命名知识集；只有“添加到项目”才建立固定 pin mount，可显式移除 |
| V12 | 安全路径 | 拒绝绝对路径、`..`、反斜杠、NUL、根挂载、重叠 mount 和 symlink 穿越 |
| V13 | 宿主失败可解释 | 缺 `/dev/fuse`、`fusermount3`、TreeStore capability 或非空 mountpoint 时明确失败 |
| V14 | 首次使用可发现、可恢复 | 新项目的“知识”侧栏可展开目录但文件树默认隐藏且不预扫；未接入时可选择命名知识集；Skill 从自然语言引导 SEARCH→Canonical READ，不要求用户先懂命令 |

环境要求：Linux 可访问 `/dev/fuse`，安装 `fusermount3`；容器显式暴露设备和挂载 capability；每个 mountpoint 不存在或为空。mountpoint 是目录，不支持单文件 mount，也不允许挂到项目根。

```bash
go test ./workspacefs ./catalog ./cli ./internal/arch -count=1
npm --prefix dsh-plugin run typecheck
npm --prefix dsh-plugin test
./scripts/e2e-kcfs-linux.sh
./scripts/e2e-kcfs-docker.sh
```

若未来提供可写文件体验，应使用显式 checkout/overlay + reconcile/commit，保留 `expectedTargetCommit`、`command_id`、逐 Repository 结果和冲突恢复。不能把普通 FUSE write 直接解释成协议 COMMIT，也不能伪装跨 Repository 原子事务。
