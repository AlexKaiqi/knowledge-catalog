# Knowledge Catalog MVP 验收

本页回答：当前实现是否足以让接入方独立供给知识，让人、应用和 Agent 按任务采用，
保留可复核依据，并把使用中发现的问题带回修订流程。产品原则与目标形态由
[架构总览](core-architecture.md) 拥有；本页负责已有能力、采用前提与尚未闭环的差距。

本页综合用户任务、实现边界与声明的验证入口，不承担用例库存或执行结果台账。
验证方法和结果判读见 `test-catalog.md` §0.2；具名 Test 可由 `make validation-inventory`
定位，执行是否通过必须引用同次 run-id、scope、源码指纹与必要的 live/Agent 原始产物。
没有这些信息的历史通过描述，不作为当前版本的验收凭据。

## 结论与边界

| 形态 | 当前结论 | 承诺边界 |
|---|---|---|
| 单实例 Server/Client 参考实现 | **预配置与既有授权下可用** | 本机部署也由 Client 经 typed API 进入 Server；lakeFS 提供 Snapshot authority 与精确 Knowledge 回读；SEARCH/RELATIONS 只经 exact-basis Retriever 发现候选，再按同一 commit 回读 Canonical。未配置对应能力时明确失败，不扫描降级 |
| 接入方、消费方全程自助 | **自助入口已有实现，按环境与旅程验收** | 已提供保存登录、显式首次准入、按名申请 lakeFS 托管仓及管理地址、自有 lakeFS 连接与轮换、受限分享和可选文件挂载。既有修复记录不替代当前版本验收；外部 SSO、各 authority/宿主及恢复承诺分别引用正式证据 |
| 共享服务试点 | **有条件可用** | 需部署方提供 TLS、可信认证器、备份与单实例写入约束；独立 Gitea 认证和远程 lakeFS authority 可用，但不是完整生产平台 |
| 多实例生产服务 | **尚未验收** | 跨进程幂等/租约、独立 Dataset 登记服务/Knowledge 服务部署、多语言 SDK/MCP、容量与故障演练仍不在当前保证内；Go typed Client 已有 |

已有协议、发布与固定版本消费能力不等于全程自助产品已验收，也不表示每个 adapter 或宿主
都已生产就绪。按[架构总览](core-architecture.md)与[服务边界](service.md)，部署方一次性建立
运行能力与显式策略，后续日常任务不要求其逐仓配置或代执行。验收应覆盖两条完整旅程：

1. **接入方**通过交付客户端登录，自助申请平台 Snapshot 或连接自己的 Snapshot，完成验证、登记与策略授权，再发布、修改、移除和恢复带身份、Schema 与来源的知识。
2. **消费方**通过同一客户端登录，发现可见来源，直接使用获准单仓或采用已发布 Dataset；已知身份直接读，不知道身份再检索。每次请求使用固定依据，宿主保存长任务上下文；重新采用发布版才更新已打开的固定视图。独立 CLI 命令间需要复核时保存返回的 Repository、commit 与对象身份，不要求手工 pin 文件。

## 产品价值的任务验收

协议合同检查身份、版本、范围、授权和失败语义；任务验收检查这些保证是否降低知识供给
与使用成本。两种证据分别成立，不把命令退出成功或模型回答流畅当成任务正确。

| 任务阶段 | 应观察的用户结果 | 必须同时覆盖的反例 |
|---|---|---|
| 发布并复用 | 接入方发布一份带模型、来源与变更说明的知识，两个获准消费者可独立使用 | 日常接入需要管理员逐仓改部署配置；消费者必须取得存储机器凭证 |
| 按任务交付 | 多个来源的选定部分组成 Dataset，身份、范围与版本仍可区分 | 未选文件被补入；同名对象被覆盖；组合隐式取得整仓权限 |
| 完成消费任务 | 使用者找到所需定义和关系，取得有依据的内容或外部观察，结果引用可重读 | 搜索与回读混版；缺能力当零命中；旧观察冒充当前完整状态 |
| 修订与采用 | 具体反馈关联访问依据，维护者核对后发布修订，新任务采用新版，旧任务仍可复核 | 反馈直接变成知识或发权；推断自动回写；撤权后旧引用仍放行 |
| 故障后续用 | 服务替换、响应丢失或索引故障后恢复原范围与操作结果 | 重试重复提交；缓存丢失补建空权威；派生失败撤销已接受提交 |

任务评估记录接入耗时、任务完成质量、引用可复核性、更新采用与恢复成本；采用相同
知识依据、授权范围和预算比较消费路线。协议场景负责其不变量，领域材料留在已有场景
的 `_materials/`。当前清河材料和具名测试提供组成能力的证据入口，整条任务收益与
反馈修订循环仍需专门运行，不能据文档或局部用例宣称已经验收。

## 当前可执行旅程：已有部署与显式准入

以下展示参考实现当前能执行的路径，不能当作全程自助验收。部署方仍须先持久保存声明配置、
远端 Dataset 登记服务 Snapshot 权威和独立服务状态，再显式初始化；部署可启用托管池、首次准入与自有 lakeFS 连接策略，用户随后通过公开入口申请或连接来源。
配置形状和示例见仓库 README。已有部署只恢复并启动，不必重放首次搭建。这里的 local
身份仅用于本机测试配置，不是普通用户的产品登录：

