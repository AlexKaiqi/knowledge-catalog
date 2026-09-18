# Knowledge Catalog MVP 验收

本页只回答一件事：当前实现是否足以让知识接入方发布可验证知识，并让知识消费方在固定版本上发现、检索、读取和溯源。

本页综合用户任务、实现边界与声明的验证入口，不承担用例库存或执行结果台账。
验证方法和结果判读见 `TEST_CATALOG.md` §0.2；具名 Test 可由 `make validation-inventory`
定位，执行是否通过必须引用同次 run-id、scope、源码指纹与必要的 live/Agent 原始产物。
没有这些信息的历史通过描述，不作为当前版本的验收凭据。

## 结论与边界

| 形态 | 当前结论 | 承诺边界 |
|---|---|---|
| 单实例 Server/Client 参考实现 | **预配置与既有授权下可用** | 本机部署也由 Client 经 typed API 进入 Server；Dolt/Gitea 都提供 Snapshot authority 与精确 Knowledge 回读；SEARCH/RELATIONS 只经 exact-basis Retriever 发现候选，再按同一 commit 回读 Canonical。未配置对应能力时明确失败，不扫描降级 |
| 接入方、消费方全程自助 | **核心入口已实现，等待同次完整验收** | 已提供保存登录、显式首次准入、按名申请 Dolt/Gitea 托管仓及管理地址、Gitea 自有连接与轮换、受限分享、无 FUSE 的任务上下文和 Catalog 发现搜索。外部 SSO 配置与真实环境、各 authority/宿主及恢复承诺仍须分别引用正式证据；自有远程 Dolt 尚未支持 |
| 共享服务试点 | **有条件可用** | 需部署方提供 TLS、可信认证器、备份与单实例写入约束；Gitea 认证和远程 authority 可用，但不是完整生产平台 |
| 多实例生产服务 | **尚未验收** | 跨进程幂等/租约、独立 Catalog/Knowledge 服务部署、SDK/MCP、容量与故障演练仍不在当前保证内 |

已有协议、发布与固定版本消费能力不等于全程自助产品已验收，也不表示每个 adapter 或宿主
都已生产就绪。产品目标由 `KNOWLEDGE_PRODUCT_AND_SCHEMA.md` 拥有：部署方一次性建立
运行能力与显式策略，后续日常任务不要求其逐仓配置或代执行。验收应覆盖两条完整旅程：

1. **接入方**通过交付客户端登录，自助申请平台 Snapshot 或连接自己的 Snapshot，完成验证、登记与策略授权，再发布、修改、移除和恢复带身份、Schema 与来源的知识。
2. **消费方**通过同一客户端登录，浏览可见源并自主临时选源或采用已有命名知识集，固定本次任务，在同一 pin 上 SEARCH、READ 和 GET_PROVENANCE；显式建立新 pin 才采用上游更新。

## 当前可执行旅程：已有部署与显式准入

以下展示参考实现当前能执行的路径，不能当作全程自助验收。部署方仍须先持久保存声明配置、
远端 Catalog Snapshot 权威和独立服务状态，再显式初始化；部署可启用托管池、首次准入与自有 Gitea 连接策略，用户随后通过公开入口申请或连接来源。
配置形状和示例见仓库 README。已有部署只恢复并启动，不必重放首次搭建。这里的 local
身份仅用于本机测试配置，不是普通用户的产品登录：

```bash
kc deployment init --config deployment.yaml   # 首次：建立 Catalog 和首个管理主体
kc deployment status --config deployment.yaml
kc serve --config deployment.yaml            # 终端 A：只恢复

export KC_SERVER_URL=http://127.0.0.1:7380      # 终端 B
kc login --mode local --as admin    # 示例配置显式使用 auth: local
kc catalog use kr://acme/catalog
kc attach --repo kr://acme/public/core
```

Catalog 接入在 Server 内只读验证已配置的 Repository，随后原子登记成员；不创建业务 Snapshot，
也不因普通 attach 授予维护或消费权限。新增自有 Gitea 连接使用
`kc create --url --credential-file`，只允许部署批准的 provider；逐仓授权保存于 Server 私有
存储，无需逐仓修改部署配置。
`local` 命令和独立 `catalog repo register` 已退役。

### 平台仓申请与重部署续用

