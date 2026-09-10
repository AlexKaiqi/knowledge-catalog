# home/

部署装配入口：按声明配置连接 Catalog Git 权威和已有 Snapshot，把 Writer / Reader / ControlPlane / Index 装配到 Server。具体 Dolt/Gitea adapter 只允许在 `authority_drivers.go` 被 import（A-01）。本包不得 import `cli`、`client` 或 `httpsurface`。

业务 CLI 经 Server 和显式 principal。`kc deployment init --config` 显式初始化；`kc serve --config` 与 `OpenDeployment` 仅恢复。没有公开 `local` 命令或由实例目录扫描得到的部署清单。

## 声明配置

`ReadDeployment` 接受单个 YAML 或 JSON 文档，拒绝未知字段。`DeploymentConfig` 是配置合同；部署工具负责版本化和分发配置，凭证通过环境或 Git credential helper 注入。

```yaml
version: 1
auth: local
listen: 127.0.0.1:7380
bootstrapPrincipal: agent:operator
stateDir: /srv/kc-state
cacheDir: /var/cache/kc
catalogs:
  - id: kr://acme/catalog
    remote: ssh://git@example.org/acme/catalog.git
    ref: refs/heads/main
repositories:
  - id: kr://acme/knowledge
    driver: gitea
    dsn: https://git.example.org/acme/knowledge
stores:
  index: none
```

`stateDir` 必须挂载独立持久卷；`cacheDir` 可随实例删除。两者是绝对且不互相嵌套的目录。Catalog 的 Git container 必须预先存在；初始化只允许创建其中的 Catalog branch。文件路径形式的 Git 权威也必须独立于状态目录与缓存，不能用同一实例缓存里的 bare repository 冒充可恢复部署。

`stores` 在部署配置中仅接受 `index` 与 `opensearch`；夹具的 `profile`、默认 `repository` driver 与 `layout` 不得混入部署配置。每个来源声明自己的 driver，实例路径由 `stateDir` / `cacheDir` 唯一决定。

`managedRepositories` 是可选的平台仓供给策略，与静态 `repositories` 并列。客户端调用 `catalog repo create --catalog <id> --repo <id> --command-id <id>`，不传 driver、地址、凭证或 grants。

```yaml
managedRepositories:
  driver: dolt
  root: /srv/kc-managed-authority
  publicURL: https://kc.example
  creatorActions: [writer.preview, writer.commit, writer.receipt.read, repository.metadata.read, knowledge.read, knowledge.provenance]
```

Dolt 使用独立耐久 root；Gitea 使用部署声明的 URL `dsn` 与服务凭证，客户端不接触凭证。创建动作不要求目标仓预先出现在 repositories。`creatorActions` 必须显式选择 `validateCreatorActions` 允许的窄动作，包含可选的仓元数据、自己的回执、分享、评审和仓维护能力；没有默认发权，不允许全局管理员、Catalog 管理或投影写权限。创建者获得的普通 grant 可以撤销。重复创建请求返回原结果，重启与重试不得补回已撤销权限。新仓 binding、创建身份与分阶段进度保存在 `stateDir/managed.db`；RESERVED → OWNED → REGISTERED → READY 逐步记录分配、拥有来源、Catalog 准入与创建完成。已完成结果保留初始 HEAD，重复请求不把它替换成当前 HEAD；Catalog 仍只保存 Git 成员与配方。

用户自助选择由 `managedStores` 声明的命名池；它与旧 `managedRepositories` 单池配置互斥。只有一个池时自动选择，多个时要求用户的 `--store`。`--name` 接受可读名称（包括中文），Server 从当前用户名和请求坐标生成稳定仓身份和恢复命令。同一人重复提交相同名称与 Store 恢复原结果。新的规范人类用户名请求供给同名 Store 账号或隔离空间；旧持久分配和机器主体保持原有恢复身份。

```yaml
managedStores:
  gitea:
    driver: gitea
    dsn: https://gitea.example/platform
    publicURL: https://kc.example
    creatorActions: [writer.preview, writer.commit, writer.receipt.read, knowledge.read, knowledge.schema.read, repository.metadata.read, repository.shares.manage, workspace.resolve, workspace.consume]
    shareActions: [knowledge.read, workspace.resolve, workspace.consume]
  dolt:
    driver: dolt
    root: /srv/kc-authority
    publicURL: https://kc.example
    creatorActions: [writer.preview, writer.commit, writer.receipt.read, knowledge.read, repository.metadata.read]
```