```bash
kc deployment init --config deployment.yaml   # 首次：建立 Dataset 登记服务 和首个管理主体
kc deployment status --config deployment.yaml
kc serve --config deployment.yaml            # 终端 A：只恢复

export KC_SERVER_URL=http://127.0.0.1:7380      # 终端 B
kc login --mode local --as admin    # 示例配置显式使用 auth: local
kc attach --repo kr://acme/public/core
```

Dataset 登记服务 接入在 Server 内只读验证已配置的 Repository，随后原子登记成员；不创建业务 Snapshot，
也不因普通 attach 授予维护或消费权限。新增自有 lakeFS 连接使用
`kc create --url --credential-file`，只允许部署批准的 provider；逐仓授权保存于 Server 私有
存储，无需逐仓修改部署配置。
`local` 命令和独立 `catalog repo register` 已退役。

### 平台仓申请与重部署续用

部署配置声明托管存储池及 creatorActions，普通接入方已获当前 Dataset 登记服务 创建准入后，可通过
`kc create --name <名称>` 申请新仓；只有多个池时需选择 `--store`。Server 生成仓身份与恢复
坐标，返回管理地址。成功后显式 attach，再 PUT 或 `writer commit --dir`；实例替换从耐久创建账恢复
连接与原结果，创建重放不补回已撤销权限。

正式组件验证入口为 `TestManagedRepositoryProviderCreatesPublishesAndResumes`、`TestManagedProductHumanSelfServiceOnLakeFS`、`TestRepositoryConnectionCLIRecoversExpiredCredentialsWithoutRebinding` 与 `TestRepositoryConnectionOnLakeFS`，检查创建、发布、缓存替换、耐久恢复、重放、CAS 与撤权。Gitea 存储及专属 live 测试已退役；这些 lakeFS 协议夹具的结果不能代替真实部署证据。是否验收通过仍须引用包含对应测试和实际后端验证的同次源码记录。

### 知识接入方

已登记且具备仓维护和回读权限时，接入方可通过 Client 自行完成目录提交，再按对象引用读回。Dataset 面向消费组合，不是写入前置条件。接入方提交完整对象内容，不必命名 Snapshot ref。Client 对照当前版本求差后提交；可选 `kc diff --dir` 看同一对照，不是提交前必经步骤，也不写出 ChangeSet。

```bash
kc writer commit --command-id source-1 --repo kr://acme/public/core --dir ./drafts
kc writer head --repo kr://acme/public/core
kc read 'urn:kc:object:kr%3A%2F%2Facme%2Fpublic%2Fcore:runbook%2Fpayment-oncall'
kc provenance --repo kr://acme/public/core --object runbook/payment-oncall
```

等价的单条 PUT 仍可用。需要 SEARCH 时先发布带 `text` AccessHints 的 `schema/*`。批量文件或外部源仍只有一条写边界：Client 对期望正文求差，或墙外 `connector.Preview` 计算 diff，由 `commit --command-id` 提交。采集器、源凭证和业务映射留在底座之外。

日常修订保持同一 Address，改草稿目录并用新的命令身份发布；重试同一笔提交保留命令身份与
内容，结果不确定时查询原 Receipt。并发冲突后读取最新版本、比较合并并重新提交，不能
取消前置条件覆盖别人。显式移除与历史内容恢复同样经 Writer；恢复是新的发布，不倒退
权威历史。完整维护语义见 [知识读写](knowledge.md)。

### 当前授权准备与可选共享知识集

托管仓创建者能力来自部署明确配置的窄动作策略，并形成可撤销的仓级规则。用户通过
`admission show` 查询当前 grants 与外部申请入口；审批后由授权管理者运行 `grant add`。
命名知识集是可选配方，不是自行选源的前置条件。下列为
授权管理者的操作示例，消费方不运行这些命令：

```bash
kc grant add --principal consumer --action inventory.read,dataset.resolve,file.read
kc grant add --principal consumer --action knowledge.read,knowledge.search,knowledge.schema.read --repo kr://acme/public/core
kc dataset define --dataset oncall --revision 1 --source kr://acme/public/core  # 可选共享配方
```

长寿命 `kc serve` 会把 live 检索投影追到各源 published HEAD；`projection sync` 只用于历史
commit、强制重建和排障，不是每次发布或消费的步骤。精确 READ 不依赖检索投影。

### 知识消费方

已有 Dataset 登记服务 使用与消费授权时，用户可发现来源，选择获准单仓或已发布 Dataset。
调用方不必预知 Dataset 登记服务/Dataset id。`kc show` 的 `repositories` 是身份列表（及 `schemaCount`），不含 README
title/summary，也不含宿主路径或 Snapshot selector。检索投影由服务维护，不是消费命令。

```bash
kc show                         # 发现服务的唯一库存
kc show                                 # 发现知识源与已有知识集
kc schema list --repo <发现的源>
kc search --repo <发现的源> --query 冻结窗口
kc read <命中来源对象的URN[#/locator]>
```