部署配置声明托管存储池及 creatorActions，普通接入方已获当前 Catalog 创建准入后，可通过
`kc create --name <名称>` 申请新仓；只有多个池时需选择 `--store`。Server 生成仓身份与恢复
坐标，返回管理地址。成功后显式 attach，再 PUT 或 `writer commit --dir`；实例替换从耐久创建账恢复
连接与原结果，创建重放不补回已撤销权限。

正式验证入口：`TestManagedRepositoryProviderCreatesPublishesAndResumes` 从无目标绑定的已有部署和仅有创建准入的主体开始，在真实 Dolt 上检查发布、来源、缓存替换、create/Writer 幂等重放、CAS 维护与撤权；`TestManagedRepositoryProviderOnLiveGitea` 另在真实远端 Gitea 上检查创建、发布、缓存替换后的重放与回读。按名创建、同名账号或隔离空间与管理地址还由 `TestManagedProductHumanSelfServiceOnLiveGitea`、`TestManagedProductHumanSelfServiceOnDolt` 验证。自有 Gitea 连接见 `TestRepositoryConnectionCLIRecoversExpiredCredentialsWithoutRebinding` 与 `TestRepositoryConnectionOnLiveGitea`。这些入口的存在不等于正式验收已全绿；是否通过仍须引用包含对应测试且未跳过的同次运行记录。

### 知识接入方

已登记且具备仓维护和回读权限时，接入方可通过 Client 自行完成 `kc writer commit --dir → kc read --repo`。Workspace 面向消费组合，不是写入前置条件。接入方只提交自己的知识源 id 和草稿，不必命名 Snapshot ref。Client 对照当前版本求差后提交；可选 `kc diff --dir` 看同一对照，不是提交前必经步骤，也不写出 ChangeSet。

```bash
kc writer commit --command-id source-1 --repo kr://acme/public/core --dir ./drafts
kc writer head --repo kr://acme/public/core
kc read --repo kr://acme/public/core --object runbook/payment-oncall
kc provenance --repo kr://acme/public/core --object runbook/payment-oncall
```

等价的单条 PUT 仍可用。需要 SEARCH 时先发布带 `text` AccessHints 的 `schema/*`。批量文件或外部源仍只有一条写边界：Client 对期望正文求差，或墙外 `connector.Preview` 计算 diff，由 `commit --command-id` 提交。采集器、源凭证和业务映射留在底座之外。

日常修订保持同一 Address，改草稿目录并用新的命令身份发布；重试同一笔提交保留命令身份与
内容，结果不确定时查询原 Receipt。并发冲突后读取最新版本、比较合并并重新提交，不能
取消前置条件覆盖别人。显式移除与历史内容恢复同样经 Writer；恢复是新的发布，不倒退
权威历史。完整维护语义见 `KNOWLEDGE_PRODUCT_AND_SCHEMA.md` §5。

### 当前授权准备与可选共享知识集

托管仓创建者能力来自部署明确配置的窄动作策略，并形成可撤销的仓级规则。用户通过
`admission show` 查询当前 grants 与外部申请入口；审批后由授权管理者运行 `grant add`。
命名知识集是可选配方，不是自行选源的前置条件。下列为
授权管理者的操作示例，消费方不运行这些命令：

```bash
kc grant add --principal consumer --action catalog.read,dataset.resolve,file.read --catalog kr://acme/catalog
kc grant add --principal consumer --action knowledge.read,knowledge.search,knowledge.schema.read --repo kr://acme/public/core
kc dataset define --dataset oncall --revision 1 --source kr://acme/public/core  # 可选共享配方
```

长寿命 `kc serve` 会把 live 检索投影追到各源 published HEAD；`projection sync` 只用于历史
commit、强制重建和排障，不是每次发布或消费的步骤。精确 READ 不依赖检索投影。

### 知识消费方

已有 Catalog 使用与成员仓消费授权时，用户可浏览可见源，自行选择并形成临时 Workspace；
也可以使用已有命名知识集。调用方不必预知 Catalog/Workspace id，不要跨多条命令各自追随
`latest`。`kc show` 的 `repositories` 是身份列表（及 `schemaCount`），不含 README
title/summary，也不含宿主路径或 Snapshot selector。检索投影由服务维护，不是消费命令。