Gitea 从配置 URL 取已批准服务地址，物理仓在用户同名账号下建立；账号占用必须通过本分配标记或同一可信 Gitea issuer/数字 subject 验证，不按名字收养。共享 SSO 可用 `authSourceId` 指向部署预先配置的认证源；没有共享 SSO 或可信既有账号时，原生网页登录未准备。配置 `publicURL` 可由 KC 管理页提供统一入口，真实 Gitea 页保留在 `providerURL`，`managementState` 明确区分管理入口准备状态。Dolt 必须配置 `publicURL`，在独立 root 下按用户名分配租户目录，并返回真实 KC 管理页；没有假定存在 DoltLab。

托管 Gitea 恢复时仅装配耐久绑定的延迟句柄，已保存的管理地址不依赖远端在线；每次实际读写重新验证原 backend、allocation 和初始 commit。Dolt 保持原 native 句柄与能力；静态源和 Catalog 的恢复仍遵守既有合同。创建进度 READY 不表示远端当前健康，管理详情会另外查询当前发布和检索状态。

`ManagedRepositoryResult` 的名称、owner、Store、管理地址和分配进度来自耐久账。`ListManagedRepositories`/`GetOwnedManagedRepository` 只选择本人控制记录；HTTP 应用再检查当前 `repository.metadata.read`，不会为导航授予 Catalog 全局发现或知识正文读权。

`repositories` 只是运营者批准的连接绑定，不代表 Catalog 成员关系。`catalog repo attach --repo <id>` 使用静态绑定或已供给托管仓的耐久绑定，只读检查已有 Snapshot，再通过一次 Catalog Git 提交完成接入；托管仓可据此加入另一 Catalog。没有单独手工 register，也没有持久化的第二份 attached 状态。Dolt 完全没有 Knowledge 原生表时可作为纯 Snapshot 接入；已有但不兼容的原生表必须报错，不能在接入或恢复时迁移。

System 的二进制信任根默认以不可变内置发布提供。若要在外部 Snapshot 发布，将 `kr://kc/system` 加到 `repositories` 后显式执行 `kc deployment system publish --config`；该操作使用 System Writer，来源绑定仍只存在于配置中。

## 权威与恢复

| 内容 | 所有者与存储 | 实例替换 |
|---|---|---|
| Catalog 身份、成员、Workspace 配方、归档与历史 | 配置指定的独立 Git remote/ref | 拉取恢复；写成功以远端接受 CAS 为准 |
| Snapshot 内容、版本与候选分支 | 静态连接或托管仓记录指定的 authority | 只读打开；不得初始化、stamp 或迁移 |
| 托管仓连接、创建请求与分阶段进度 | `stateDir/managed.db`；Snapshot 仍在托管 authority | 恢复新仓连接及原创建结果，不改静态配置、不补发已撤销 grant |
| grants、hooks、gates | `stateDir` 内应用数据 | 持久卷保留，不从 bootstrap 配置重建 |
| 可信用户名绑定 | `stateDir/identities.json`；初始化回执记录其存在 | 绑定用户名与 provider/issuer/subject；缺失失败，不重新接管同名账号 |
| Writer command-id 回执、提案、Preview、验证记录 | `stateDir` 内账本和 control state | 保持幂等重放与维护流程连续性 |
| 过程证据、访问证据、待投递 outbox | `stateDir` | 保留；队列清空后允许删除空 outbox 文件 |
| Catalog 工作副本、投影与 checkout | `cacheDir`；检索投影也可由配置的外部 provider 保存 | 可丢弃并按明确版本重建 |
| endpoint、driver、路径、认证模式、首个管理员 | 声明配置 | 由部署系统恢复；bootstrap 只在显式首次初始化使用 |
| 部署 token、password | 外部凭证提供方 | 重新注入，不能写入配置或 Catalog |
| 用户授权的逐仓连接 credential 与固定绑定 | 模式 0600 的 `stateDir/connections.db`，秘密独立 bucket | 依赖原持久卷和初始化回执，恢复管理面后可显式轮换 |

`InitializeDeployment` 将初始控制数据写入 staging 后一次 rename 安装，已初始化的数据不能被第二次 init 清空。 若 Git 中已有 Catalog 而持久卷或标记丢失，init 也必须拒绝重新发放 bootstrap 权限，要求恢复原卷。`deployment-state.json` 记录已初始化 Catalog 身份的回执，区分新增配置与已丢失的 Git 分支；它不保存成员清单或来源连接配置。已初始化 Catalog 的分支丢失时 init 同样拒绝重建空 Catalog。首次初始化跨 Git 和持久卷时不提供分布式事务：若在 Git 发布后、初始状态安装前中断，会保守拒绝再次初始化，需要核对并处理未完成的部署，不能把它当成空治理状态。`ValidateDeploymentState` 拒绝缺失的持久化策略、账本或控制文件；Server 在请求前再次检查，不能把卷故障解释为“没有 gate”。Catalog 接入的原子性只涵盖一次 Catalog 提交，不宣称来源与 Catalog 的跨仓事务或持续在线保证。