SEARCH 使用 Repo 或 Dataset 范围；READ 使用完整对象引用，可另限定 Dataset。来源选择属于 Dataset 发布配方，
不是消费命令的临时选源入口。获准维护者先用 `dataset define` 发布组合，消费者随后按
名称使用；宿主可保存固定任务上下文，不要求 FUSE 或产品 argv 中的 pin 文件。

Dataset 默认读取最新已发布版本，每次请求只解析一次；两条独立 CLI 命令之间可能有新
发布。需要复核某次搜索时保存命中坐标，具备仓级权限后按 `--repo --commit` 重读；
仅有 Dataset 消费权不能据此取得整仓或历史读取权。已打开的固定上下文不随上游更新
移动，采用更新由宿主明确处理；撤权仍在下一请求生效。SEARCH 命中须在同一依据回读 Canonical。`partial`
必须附带 claims；能力不足返回 `CAPABILITY_UNSATISFIED`，不能伪装成零命中。精确 READ
无法诚实表达缺失成员时 fail closed。

## 产品 MVP 必须满足

### 接入方

| ID | 用户结果 | 机器可判定条件 |
|---|---|---|
| P1 | Repository 能独立接入 | 接入方经客户端自行申请平台 Snapshot 或连接自己的 Snapshot，按显式策略完成验证、登记与维护授权；登记不初始化 Snapshot、不改 HEAD、不复制正文、不隐式发权。平台仓按名创建和自有 lakeFS 连接均有 typed 入口与具名正式旅程；连接失败保留旧绑定，凭证轮换不改变 authority |
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
| C1 | 能发现消费入口 | 连接服务后 `kc show` 直接列出已登记 Repository 与 Dataset；不要求选择或切换空间，库存不复制正文 |
| C2 | 一次任务版本一致 | 消费方用 `--dataset` 对准已发布配方，或用 `--repo` 对准单仓；命令开始时内部 resolve 一次（`V-01`），回执带 `commit`。精确历史重放抄 `--repo --commit`。产品 argv 不出现 `kc pin` / `--pin`。Dataset 发布冻 commit（`KS-01`）；权限仍按当前 grants 求值（`KS-02`） |
| C3 | 多仓读取不覆盖 | 同 `object_id` 的成员结果并集返回，public/group/personal 不互相覆盖 |
| C4 | 搜索结果可信 | Provider 只给 CandidateRef；公开 hit 在 SearchView basis 回读 Canonical，并带 version/evidence/completeness |
| C5 | 能区分空、缺能力和部分结果 | 零命中、`CAPABILITY_UNSATISFIED`、`partial + claims` 形状不同 |
| C6 | 可追溯 | READ、SEARCH、LOG 和 GET_PROVENANCE 都绑定明确 Repository commit |
| C7 | 授权不越层 | `read-workspace` 不隐式授予成员 Repository 的 `read`；裸 READ 不返回假完整结果 |
| C8 | Agent 能解释并选入口 | 自然语言询问核心概念、接入边界或 SEARCH 缺能力时，Agent 依据公开系统指引与 CLI help 给出可判定的正确答案和最小下一步；真实模型语义验收未执行 |

### 服务与运维

| ID | 用户结果 | 机器可判定条件 |
|---|---|---|
| S1 | Transport 唯一 | 业务 CLI 是 typed Client，即使本机部署也不打开 Home；HTTP route 调用共享应用服务，不依赖 CLI parser/command table |
| S2 | 身份来源可信 | 部署配置显式声明认证；local 模式只接受 `X-Kc-As`，taihu/gitea 模式只接受已验证 `Authorization`；错配失败关闭。新部署由显式 init 建立首个管理主体，后续授权经 Server，重部署恢复既有授权 |
| S3 | 可判断存活与就绪 | `/livez`、分 surface `/readyz`、`/metrics` 不依赖知识响应正文 |
| S4 | 权威与派生可区分 | Snapshot 保存知识，独立 Dataset 登记服务 Snapshot 权威保存组合；配置、授权、gate、幂等/Receipt、控制面与审计证据独立持久保存。工作 clone 与索引可丢可重建，缺失耐久状态失败关闭 |
| S4a | 更换实例仍能继续任务 | 删除整个 cache 并换进程后，Dataset 登记服务、既有 pin、授权、Receipt 重放与治理状态仍可恢复；启动不补建空授权或业务 Snapshot |
| S5 | 分层可执行 | `internal/arch` 阻止 Dataset 登记服务 感知知识协议、Writer 依赖 Retrieval 等反向依赖 |

## 自动化证据

```bash
make test          # lakeFS HTTP 夹具的普通/索引场景 + 真实 OpenSearch
make deploy-local-scenes # 独立真实 lakeFS 部署场景；需已有 local 测试栈
make test-contracts # 原 component + boundary + 应用/transport 合同组合
make test-cover    # short suite、公开动词覆盖和 statement coverage 门禁
make test-race     # 并发敏感包的 race detector
make test-server-web # Server 浏览器登录与 Web 入口测试
make test-kcfs-e2e # Docker Linux /dev/fuse：kcfs 真实生命周期
make test-service-e2e # 真实 Gitea 认证、lakeFS/OpenSearch、双身份 HTTP 旅程
make test-all      # 夹具、真实 lakeFS 部署、混合合同、Server Web 与其它 adapter/FUSE；需 local 测试栈
```