```bash
kc catalog list                         # 发现可见 Catalog，不必先知道 catalog id
kc show                                 # 发现知识源与已有知识集
kc schema list --repo <发现的源>
kc search --repo <发现的源> --query 冻结窗口
kc read --repo <发现的源> --object <search 命中的 object-id>
```

可重复 `--source` 选择多个源，或用 `--dataset <发现的知识集>` 固定已有命名配方。临时
pin 保存定义、Catalog 与固定版本，不写 Catalog；可通过每个所选仓的解析、消费与读取授权，或相应 Catalog 范围授权建立；仅有某个命名知识集的授权不能任意临时选源。结构化任务上下文不要求文件挂载。

多源任务才重复 `--source` 生成 pin；没有 Catalog 范围 SEARCH。

上游更新不改变已有任务基点。需要采用更新时显式重新 pin 并保存为新文件；旧 pin 保持原
版本，权限撤销仍在下一请求生效。SEARCH 命中必须从同一 basis 回读 Canonical。`partial`
必须附带 claims；能力不足返回 `CAPABILITY_UNSATISFIED`，不能伪装成零命中。精确 READ
无法诚实表达缺失成员时 fail closed。

## 产品 MVP 必须满足

### 接入方

| ID | 用户结果 | 机器可判定条件 |
|---|---|---|
| P1 | Repository 能独立接入 | 接入方经客户端自行申请平台 Snapshot 或连接自己的 Snapshot，按显式策略完成验证、登记与维护授权；登记不初始化 Snapshot、不改 HEAD、不复制正文、不隐式发权。平台仓按名创建和自有 Gitea 连接均有 typed 入口与具名正式旅程；连接失败保留旧绑定，凭证轮换不改变 authority，自有远程 Dolt 尚未支持 |
| P2 | 身份不依赖路径 | 文件移动后 `object_id` 和 KnowledgeRef 不变 |
| P3 | 写入可安全重试 | 同 `command_id` + 同 digest 返回原 Receipt；异 digest 返回 `IDEMPOTENCY_CONFLICT` |
| P4 | 并发写不静默覆盖 | 过期 `expectedTargetCommit` 返回 `NON_FAST_FORWARD`；失败无部分提交 |
| P5 | Schema 与正文同仓解析 | 带 `schema_ref` 的 PUT 只能引用 target Repository 可解析的 `schema/*` |
| P5a | System Schema 信任根 | 显式初始化登记 `kr://kc/system`，后续部署恢复保留该成员；Meta Schema digest 与二进制一致，普通已认证用户只读 |
| P5b | Schema/实例校验 | Domain Schema 先过 Meta Schema；同批或既有 Schema 精确校验实例 Address、必填、类型与未知字段，失败不推进 HEAD；PUT 省略 `schema_ref` 而继承既有声明时同样校验 |
| P5c | Schema 兼容性 | 同一 Schema object ID 的 breaking 变化返回 `SCHEMA_INCOMPATIBLE`；兼容的非必填字段扩展可继续版本化 |
| P5d | Schema 反向依赖 | 更新 Schema 时按有界原生反向索引校验固定 basis 上全部受影响实例，失配返回 `SCHEMA_INSTANCE_INVALID`；删除 Schema 仍有引用者返回 `SCHEMA_INCOMPATIBLE`；provider 无该索引时失败关闭，不退化为全仓扫描 |
| P6 | 来源可验收 | READ 返回固定 commit 的值；GET_PROVENANCE 返回各知识单元的来源信封 |
| P7 | 采集预览不写仓 | `connector.Preview` 不写仓；只有 Writer COMMIT / PROPOSAL 改 Snapshot。人写 YAML 直接 `writer commit --dir`；可选 `kc diff` 看对照，不写仓 |

### 消费方