## 装配与夹具

旧部署首次启用供给能力时，显式 `deployment init` 为其建立空的 managed 账和初始化回执；它不重建授权或业务仓。初始化回执表明账本已经存在后，缺失或损坏的账本只能恢复，不能重新初始化为空。关闭新仓供给不会删除历史绑定，启动仍验证并恢复既有托管来源。

| 文件 | 职责 |
|---|---|
| `deployment.go` / `deployment_runtime.go` | 声明解析、初始化、恢复、只读接入与持久状态检查 |
| `home.go` | Store、Catalog、Writer、Reader、ControlPlane、Index 的共同装配 |
| `managed.go` / `managed_ledger.go` | 显式托管仓创建、耐久分配账和只读恢复；重试不重新发权 |
| `authority_drivers.go` | 唯一具体 Snapshot adapter 选择点；创建与只读打开分离 |
| `home_sidecar.go` | AfterSnapshot → Index、Merge Gate |
| `home_system.go` | System Writer 发布与内置信任根校验 |
| `journal.go` | Writer / Reader / Catalog / ControlPlane 的过程账 |
| `read_view.go` | 正式部署请求的独立 Reader、过程账上下文和 Catalog 已接受状态视图 |
| `home_discover.go` / `home_mount.go` | 组件夹具的显式创建与目录发现；不用于生产恢复 |
| `stores*.go` | 运行时 provider 配置及旧组件夹具布局 |

`Open`、`InitHome`、`AddRepository` 留给显式组件装配和测试夹具；不能用它们实现生产 `attach` 或 Server 恢复。配置绑定新增 Catalog 后显式 `deployment init`，启动过程本身不登记 System 或业务来源，不隐式收养工作区配方。

正式部署的并行读取通过 `ReadView` 隔离每个请求的 Reader、过程账身份和 Catalog 已接受状态，不修改进程共享的 journal 或 Writer stamp。视图借用 Store、Writer、命令账本与 Index，保留回执查询能力，调用方不得关闭共享资源。Catalog 视图不重新加载 Git 权威、不订阅 Snapshot 事件，也不能持久化变更。只有保证读取不会隐式收养配方的正式部署路径使用该视图；组件夹具的显式装配语义保持独立。

## 首次准入与受限分享

`AdmissionConfig` 只声明可选的外部申请入口 `requestURL`。KC 的 admission 查询汇总本人
当前 grants 与 grant 管理者，不承载申请、审批或自动发权策略。

登录不应用准入。当前可信人类用户请求 `admission request` 后，应用在 `stateDir/allow.json` 中一次原子保存准入回执和规则。Agent/service 不能以 authenticatedUsers 策略申请；显式 local 测试身份除外。后续重试、重启和策略增补不重新应用该用户已完成的准入；已撤销规则保持撤销。`AdmissionResult` 同时报告原始决定与当前仍存在的动作。

仓的 `ShareActions` 是供给时冻结的消费动作白名单，与 `CreatorActions` 分开。Server 还检查发起人当前拥有的整仓能力，只有两者交集可以分享。分享管理权自身不在可分享动作中。带 `ShareID`/`SharedBy` 的仓级规则与管理员规则共存；按仓和 share ID 撤销，不删除其他仓或无分享来源的规则。

历史托管请求的 principal 不重写。显式用户迁移在身份账登记核验过的旧主体归属别名，`ListManagedRepositories`/`GetOwnedManagedRepository` 将 owner 解析成规范用户名。原 command 或原 name 的重试核对别名后仍使用原请求摘要，保留仓身份、管理地址、allocation 与初始发权回执；其他字段变化及同名歧义拒绝。归属解析不发权，实际业务请求继续验证当前 grant。

## 自有 Snapshot 连接

`connections` 显式开放已有 Gitea 仓的自助接入。`allowedOrigins` 是部署批准的 provider origin（可含 Gitea 部署前缀），不能带凭证、查询或 fragment。用户不能据此探测任意网络地址，也不能提交 Server 上的 Dolt 目录。远程 Dolt adapter 尚未选定，当前该入口只接受 Gitea。