关键证据入口：

- `cli/mvp_acceptance_test.go`：通过测试夹具固定两条最短角色旅程；
- `cli/deployment_recovery_test.go`：`TestDeploymentSurvivesInstanceReplacement` 与 `TestDeploymentMissingDurableStateFailsClosed` 验证正式 Run/HTTP 的实例替换与缺失状态边界；`cli/deployment_contract_test.go` 守卫退役命令和显式配置入口；
- `cli/temporary_knowledge_test.go`：`TestProductRepoCommitFreezesKnowledgeAndCurrentPermissions` 验证产品 `--repo --commit` 历史消费、上游更新隔离与当前权限；`cli/remote_knowledge_basis_test.go` 验证所有 Knowledge DTO 传递同一任务定义和 pin；
- `cli/server_client_only_test.go`：`TestRemoteProviderReadBackAndConsumerDiscovery` 按部署 → 接入方发布 → 治理方 compose/grant → 消费方发现 的顺序，用产品 `--server` Client 走 commit/read 与 list/show/schema list/search/read；`TestServeProjectionWorkerCatchesCommitWithoutSync` 证明长寿命 serve 在无手工 `projection sync` 时仍能追上 published HEAD；角色命令与库存 JSON 不得出现 `--home`、宿主路径或 Snapshot selector，显式任务 pin 另承载固定版本；消费 SEARCH 失败不得教运维命令；
- `cli/service_roles_live_test.go`：真实 Gitea 认证、lakeFS/OpenSearch 上的 provider/consumer 独立身份、固定 pin 与更新隔离；
- `object/writer/*_test.go`：P2–P7；
- `snapshot/command*_test.go`：跨写面的 command-id claim、重放和冲突；
- `snapshot/dataset/*_test.go`、`cli/consume_flow_test.go`：C1–C7；
- `discovery/projection/*_test.go`、`discovery/*_test.go`：候选回读、basis、能力与 continuation；
- `cli/user_journey_test.go`：通过测试专用 embedded seam 验证共享应用语义；`cli/serve*_test.go` 和 remote CLI 测试验证产品 Server/Client 边界；
- `cli/command_evidence_test.go`：以生产 `cliSurface` 为分母的逐命令成功与风险分级边界报告；
- `cli/http_contract_inventory_internal_test.go`、`cli/http_surface_coverage_test.go`：以生产 route registry
  为分母的 HTTP 路由所有权、method、namespace 与 HTTP/Client 成功语义；当前分母以 `httpsurface.Patterns()` 为准，不固定手写数量；
- `internal/arch`：分层与术语守卫。

DSH 宿主集成及专属真实模型运行器已退役，不再构成当前执行分母。C8 的自然语言解释、入口
选择与恢复目标，以及 `VALUE-01` 的真实任务效果仍保留为未执行缺口；当前没有替代运行器或
新的模型通过证据。`.data/scenes/` 的 `Agent as` brief 保留，Go Then 仍只判协议结果。
未来模型验收须登记角色与任务分母，保存回答、执行 trace 和独立 oracle，同时证明合法写入
生效、越权写入未污染权威状态；普通协议测试或模型自评不能代替这些证据。

`make test` 提供 lakeFS HTTP 夹具下普通/索引场景的证据，不代表全部合同或真实 lakeFS 部署；后者由 `make deploy-local-scenes` 验证。`make test-contracts` 保留共享应用语义、分层和 typed transport 合同；`make test-service-e2e` 提供
预配置与已授权角色的 Server/Client live 证据；平台仓创建、自有连接、首次准入、分享与发现搜索各有独立具名验证入口。不能用其中一条通过代替其他入口，也不能仅凭本轮已实现状态声称正式完整验收通过。
依赖外部服务或 Linux FUSE 的能力，只有对应 live 测试真实通过才可对外宣称；SKIP 不是 PASS。
`make test-all` 不包含付费 Agent、真实 Taihu、墙外数仓或规模资格验证。普通命令退出成功也不
自动证明所有功能点；运行报告没有观察到的库存项保持未验证。发布评审应选择与承诺对应的
分组并引用同一个运行目录，不能拼接各节点历史 `_results/latest.json` 宣称全绿。

## 本轮已实现入口与待核验边界

以下记录已落地行为和具名验证入口，不宣布同次完整验收已经通过。公开形状以各包 README、公开类型与正式 CLI/HTTP registry 为准。