| ID | 用户结果 | 机器可判定条件 |
|---|---|---|
| C1 | 能发现消费入口 | `kc catalog list` 返回可见 Catalog ID（不含宿主路径）；`kc show` 的 `repositories` 为 `{id, schemaCount?}`，`workspaces` 只返回成员源 id；单 Catalog 自动 use |
| C2 | 一次任务版本一致 | 消费方用 `--dataset` 对准已发布配方，或用 `--repo` 对准单仓；命令开始时内部 resolve 一次（`V-01`），回执带 `commit`。精确历史重放抄 `--repo --commit`。产品 argv 不出现 `kc pin` / `--pin`。Dataset 发布冻 commit（`KS-01`）；权限仍按当前 grants 求值（`KS-02`） |
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
| S4 | 权威与派生可区分 | Snapshot 保存知识，独立 Catalog Snapshot 权威保存组合；配置、授权、gate、幂等/Receipt、控制面与审计证据独立持久保存。工作 clone 与索引可丢可重建，缺失耐久状态失败关闭 |
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
- `cli/server_client_only_test.go`：`TestRemoteProviderReadBackAndConsumerDiscovery` 按部署 → 接入方发布 → 治理方 compose/grant → 消费方发现 的顺序，用产品 `--server` Client 走 commit/read 与 list/show/schema list/pin/search/read；`TestServeProjectionWorkerCatchesCommitWithoutSync` 证明长寿命 serve 在无手工 `projection sync` 时仍能追上 published HEAD；角色命令与库存 JSON 不得出现 `--home`、宿主路径或 Snapshot selector，显式任务 pin 另承载固定版本；消费 SEARCH 失败不得教运维命令；
- `cli/service_roles_live_test.go`：真实 Gitea 认证、Dolt/OpenSearch 上的 provider/consumer 独立身份、固定 pin 与更新隔离；
- `knowledge/writer/*_test.go`：P2–P7；
- `snapshot/commandlog/*_test.go`：跨写面的 command-id claim、重放和冲突；
- `catalog/*_test.go`、`cli/consume_flow_test.go`：C1–C7；
- `index/*_test.go`、`retrieval/*_test.go`：候选回读、basis、能力与 continuation；
- `cli/user_journey_test.go`：通过测试专用 embedded seam 验证共享应用语义；`cli/serve*_test.go` 和 remote CLI 测试验证产品 Server/Client 边界；
- `cli/command_evidence_test.go`：以生产 `cliSurface` 为分母的逐命令成功与风险分级边界报告；
- `cli/http_contract_inventory_internal_test.go`、`cli/http_surface_coverage_test.go`：以生产 route registry
  为分母的 HTTP 路由所有权、method、namespace 与 HTTP/Client 成功语义；当前分母以 `httpsurface.Patterns()` 为准，不固定手写数量；
- `dsh-plugin/scripts/agent-scenarios.json`：真实 Agent 验收的机器可读分母，登记六个核心角色、
  四个首次使用/概念问答和 `KC-AGENT-01` metric 权限 companion；
  runner 与清单漂移立即失败；
- `dsh-plugin/scripts/e2e_agent_roles.py`：真实 Agent 分别完成 source 发布、Workspace 治理检查、
  固定 pin 读取、audit/log/provenance 审计、坐标冲突恢复与越权写拒绝；每个角色保存回答和
  Skill/shell trace，最终状态 oracle 同时证明合法写入生效、越权写入未污染权威状态；
- `dsh-plugin/scripts/e2e_agent_questions.py`：真实 Agent 回答消费者心智模型、提供方
  接入边界和缺能力恢复问题；每题保存回答、Skill-only trace 和确定性语义 oracle；
- `internal/arch`：分层与术语守卫。

`make test` 通过证明共享应用语义、分层和 typed transport 合同；`make test-service-e2e` 提供
预配置与已授权角色的 Server/Client live 证据；平台仓创建、自有连接、首次准入、分享与发现搜索各有独立具名验证入口。不能用其中一条通过代替其他入口，也不能仅凭本轮已实现状态声称正式完整验收通过。
依赖外部服务或 Linux FUSE 的能力，只有对应 live 测试真实通过才可对外宣称；SKIP 不是 PASS。
`make test-all` 不包含付费 Agent、真实 Taihu、墙外数仓或规模资格验证。普通命令退出成功也不
自动证明所有功能点；运行报告没有观察到的库存项保持未验证。发布评审应选择与承诺对应的
分组并引用同一个运行目录，不能拼接各节点历史 `_results/latest.json` 宣称全绿。

## 本轮已实现入口与待核验边界

以下记录已落地行为和具名验证入口，不宣布同次完整验收已经通过。公开形状以各包 README、公开类型与正式 CLI/HTTP registry 为准。