```yaml
connections:
  allowedOrigins: [https://git.example.org]
  creatorActions: [repository.connections.manage, repository.metadata.read, writer.preview, writer.commit, knowledge.read]
  shareActions: [knowledge.read, workspace.resolve, workspace.consume]
```

`catalog.repositories.connect` 授予指定 Catalog 的连接准入。`ConnectRepository` 只读验证已存在的非空 Snapshot，不建仓、不初始化分支、不写 stamp。固定 provider repository ID 与初始 commit 和逻辑 identity 一起进入 Server 私有 `connections.db`；Catalog 只登记 identity。`creatorActions` 必须显式包含 `repository.connections.manage`，其余动作沿用窄创建动作白名单。初始 grant 使用稳定 allocation 回执只应用一次，重放连接不能补回撤销的权限。`shareActions` 在连接时冻结；分享仍要求发起人当前拥有相应整仓能力。

`GetConnection` 返回 owner、管理 URL、初始 HEAD、状态与 revision，不返回凭证；`CheckConnection` 只读验证并报告当前 HEAD；`RotateConnectionCredential` 只接受新凭证，先验证同 provider identity 与初始 commit，成功才原子替换秘密。三个管理动作都要求当前仓级管理权以及 durable owner 相符；知道一个仓的 identity 不意味着可以替换它的凭证。旧绑定和可用凭证在失败时保留。

用户显式授权的逐仓 credential 是部署凭证环境注入规则的窄例外：仅在模式 `0600` 的私有 `connections.db` credential bucket 保存，和公开 binding 分开，只有具体 adapter 打开时读取。它不进入配置、Catalog、知识对象、日志、trace 或客户端 pin。备份必须包括该私有账本及初始化回执；回执存在后账本缺失或损坏会失败，不重新创建空账。

恢复只装配固定 Gitea authority 的延迟句柄，不要求外部服务或旧 credential 先可用。普通 Snapshot 操作按当前 credential 和固定身份验证，失败关闭；管理面仍可查询和轮换。该句柄精确保留 Gitea 的 Store、TreeStore、DirectoryReader、HistoryStore 能力，不模拟额外的知识能力。启动不会因此写入外部仓，也不自动更换 authority。

## 同版本正文缓存与预热配置

Home 默认装配同一份 `retrieval/cache` 到 Reader、请求级 ReadView、Workspace Serving 与
Index 回读接缝。缓存持有 Snapshot 声明正文，不持有动态 State observation 或裁剪后的权限结果。
默认预算为 64 MiB 计量字节、4096 条目；该计量不是进程 RSS 上限。关闭 Home 先停止后台消费者，
再清空缓存。一次性 `Open` 不启动维护，长期 Server 才启动 Controller。

部署文件的 `stores.hydrationCache`（本机夹具的 `stores.yaml` 使用同名字段）可以调整：

```yaml
stores:
  hydrationCache:
    maxBytes: 67108864
    maxEntries: 4096
    warmLimit: 128
    warmBatchSize: 32
    coldStart: true
```

不填写时启用缓存和热点预热；`coldStart` 默认 false，设为 true 才在无热点时取有限首个维护身份页。
`disabled: true` 禁用缓存及其预热，不禁用已配置的搜索投影。数值 0 使用默认值，负值拒绝。
`snapshot-body-cache/v1` 是注册的独立后台消费者，按当前 published HEAD 刷新有限热点；
没有 OpenSearch 也可以工作。Cache `Stats()` 和 Controller `ConsumerTargets()` 提供装配侧观测，
不新增公开知识查询字段或运维路由。缓存 miss 始终可同版本回源；旧 pin 不跟随预热到新版本。

## Catalog 发现入口

`CatalogBinding.discoveryWorkspaceId` 可指向本 Catalog 中一条普通、管理员维护的 WorkspaceDefinition。配置只选择发现入口，不创建或更改配方，也不自动收集全部已登记来源。`catalog show` 返回该 identity；入口未配置时，Catalog 范围搜索明确返回能力缺失。

`knowledge search --catalog <id>` 由 Client 执行 show → 普通 ResolveWorkspace → 固定 pin SEARCH。已有 typed Resolve 与 Search 请求的 `catalogDiscovery: true` 只表达发现语境，Server 必须校验它匹配本部署配置的精确 Catalog/Workspace，禁止混入临时 definition 或单仓坐标，SEARCH 必须携带固定 pin。通过该验证后的准入只看当前 `catalog.read`；没有 `workspace.consume` 或逐仓 `knowledge.search` 不裁掉 discovery 成员。正文仍通过现有交付链按当前逐仓 `knowledge.read` 屏蔽；其他 Workspace、READ、Schema、文件、rerank 等动作不使用这条例外。