- **客户端入口与普通用户登录**：成功登录保存默认 Server，按 Server 隔离会话；Taihu 浏览器授权与刷新经 Server broker，客户端不持有部署应用秘密。验证见 `TestLoginPersistsDefaultServerAndIndependentSessions`、`TestLoginSavedCredentialNeverCrossesServer`、`TestBrowserCodeCompletesThroughBrokerWithoutClientSecret`、`TestSavedLoginRefreshesWithoutClientSecretAndKeepsServer`、`TestServerUsernameBindingSurvivesRestartAndRejectsRecycledAccount`。真实 Taihu/外部 SSO 的连通、账号映射和登录配置仍需独立 live 证据；Gitea token 登录不等于每个部署的原生网页 SSO 已准备。
- **Snapshot 自助供给、连接与凭证**：平台仓按名创建，使用规范用户名进行 lakeFS 托管供给；结果和我的仓列表可查管理地址。自有 lakeFS 的 connect/show/check/rotate 只在批准的 provider 下验证并保存私有逐仓授权，固定后端仓身份与初始 commit；失败不替换旧绑定，过期凭证不阻止重启后的管理恢复。验证见前述托管旅程、`TestConnectionReadOnlyRotationRecoveryAndAuthorityBinding`、`TestRepositoryConnectionCLIRecoversExpiredCredentialsWithoutRebinding`、`TestRepositoryConnectionOnLakeFS`。托管管理入口要求平台配置 publicURL；不能用任意 Server 目录冒充支持。
- **查权与外部申请入口**：`admission show` 只返回调用方当前 grants、当前授权管理员和部署声明的外部申请 URL；KC 不维护 admission 请求/审批队列，授权管理员使用 `grant add` 发权。验证见 `TestAdmissionReportsOnlyCallerGrantsAndCurrentAdministrators`、`TestAdmissionCLIReportsCurrentGrantsAndExternalRequestRoute`、`TestRemovedCommandsAreRejected`。认证、登记、便携配方和 pin 本身不发权。
- **库存发现 typed 合同**：库存范围 SEARCH 已从产品 CLI 退役；HTTP 内部保留受信的库存发现合同，供批准的发现入口把配置的 `discoveryWorkspaceId` 解析为固定 pin。Server 验证精确配置语境，结果逐仓按当前正文权限交付；其他 Dataset、临时定义和缺 pin 不可冒用该语境。验证见 `TestInventoryDiscoveryContextRequiresExactPublishedWorkspaceAndAction`、`TestInventoryDiscoveryActualServerPinsSelectedSourcesAndMasksBodies`。普通用户 SEARCH 只接受 `--repo` 或 `--dataset`，不存在 Catalog 选择维度。
- **墙外 Connector runtime**：`integrationruntime` 与 `kc-integration` 已提供 build、activate、run、status、pause/resume、daemon，复用保存的 KC 登录，通过 typed Writer 发布。源客户端与领域映射保留在接入方 integration repo；运行方管理进程，不把运行宿主宣称为云端托管服务。验证见 `TestCommandBuildActivateRunReusesKCLogin`、`TestIntegrationRuntimePublishesThroughAuthenticatedWriterHTTP`、`TestPendingRecoveryKeepsCommandAndCheckpoint`、`TestCredentialsStayOutsideLedgerAndRotateAtRuntime`；完整操作与状态恢复合同见 `integrationruntime/README.md`。
- **SEARCH 召回策略合同（2026-09）**：请求级召回策略闭集 `lexical`/`semantic`/`hybrid`（`discovery/recall.go`，`SearchRequest.Recall`，省略即 lexical）已落：`ValidateSearch` 拒绝未知策略与 reserved `hybrid`（USAGE_INVALID），`semantic` 要求 MATCH 文本叶（legacy/typed expression 同规则）；缺少向量投影或 embedding 提供方时失败关闭，不静默降级 lexical。Dataset 通道授权先行，召回资格门由 `embedSemanticQuery` 与 Dataset 执行器的 `Authorize` 前置表达。验证见 `TestValidateSearchRecallStrategyClosedSet`、`TestValidateSearchHybridReserved`、`TestValidateSearchSemanticRequiresMatchText`、`TestSearchExecutorFailsClosedOnSemanticRecall`、`TestDatasetSearchExecutorSemanticAuthorizesFirstThenFailsClosedWithoutProvider`。向量投影、embedding 与 Approximate 路径已由下述垂直切片接入。
- **SEARCH 语义召回垂直切片（2026-09，第二批）**：embedding 提供方合同（`retrieval.Embedder` +
  llmhttp embeddings 适配器，OpenAI 兼容 /embeddings，环境配置不落盘）与 OpenSearch k-NN 派生投影（`semantic_vector` knn_vector 字段，构建批次派生向量，模型/维度环境配置）已落。单仓 `semantic` 路径：执行器取查询向量 → `index.SemanticWindowAtContext` → k-NN top-K 窗口（单次有界，approximate lane 证据）→ 授权内 hydrate；`MarkApproximate` 强制 partial 并披露窗口与模型身份。验证见 fake embeddings 服务 + 真 OpenSearch 的 `TestSemanticWindowRecallApproximateEnvelopeAndFailClosed`（fail-closed ×3、近似信封、最近邻排序）、`TestSemanticExecutorEndToEndOverVectorProjection`（执行器端到端）与 llmhttp 适配器测试。Dataset 成员窗口、证据披露及 HTTP 链路已有下述第三批实现；真实 embedding 模型的部署验收仍属独立入口的合同，环境接线测试不能代替它。