- **客户端入口与普通用户登录**：成功登录保存默认 Server，按 Server 隔离会话；Taihu 浏览器授权与刷新经 Server broker，客户端不持有部署应用秘密。验证见 `TestLoginPersistsDefaultServerAndIndependentSessions`、`TestLoginSavedCredentialNeverCrossesServer`、`TestBrowserCodeCompletesThroughBrokerWithoutClientSecret`、`TestSavedLoginRefreshesWithoutClientSecretAndKeepsServer`、`TestServerUsernameBindingSurvivesRestartAndRejectsRecycledAccount`。真实 Taihu/外部 SSO 的连通、账号映射和登录配置仍需独立 live 证据；Gitea token 登录不等于每个部署的原生网页 SSO 已准备。
- **Snapshot 自助供给、连接与凭证**：平台仓按名创建，新的规范用户名对应同名 Gitea 账号或 Dolt 隔离空间；结果和我的仓列表可查管理地址。自有 Gitea 的 connect/show/check/rotate 只在批准的 provider 下验证并保存私有逐仓授权，固定数字仓身份与初始 commit；失败不替换旧绑定，过期凭证不阻止重启后的管理恢复。验证见前述托管旅程、`TestConnectionReadOnlyRotationRecoveryAndAuthorityBinding`、`TestRepositoryConnectionCLIRecoversExpiredCredentialsWithoutRebinding`、`TestRepositoryConnectionOnLiveGitea`。Dolt 管理入口要求平台配置 publicURL；自有远程 Dolt 尚无对应 adapter，不能用任意 Server 目录冒充支持。
- **查权与外部申请入口**：`admission show` 只返回调用方当前 grants、当前授权管理员和部署声明的外部申请 URL；KC 不维护 admission 请求/审批队列，授权管理员使用 `grant add` 发权。验证见 `TestAdmissionReportsOnlyCallerGrantsAndCurrentAdministrators`、`TestAdmissionCLIReportsCurrentGrantsAndExternalRequestRoute`、`TestRemovedCommandsAreRejected`。认证、登记、便携配方和 pin 本身不发权。
- **Catalog discovery typed 合同**：Catalog 范围 SEARCH 已从产品 CLI 退役；HTTP 内部仍保留 `catalogDiscovery` typed 合同，供批准的发现入口把配置的 `discoveryWorkspaceId` 解析为固定 pin。Server 验证精确配置语境，结果逐仓按当前正文权限交付；其他 Workspace、临时定义和缺 pin 不可冒用该语境。验证见 `TestCatalogDiscoveryContextRequiresExactPublishedWorkspaceAndAction`、`TestCatalogDiscoveryActualServerPinsSelectedSourcesAndMasksBodies`。普通用户 SEARCH 只接受 `--repo` 或 `--dataset`，不存在第三种 Catalog scope。
- **墙外 Connector runtime**：`integrationruntime` 与 `kc-integration` 已提供 build、activate、run、status、pause/resume、daemon，复用保存的 KC 登录，通过 typed Writer 发布。源客户端与领域映射保留在接入方 integration repo；运行方管理进程，不把运行宿主宣称为云端托管服务。验证见 `TestCommandBuildActivateRunReusesKCLogin`、`TestIntegrationRuntimePublishesThroughAuthenticatedWriterHTTP`、`TestPendingRecoveryKeepsCommandAndCheckpoint`、`TestCredentialsStayOutsideLedgerAndRotateAtRuntime`；完整操作与状态恢复合同见 `integrationruntime/README.md`。

## 当前已知缺口

这些是**实然落后于应然**，不是把设计改小。对外宣称时不得假装已经具备。

- **Gitea Knowledge READ**：tree 仓的精确读已经走 Writer 同 commit 写入的每对象
  `.kc/knowledge-locators/objects/*.entry`，不是 ListFiles 或全仓 manifest；10/1000 对象点读的
  authority 调用与解码字节相等。旧 whole-repository locator 只作迁移读兼容，普通写入失败关闭
  并要求显式迁移。`TestT12GiteaContract` 证明 Gitea 上 Reader/Writer 合同成立。缺的是 Gitea
  原生 ② 表（规模 profile，见 `SCALE_ARCHITECTURE.md`），以及 SEARCH 仍依赖 exact-basis
  检索投影（`R-01`/`R-02`），不是「Gitea 仓不能成为 Knowledge Repository」。
- **README 热状态与 discovery 关闸**：`KNOWLEDGE_PRODUCT_AND_SCHEMA.md` U6 / §3.5。`kc show` 只列仓库身份；README 是知识对象，默认 SEARCH 走该对象 `body` 的 `text`，不是文件 contains，也不展成库存 title/summary。缺的是投影 READY/lag claims。缺 README 不从库存抹仓，也不得由平台补写。`RETRIEVAL.md` 延期的是 SEARCH 的 Facet/total count，不能用来取消有界源发现，也不能把源发现改成对象 LIST。
- **State 投影控制收口进度**：change notice 入站合同是 `index.ChangeNotice`（仓/ref/可选 Address/可选 sourceRevision hint，拒绝正文）。`Controller.Notify` / `CatchUp` 与 Snapshot Desire 分钥；冷启动全量 `RefreshState`，notice 走 `RefreshStateObjects`。公开入口是 `kc operations projection notice` 与 `POST /operations/v1/projections:notice`。消费 SEARCH 仍不得 `RefreshState`。尚未收口的是 `PROJECTION_CONTROLLER.md` §11.3 Docker 首版（真实 Observer、Gitea、KC 重启）。`index-sync` 仍可用于 Snapshot EnsureAt、历史 pin、强制重建和排障，不再是动态 live 的唯一入口。
- **Stream projection / RetrievalPlan**：Aspect 可声明 Stream Binding。普通 READ 对 Stream 已失败关闭（`TestOrdinaryReadRejectsStreamBinding`）。缺的是 window/query 面与投影；Binding 里的 `protocol: mcp` 只是 ResourceDescriptor 字段，不是 MCP Gateway。
- **多实例 / MCP Gateway / 多语言 SDK**：`SERVICE_ARCHITECTURE.md` 的规模化拆分与 MCP 网关是方向；未落地记在这里，不是 §12 否决。公开 `append`/`stream` 命令与退役 HTTP 路由已保持 404（`TestAppendAndStreamSurfacesStayAbsent`）。
- **Tree Writer 写放大**：file-backed COMMIT 在 `knowledge/writer/treecodec.go` 仍 `ListFiles` 整棵知识树再写 manifest。Dolt 走 `ChangeStore` 增量。这是规模缺口，不否定 Gitea 精确 READ。
- **Dolt native/Tree 边界尚未完全分离**：`SCALE_ARCHITECTURE.md` 的目标要求 native Knowledge adapter 不实现 Snapshot TreeStore；当前 `knowledge/dolt/repository.go` 仍实现 TreeStore/DirectoryReader 并转发 ApplyTreeCommit。这是设计与实现的确定差距，不能因 native 点读/增量合同存在就宣称目标边界已验收；本轮只登记，行为修复需独立合同与反例。
- **公开入口仍叫 Loom**：`TERMINOLOGY.md` 禁止把 Loom 当产品名；实现里 `dsh-loom`、`dsh --profile dsh-loom`、`/api/loom/vfs` 仍在用。协议层不要跟着改回 Loom。
- **Schema breaking 迁移的合同与实现**：实现目前覆盖同一 Schema ID 的兼容演进；breaking 变更的身份/迁移边界在设计合同间仍有待评审分歧（本轮 REVIEW-03），不能宣称“原位 breaking”已经确定为唯一目标。先统一 owner 决策，再补迁移证据与验证，不用现有实现反向决定设计。
- State Binding 已有独立动态投影和双 basis（精确 READ、同 revision SEARCH hydrate）。change notice 与控制器第二条输入已收口（见上条）。尚未验收的是 `PROJECTION_CONTROLLER.md` §11.3 Docker 首版、Stream 窗口与多实例生命周期。
- `kc serve` 已按正式 namespace 形成模块化单体；进一步拆成独立进程是部署选择，不是新协议层。
- command-id 能覆盖当前进程/共享日志的知识写面重试；PENDING 可经显式 resolve/abandon
  核对，Bolt 保留清理不在启动时加载历史，ledger 读失败不会重放。多实例协调、分布式租约、
  灾难恢复，以及 attach/grant 等管理写入的统一重放，尚未形成生产验收。