- **TRAVERSE 有界邻域闭包（2026-09）**：`kc traverse` 与 `POST /knowledge/v1/traverse:query`（Dataset pin 与单仓 commit 两个范围通道，client SDK `KnowledgeService.Traverse` 与远程 typed 分发同接入）按[检索](retrieval.md)的关系遍历设计提供有界类型化遍历：nodes 按对象去重带最小跳深、edges 保留关系坐标与完整端点、范围外 frontier 显式 boundary、游标续传闭包增量且绑定清单摘要；不枚举路径、不扩大授权。验证见 `TestTraverseClosureDeduplicatesAndReportsBoundaries`、`TestTraverseHopCeilingAndMinHopsFilter`、`TestTraverseContinuationBoundToDatasetScope`、`TestTraverseStartOutsideScopeFailsClosed`、`TestRepoTraverseStopsAtRepositoryBoundary`、`TestTraverseNeighborhoodClosureAcrossChannels`（OpenSearch live 夹具）与场景 probe `traverse-closure-follows-relations.feature`（`dataset-query-principals-granted`）。`retrieval.jsonl` 证据 lane 已接入 `RetrievalOperatorTraverse`，见下述证据覆盖。
- **统一访问第三批（2026-09）：Dataset 语义扇出、检索证据 lane 补全、HTTP 部署链路**：Dataset 通道 `semantic` 已落（`discovery/service/dataset_search_semantic.go`）：授权先行，一次查询向量，每个固定成员各取一次有界向量窗口（成员无向量面或 State 成员整体失败关闭，不静默降级），按成员序 rank interleave 合并（k-NN 分数只局部可比，不声明跨成员全局排序），Approximate 交付并披露成员窗口数与模型。检索证据 lane：`observability.RetrievalOperatorTraverse` 接入（候选窗 = 节点身份摘要 + 关系体摘要，completeness 按页契约 exhausted/partial），语义召回在 SEARCH 事件 logicalRequest 披露 `recall` 策略、claims 披露近似与模型；证据覆盖行已更新（[服务边界](service.md)）。HTTP 部署链路：`TestHTTPDatasetSemanticRecallServesMemberVectorWindows` 走真实 `HTTPHandlerWithOptions` + 环境 embedding 能力 + 真 OpenSearch 向量投影（近似信封、lane 证据、lexical 不变、无提供方 fail-closed），`TestHTTPServerOptionsFromEnvBuildsEmbeddingProvider` 锁 serve 环境接线。质量闸：QUALITY-01 收口（含 DTO 信封共享 embed 类型收敛，JSON wire 契约不变），`make quality` 全绿。

## 当前已知缺口

这些是**实然落后于应然**，不是把设计改小。对外宣称时不得假装已经具备。

### 对象发布与发现的完善次序

[对象供给全景](object-system.md) 保留从发布到释放的目标，近期交付范围为对象发布与发现。
以下是现有机制与目标合同的差距，不表示本轮已实现新 API 或已运行新业务旅程。

| 完善顺序与位置 | 现有基础与缺口 | 需要交付的可观察结果 |
|---|---|---|
| 先确定供给范式：领域 Schema、普通 Aspect/Member、系统说明 | Writer 已有身份、结构与来源校验，但尚无统一的业务供给角色、最小描述及可供性发布约定；合同记录不能自动充当业务对象 | 图片、分散模块、外部表能表达边界、用途、组成/来源、能力与选择信息；对象角色由声明解释，不从目录或文件数推断；系统说明同步而不虚构运行能力 |
| 再明确可解释发布保证：[Writer](../../object/writer/README.md)、[Interface 解析](../../interfaces/implementation.go) | Writer 的结构校验与 Reader 的固定合同/Schema 解析分别存在；尚未形成对象供给的统一依赖检查保证 | 发布的供给在明确固定范围内可解释，普通 Schema 同仓、跨仓合同采用保留来源；结构非法、解释依赖缺失、处理器未装配分别有证据；不检查所有关系端点或要求外部源在线 |
| 选定能力发现写法：[Schema](../../object/schema.go)、[查询合同](../../discovery/spec.go) | 字段 access 与类型化过滤已有；内置 LiveAccess 的实现引用可过滤，独立实现未有统一查询约定 | 按实际实现声明发现能力，说明里仅提及能力不命中；字段上下文、成员解释、合同来源与采用版本明确；固定范围内解析到同一合同依据的合法引用不因字面不同漏匹配，跨来源同名合同或不同采用版本不自动合并；仍只有字段 access 一种索引写法 |
| 完成发现交付与新用例：[结果合同](../../discovery/result.go)、[场景规范](../../.data/scenes/README.md) | SEARCH 已有对象候选和同依据回读，已知对象也可有界发现 Interface；异构供给的用途/能力发现与固定回读任务尚无完整证据 | 同一消费者经 Server、显式主体完成发布后的检索、读回与关系定位；结果分清业务对象和合同记录，保留命中单元及版本；未就绪不冒充零命中，旧 pin 仍按旧供给解释 |

精确读、字段查询、关系与 Interface 分派是可复用机制，不等于上述对象范式已经验收。
供给声明发现不为证明运行可用性探测来源，也不取得租约或推导执行权限；既有显式动态
查询继续遵守观察合同。现有字段命中不证明精确命中某个 Member，该证据须另行补齐。实际材料访问、领域处理正确性、
业务操作恢复及高级能力治理分别取得后续证据。可供性结构、供给角色与能力查询的具体
形状须在公开协议和 Conformance 中选定，不用设计正文里的示意直接扩展公开返回值。