- 已有 Go typed client。Catalog/命名知识集发现走 `/catalog/v1` 与 `kc catalog list`，单仓 Schema 发现走固定 basis 的 `/knowledge/v1/schemas:list` 与 `kc schema list`；维护读回走 `kc read --repo`；消费读走 `kc read --dataset`。精确历史重放抄回执 `--repo --commit`。
- Gitea adapter 为原子 ref CAS 使用短生命周期 `kc-wip/*` branch；Gitea 1.26 的异步 action notifier 可能在清理后记录“ref 不存在”，不影响 commit/ref 结果，但生产日志治理仍需改用无临时 branch 的底层 commit API。
- Linux 宿主 VFS 是可选文件体验，不是接入或消费协议成立的前提。VFS 不是 Writer，也不是 Catalog 成员条件；只读是选定的宿主投影合同。Plain 仓只解释组合阶梯。
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

每个 `kcfs` 进程按 `--dataset` Resolve 一次已发布 Dataset；所有 mount 共用该次冻住的 commit 清单，进程退出前不跟随 Repository HEAD。同一修订再次挂载仍是发布时的 commit；要新 bytes 须先 `dataset define --revision`。

| ID | 结果 | 机器可判定条件 |
|---|---|---|
| V1 | 附着任意已有项目 | 项目根只要求是可访问目录；非 mount 内容的 inode/bytes 不变 |
| V2 | 多目录组合 | 每个 `KnowledgeSetSource.Path` 是独立 mountpoint；可来自不同 Repository |
| V3 | 同仓多子树 | 同一 Repository 可投影多个不重叠的 `SubPath`；pin 中仍只有一个 commit |
| V4 | 一致视图 | `cat`、`rg`、IDE 和 Agent 读到相同 bytes；所有 mount 使用同一 pin |
| V5 | 固定版本 | mount 期间上游 ref 推进不改变 bytes；同一 Dataset 修订重启 `kcfs` 仍是发布冻 commit；要新数据须 `dataset define --revision` 再挂 |
| V6 | 只读 | create/write/truncate/rename/remove 均失败，Repository ref 和原文件不变 |
| V7 | 授权 | 先检查 `dataset.resolve`，再逐 Repository 检查 `file.read`；无权成员不进入 plan/mount |
| V8 | 消费语义视图 | `--view semantic` 不要求 Repository mount path；固定 pin 生成 `knowledge/<source>/<entity-plural>/*.yaml`，保留 `_kc` 坐标且不暴露 Canonical 单元信封 |
| V9 | 可选文件视图 | 插件缺省保存结构化 pin；用户显式开启文件视图时才请求宿主挂载，已有挂载可显示/隐藏或移除。文件开关不发权，不改变任务固定版本 |
| V10 | 无 Agent 专用 VFS | DSH 不替换标准 filesystem/search 工具，不导出第二套 `loom-fs` / `loom-search` |
| V11 | 发现后添加 | 未配置 `KC_WORKSPACE` 也能展示可见 Catalog、Repository、Schema 和命名知识集；添加自选来源或命名集建立结构化固定 pin，不要求 FUSE。文件挂载可选，检查更新与采用更新分开，失败保留原上下文 |
| V12 | 安全路径 | 拒绝绝对路径、`..`、反斜杠、NUL、根挂载、重叠 mount 和 symlink 穿越 |
| V13 | 宿主失败可解释 | 缺 `/dev/fuse`、`fusermount3`、TreeStore capability 或非空 mountpoint 时明确失败 |
| V14 | 首次使用可发现、可恢复 | 新项目的“知识”侧栏可展开目录但文件树默认隐藏且不预扫；未接入时可自主多源选择或采用命名知识集，并明确库存截断；Skill 从自然语言引导 SEARCH→Canonical READ，不要求用户先懂命令 |

可选文件挂载的环境要求：Linux 可访问 `/dev/fuse`，安装 `fusermount3`；容器显式暴露设备和挂载 capability；每个 mountpoint 不存在或为空。mountpoint 是目录，不支持单文件 mount，也不允许挂到项目根。

```bash
go test ./workspacefs ./catalog ./cli ./internal/arch -count=1
npm --prefix dsh-plugin run typecheck
npm --prefix dsh-plugin test
./scripts/e2e-kcfs-linux.sh
./scripts/e2e-kcfs-docker.sh
```

若未来提供可写文件体验，应使用显式 checkout/overlay + reconcile/commit，保留 `expectedTargetCommit`、`command_id`、逐 Repository 结果和冲突恢复。不能把普通 FUSE write 直接解释成协议 COMMIT，也不能伪装跨 Repository 原子事务。