新用例由已有场景前态承接，材料放在对应节点的 `_materials/`，保留清河数仓用例。
只有多个用例确实复用构建条件时才提取新共享状态；Interface/对象系统产品条目与具名
断言的关联随用例维护，不能把组件 Echo 回归写成多媒体对象发布发现旅程已通过。
实施次序与认领由 TASK 的 OBJECT-DESIGN-01 后续建议承接。

### 优先完善的用户结果

| 缺口与位置 | 对使用者的影响 | 处置与完成依据 | 执行入口 / 触发条件 |
|---|---|---|---|
| Dataset 服务版的部署恢复：`knowledgeapp.DatasetPublisher`、`invocation.VerbDefineKnowledgeSet` 与 `home.PrepareDataset` | 正式入口已接静态投影准备后采用；未启用检索时仍可发布文件，动态覆盖与故障资格不能据此推导 | 用真实部署证明准备失败保留旧版、重启接续与已接受版本的持续服务；动态就绪另按观察合同验收 | `DATASET-02`；扩大部署恢复或动态搜索承诺前 |
| 动态信息的覆盖、时效与复核：资源访问、投影控制与观察保留 | 当前值可能过时，通知成功不能证明查询范围完整或过去观察可重读 | 按 owner 补范围级证据、保留与失联处理，再验收真实 lakeFS/runtime/Observer/KC/OpenSearch 的恢复链；默认时效按 `REVIEW-04` 裁决 | `DOC-20`、`DYN-01`；承诺可信动态消费前 |
| 真实任务效果与反馈修订循环：应用消费和访问证据 | 组成能力的合同正确，仍不能证明使用成本下降或修订能改善后续任务 | 在相同依据、授权和预算下运行多源任务，关联反馈、正常修订、新版采用和历史复核，报告质量与总成本 | `VALUE-01`；扩大应用或 Agent 任务效果承诺前 |
| 容量、历史增长与故障恢复：Snapshot authority 与耐久状态 | 后端名称和小规模点读证据不能给出可依赖的生产支持范围 | 从可重复基线逐档扩量，记录维护成本、瓶颈、备份与恢复结果，限定最大通过范围 | `SCALE-01`、`DOC-15`；发布规模或生产 SLA 前 |

这里维护产品缺口；任务认领、依赖与未裁决政策在根 TASK 中维护。以下列出已有技术基础
及具体限制，不把已退役后端或已修复信号继续列成活动缺口。

- **Snapshot 上的 Knowledge READ**：Reader 解释固定 commit 的知识定位内容，存储本身不解释对象身份。lakeFS 的 Reader/Writer 合同由 `TestLakeFSRepositoryContract` 与 `TestLakeFSWriterContract` 钉住；SEARCH 仍依赖 exact-basis 检索投影。Gitea 存储已退役，不再将其原生表或规模优化列为活动缺口。
- **README 热状态与 discovery 关闸**：[知识读写](knowledge.md) 的仓自描述与 [CLI 交互](cli.md) U1。`kc show` 只列仓库身份；README 是知识对象，默认 SEARCH 走该对象 `body` 的 `text`，不是文件 contains，也不展成库存 title/summary。缺的是投影 READY/lag claims。缺 README 不从库存抹仓，也不得由平台补写。[检索](retrieval.md) 延期的是 SEARCH 的 Facet/total count，不能用来取消有界源发现，也不能把源发现改成对象 LIST。
- **State 投影控制与部署证据**：State Binding 已有独立动态投影与声明/观察双依据，支持精确 READ 和同 revision SEARCH hydrate。change notice 入站合同是 `index.ChangeNotice`（仓/ref/可选 Address/可选 sourceRevision hint，拒绝正文）；`Controller.Notify` / `CatchUp` 与 Snapshot Desire 分钥，冷启动全量 `RefreshState`，notice 走 `RefreshStateObjects`。公开入口是 `kc operations projection notice` 与 `POST /operations/v1/projections:notice`，消费 SEARCH 仍不得 `RefreshState`。尚需按[索引控制](index-control.md)与 `DYN-01` 验收真实 Observer/runtime/lakeFS/KC/OpenSearch 及重启恢复；范围覆盖、时效与观察保留沿 `DOC-20` 收口。显式同步仍用于 Snapshot EnsureAt、历史固定版本、强制重建和排障，不是动态恢复的唯一救济方式。
- **Stream projection / RetrievalPlan**：Aspect 可声明 Stream Binding。普通 READ 对 Stream 已失败关闭（`TestOrdinaryReadRejectsStreamBinding`）。缺的是 window/query 面与投影；Binding 里的 `protocol: mcp` 只是 ResourceDescriptor 字段，不是 MCP Gateway。
- **多实例 / MCP Gateway / 多语言 SDK**：[服务边界](service.md)的规模化拆分与 MCP 网关是方向；未落地记在这里，不是服务篇否决。公开 `append`/`stream` 命令与退役 HTTP 路由已保持 404（`TestAppendAndStreamSurfacesStayAbsent`）。
- **Tree 提供方规模资格**：普通单对象 PUT 已按对象 locator 读取，`TestSingleObjectPutCostIsIndependentOfRepositorySize` 断言 10/1000 对象下无全树列举、调用与解码成本不随仓总量增长。显式 locator 重建、系统初始化和涉及 Schema 依赖的维护路径有自己的成本与能力合同；整体吞吐、历史增长和恢复仍须按 `SCALE-01` 实测，不能继续把普通 COMMIT 描述成全仓扫描，也不能从点读成本推出容量资格。
- **Schema breaking 迁移的合同与实现**：实现目前覆盖同一 Schema ID 的兼容演进；breaking 变更的身份/迁移边界在设计合同间仍有待评审分歧（本轮 REVIEW-03），不能宣称“原位 breaking”已经确定为唯一目标。先统一 owner 决策，再补迁移证据与验证，不用现有实现反向决定设计。
- `kc serve` 已按正式 namespace 形成模块化单体；进一步拆成独立进程是部署选择，不是新协议层。
- command-id 能覆盖当前进程/共享日志的知识写面重试；PENDING 可经显式 resolve/abandon
  核对，Bolt 保留清理不在启动时加载历史，ledger 读失败不会重放。多实例协调、分布式租约、
  灾难恢复，以及 attach/grant 等管理写入的统一重放，尚未形成生产验收。
- 已有 Go typed client。Dataset 登记服务/命名知识集发现走 Snapshot 入口，单仓 Schema 发现走固定版本的目录；维护读回与消费读共用对象 URN＋locator，消费读可另限定 Dataset。精确历史读取保留对象引用与回执中的 commit。
- Linux 宿主 VFS 是可选文件体验，不是接入或消费协议成立的前提。VFS 不是 Writer，也不是 Dataset 登记服务 成员条件；只读是选定的宿主投影合同。Plain 仓只解释组合阶梯。
- lakeFS authority 的真实服务能力需要对应部署入口的证据；默认 `make test` 使用 lakeFS HTTP 假服务与真实 OpenSearch，不能替代真实 authority 验收、其它合同或生产容量、备份、升级和故障演练。

## Linux VFS 子验收

VFS 的目标是把 Dataset 的多个 Repository 子树投影到已有项目目录。它不增加知识语义，也不把编辑器写文件升级为新的 Write Surface。

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
| V7 | 授权 | 先检查 `dataset.resolve`，再检查已发布 Dataset 范围的当前 `file.read`；无权成员不进入 plan/mount |
| V8 | 消费语义视图 | `--view semantic` 不要求 Repository mount path；固定 pin 生成 `object/<source>/<entity-plural>/*.yaml`，保留 `_kc` 坐标且不暴露 Canonical 单元信封 |
| V9 | 原宿主文件视图已退役 | 退出当前宿主执行分母；原集成缺省保存固定版本上下文，显式开启文件视图才挂载，已有挂载可显示/隐藏或移除。文件开关不发权、不改变固定版本的约束保留，当前无宿主开关验收证据 |
| V10 | 无 Agent 专用 VFS | Agent 使用标准 filesystem/search 工具与公开 CLI，不引入第二套模型专用文件访问面 |
| V11 | 原宿主发现后添加已退役 | 退出当前宿主执行分母；原集成无默认 Dataset 也可发现来源、添加自选来源或命名集并固定版本，文件挂载可选。核心 CLI/HTTP 发现仍在；检查更新与采用更新分开、失败保留旧上下文的宿主要求保留，当前无替代宿主验收 |
| V12 | 安全路径 | 拒绝绝对路径、`..`、反斜杠、NUL、根挂载、重叠 mount 和 symlink 穿越 |
| V13 | 宿主失败可解释 | 缺 `/dev/fuse`、`fusermount3`、TreeStore capability 或非空 mountpoint 时明确失败 |
| V14 | 原宿主侧栏与 Skill 已退役 | 退出当前宿主执行分母；原侧栏文件树默认隐藏、不预扫，未接入时可多源选择或采用命名集，并明确库存截断。来源发现、有界库存、多源采用与失败恢复目标保留；自然语言引导 SEARCH→Canonical READ 仍属 C8 未执行缺口，不要求用户先懂命令 |

V9/V11/V14 保留原宿主要求与退役边界，不表示新的宿主已通过。核心固定版本与当前权限由
`TestProductRepoCommitFreezesKnowledgeAndCurrentPermissions`、文件网关固定 pin 由
`TestPrepareRemoteKnowledgeSetFSUsesGatewayAndKeepsFixedPin`、来源发现由
`TestRemoteProviderReadBackAndConsumerDiscovery` 拥有既有断言；这些不证明侧栏、文件开关、
更新检查/采用或真实模型交互。V1–V8、V12/V13 的核心文件交付与 Linux/FUSE 风险继续验收。

可选文件挂载的环境要求：Linux 可访问 `/dev/fuse`，安装 `fusermount3`；容器显式暴露设备和挂载 capability；每个 mountpoint 不存在或为空。mountpoint 是目录，不支持单文件 mount，也不允许挂到项目根。

```bash
go test ./datasetfs ./snapshot/dataset ./cli ./internal/arch -count=1
./scripts/e2e-kcfs-linux.sh
./scripts/e2e-kcfs-docker.sh
```

若未来提供可写文件体验，应使用显式 checkout/overlay + reconcile/commit，保留 `expectedTargetCommit`、`command_id`、逐 Repository 结果和冲突恢复。不能把普通 FUSE write 直接解释成协议 COMMIT，也不能伪装跨 Repository 原子事务。
