# home/

部署装配入口：按声明配置连接独立登记表 Snapshot 权威和已有对象 Snapshot，把 Writer / Reader / Index 装配到 Server。Snapshot 后端只经 `kc/snapshot` 公开构造与能力契约接入；lakeFS 实现归私有 `snapshot/internal/lakefsimpl`（A-01）；`declarativeindex/opensearch` 只允许在 stores 族与就绪端口绑定（`readiness_ports.go`：`NewSearchBackendProbe` 把具体索引探测绑定成端口函数，CAP-ORG-01 B7）。本包不得 import `cli`、`client` 或 `server`。

业务 CLI 经 Server 和显式 principal。`kc deployment init --config` 显式初始化；`kc serve --config` 与 `OpenDeployment` 仅恢复。没有公开 `local` 命令或由实例目录扫描得到的部署清单。

Object Reader 的固定读取依据与历史扫描记录由根 Home 在安装状态目录装配，借用请求视图
共享同一存储端口，关闭请求不删除记录。两类记录只保存元数据，独立于索引代的生命周期；
存储版本、引用和可用性合同见 [Reader](../object/reader/README.md)。

## 声明配置

生产部署配置的形状、单文档解析与静态校验归 `home/config`：读取一份部署文档不打开 authority、
不构造索引引擎、不写部署状态、不启动维护。`home/` 根包保留装配职责——打开来源、恢复资源、
构造对象图；配置子包不反向依赖根 Home、`server` 或 `invocation`。

`ReadDeployment` 接受单个 YAML 或 JSON 文档，拒绝未知字段。`DeploymentConfig` 是配置合同；部署工具负责版本化和分发配置，服务凭证通过环境注入；用户显式连接凭证遵守下文的私有耐久账合同。

`liveaccess.credentialOrigins` 是调用方证明的可信适用 HTTP origin 列表，不启用插件；
`liveaccess.lookupTimeout`、`liveaccess.readTimeout` 是单元与单次读取的外部取值时限（正 Go duration）。
顶层 `plugins` 按插件名安装实例（见下文“部署插件与运行装配”）；Repo 与 Dataset 不选择实例。
根 Home 固定装配 Index 与 LiveAccess 两类贡献，借用视图共享同一运行视图。LiveAccess 的
构造、取值和释放归模块入口，配置及结果合同见 [LiveAccess](../liveaccess/README.md)。

`auth` 接受 identity 注册表中的认证方式：`local` 或 `taihu`。组合多用户控制台沿用现有
`taihu` 认证与 `authURL`，Repo、Dataset 均由 Snapshot 当前规则授权；
`KC_LAKEFS_CREDENTIAL` 只作为服务私有存储凭据。原 native origin 从显式 Registry lakeFS
DSN 确定，不从认证来源或首个仓推断。若使用已有浏览器认证网关会话，`authLogoutURL`
显式指定该网关的退出 URL（HTTP(S) 或同源绝对路径）；KC 不猜测或创建会话退出服务。
lakeFS 只作存储，不作身份来源；原 `auth: lakefs` 已退役。
浏览器请求检查与用户名绑定见
[`server/README.md`](../server/README.md)。登录不自动授予 登记表/Dataset 动作。

```yaml
version: 1
auth: local
listen: 127.0.0.1:7380
bootstrapPrincipal: agent:operator
stateDir: /srv/kc-state
cacheDir: /var/cache/kc
registry:
  id: kr://acme/registry
  driver: lakefs
  dsn: https://lakefs.example/kc-catalog
  ref: refs/heads/main
repositories:
  - id: kr://kc/system
    driver: lakefs
    dsn: https://lakefs.example/kc-system
  - id: kr://acme/object
    driver: lakefs
    dsn: https://lakefs.example/acme-object
stores:
  index: none
```

`stateDir` 必须挂载独立持久卷；`cacheDir` 可随实例删除。两者是绝对且不互相嵌套的目录。登记表 使用独立 Snapshot repository（与仓同类介质、不同 identity）；初始化只写入登记表 YAML，不把 登记表 变成 Object Repository。`file://` 或实例盘上的 Git remote 不能充当生产 登记表 权威。

`stores` 在部署配置中仅接受 `index` 与 `opensearch`；夹具的 `profile`、默认 `repository` driver 与 `layout` 不得混入部署配置。每个来源声明自己的 driver，实例路径由 `stateDir` / `cacheDir` 唯一决定。

`managedRepositories` 是可选的平台仓供给策略，与静态 `repositories` 并列。客户端调用 `create --name <name>`，不传 driver、地址、凭证或 grants。

```yaml
managedRepositories:
  driver: lakefs
  dsn: https://lakefs.example
  root: s3://kc-authority
  publicURL: https://kc.example
  creatorActions: [writer.preview, writer.commit, writer.receipt.read, repository.metadata.read, object.read, object.provenance]
```

LakeFS 使用部署声明的 URL `dsn`、S3 兼容对象存储 `root` 前缀与服务凭证，客户端不接触服务凭证。Snapshot 存储只支持 lakeFS，其它 driver 以 `USAGE_INVALID` 拒绝。创建动作不要求目标仓预先出现在 repositories。`creatorActions` 必须显式选择 `validateCreatorActions` 允许的窄动作，包含可选的仓元数据、自己的回执、分享、评审和仓维护能力；没有默认发权，不允许全局管理员、登记表 管理或投影写权限。创建者获得的普通 grant 可以撤销。重复创建请求返回原结果，重启与重试不得补回已撤销权限。新仓 binding、创建身份与分阶段进度保存在 `stateDir/managed.db`；RESERVED → OWNED → REGISTERED → READY 逐步记录分配、拥有来源、登记表 准入与创建完成。已完成结果保留初始 HEAD，重复请求不把它替换成当前 HEAD；登记表 仍只保存来源登记与配方。

用户自助选择由 `managedStores` 声明的命名池；它与旧 `managedRepositories` 单池配置互斥。只有一个池时自动选择，多个时要求用户的 `--store`。`--name` 接受可读名称。LakeFS 上 `--name`（合法 Graveler slug）就是协议仓 ID，与 lakeFS 仓库名相同，不另造 `repo-<hash>`。同一人重复提交相同名称与 Store 恢复原结果。恢复命令仍由账本生成，不改仓名。既有持久分配和机器主体保持原有恢复身份。

```yaml
managedStores:
  lakefs:
    driver: lakefs
    dsn: https://lakefs.example
    root: s3://kc-authority/tianqiong
    publicURL: https://kc.example
    creatorActions: [writer.preview, writer.commit, writer.receipt.read, object.read, object.schema.read, repository.metadata.read]
```

LakeFS 业务仓的 Graveler 名、协议 `--repo` 和管理入口最后一段都是 `--name`（必须能写成 lakeFS 允许的 `[a-z0-9-]{3,63}`），不含 owner；对象前缀是 `{root}/{name}`（例如 `s3://kc-authority/tianqiong/table-meta`）。`kc-` 只留给平台仓 `kc-catalog` / `kc-system`。allocation 只做账本令牌，不出现在仓名或对象前缀里。创建进度 READY 不表示远端当前健康，管理详情会另外查询当前发布和检索状态。

`ManagedRepositoryResult` 的名称、owner、Store、管理地址和分配进度来自耐久账。`ListManagedRepositories`/`GetOwnedManagedRepository` 只选择本人控制记录；HTTP 应用再检查当前 `repository.metadata.read`，不会为导航授予 登记表 全局发现或对象正文读权。

一套服务只有一份登记状态。部署认证后的主体可以发现库存；成员正文仍需逐仓或 Dataset 消费授权。配置不接受 `catalogs`、`private` 或请求 登记表 选择。

`repositoryAccess` 按仓声明已认证默认可读动作（consume 侧闭集）。未列出的仓仍要 grant。这不是仓类型，也不放行写。System Repository 选用同一声明；授权器不按保留 ID 短路。

```yaml
repositoryAccess:
  - id: kr://kc/system
    authenticatedActions: [object.read, object.schema.read, object.history.read, object.provenance, object.relations, object.access.describe, file.read, projection.read]
```

`datasetAccess` 按 Dataset 声明已认证默认消费动作，闭集为 `dataset.resolve`、`file.read`。`deployment init`/`OpenDeployment` 把声明写入 `stateDir/dataset-access.json`；授权时为当前已认证 principal 派生只作用于该 Dataset 的规则，不写回 `allow.json`、不出现在 `grant list`，也不放行来源仓的 `--repo` 读取、`dataset define` 或写。未列出的 Dataset 仍要 grant。

```yaml
datasetAccess:
  - id: qinghe-analysis
    authenticatedActions: [dataset.resolve, file.read]
```

`repositories` 只是运营者批准的连接绑定，不代表 登记表 成员关系。`attach --repo <id>` 使用静态绑定或已供给托管仓的耐久绑定，只读检查已有 Snapshot，再通过一次 登记表 Snapshot 提交完成接入；重复准入不创建新仓或重新发权。没有单独手工 register，也没有持久化的第二份 attached 状态。已有但不兼容的 authority 状态必须报错，不能在接入或恢复时迁移。

`repositories` 必须显式绑定 `kr://kc/system` 的实际 LakeFS Snapshot。`kc deployment init --config` 将随版本交付的 `systemrepo/content` 出版源经 System Writer 发布到空仓，然后登记到 登记表；相同出版物只核对，不产生新提交。已初始化部署重新 init 只核对既有 system 内容，缺失或漂移须恢复权威。`kc deployment system publish --config` 保留为显式发布/核对入口，同样不覆盖已有不同版本。

`OpenDeployment` / `kc serve` 只打开并核对实际 system 仓。运行时对象与文件读取全部来自该 Snapshot，引用修改、归档和字面文件写入均拒绝；不存在内置仓或来源失败时的回退。二进制携带出版源和校验信任根，不充当运行时仓。

## 权威与恢复

| 内容 | 所有者与存储 | 实例替换 |
|---|---|---|
| 登记权威、成员、Dataset 发布配方与历史 | 配置指定的独立登记表 Snapshot 权威 | 重新打开该 Snapshot；写成功以 ref CAS 为准 |
| Snapshot 内容、版本与候选分支 | 静态连接或托管仓记录指定的 authority | 只读打开；不得初始化、stamp 或迁移 |
| 托管仓连接、创建请求与分阶段进度 | `stateDir/managed.db`；Snapshot 仍在托管 authority | 恢复新仓连接及原创建结果，不改静态配置、不补发已撤销 grant |
| grants、hooks | `stateDir` 内应用数据 | 持久卷保留，不从 bootstrap 配置重建 |
| 可信用户名绑定 | `stateDir/identities.json`；初始化回执记录其存在 | 绑定用户名与 provider/issuer/subject；缺失失败，不重新接管同名账号 |
| Writer command-id 回执（含 MERGE） | `stateDir` 内账本 | 保持幂等重放与维护流程连续性 |
| 过程证据、访问证据、待投递 outbox | `stateDir` | 保留；队列清空后允许删除空 outbox 文件 |
| 派生投影与正文缓存 | `cacheDir`；检索投影也可由配置的外部 provider 保存 | 可丢弃并按明确版本重建；登记表 发布权威始终是 Snapshot |
| endpoint、driver、路径、认证模式、首个管理员 | 声明配置 | 由部署系统恢复；bootstrap 只在显式首次初始化使用 |
| 部署 token、password | 外部凭证提供方 | 重新注入，不能写入配置或 登记表 |
| 用户授权的逐仓连接 credential 与固定绑定 | 模式 0600 的 `stateDir/connections.db`，秘密独立 bucket | 依赖原持久卷和初始化回执，恢复管理面后可显式轮换 |

`InitializeDeployment` 将初始控制数据写入 staging 后一次 rename 安装，已初始化的数据不能被第二次 init 清空。 若登记表 Snapshot 权威已有登记而持久卷或标记丢失，init 也必须拒绝重新发放 bootstrap 权限，要求恢复原卷。`deployment-state.json` 记录唯一 registryAuthority 回执，拒绝切换权威或重建已丢失的权威 ref；它不保存成员清单或来源连接配置。已初始化登记表的权威丢失时 init 同样拒绝重建空登记表。首次初始化跨登记表 Snapshot 和持久卷时不提供分布式事务：若在权威发布后、初始状态安装前中断，会保守拒绝再次初始化，需要核对并处理未完成的部署，不能把它当成空治理状态。`ValidateDeploymentState` 拒绝缺失的持久化策略、账本或控制文件；Server 在请求前再次检查，不能把卷故障解释为“没有 gate”。来源接入的原子性只涵盖一次登记表提交，不宣称来源与 登记表 的跨仓事务或持续在线保证。

## 装配与夹具

旧部署首次启用供给能力时，显式 `deployment init` 为其建立空的 managed 账和初始化回执；它不重建授权或业务仓。初始化回执表明账本已经存在后，缺失或损坏的账本只能恢复，不能重新初始化为空。关闭新仓供给不会删除历史绑定，启动仍验证并恢复既有托管来源。

| 文件 | 职责 |
|---|---|
| `config/` | 部署配置形状、单文档解析与静态校验；读取配置不打开来源或引擎 |
| `deployment.go` / `deployment_runtime.go` | 配置类型的根包别名、初始化、恢复、只读接入与持久状态检查 |
| `registry_authority.go` | 独立登记表 Snapshot 容器的打开/创建；不进入对象 Store |
| `home.go` | Store、登记表、Writer、Reader、Index 的共同装配 |
| `managed.go` / `managed_ledger.go` | 显式托管仓创建、耐久分配账和只读恢复；重试不重新发权 |
| `authority_drivers.go` | 唯一具体 Snapshot adapter 选择点；创建与只读打开分离 |
| `home_sidecar.go` | AfterSnapshot → Index |
| `home_system.go` | System Writer 发布与内置信任根校验 |
| `journal.go` | Writer / Reader / 登记表 的过程账 |
| `read_view.go` | 正式部署请求的独立 Reader、过程账上下文和 登记表 已接受状态视图 |
| `home_discover.go` / `home_mount.go` | 组件夹具的显式创建与目录发现；不用于生产恢复 |
| `stores*.go` | 运行时 provider 配置及旧组件夹具布局 |

`Open`、`InitHome`、`AddRepository` 留给显式组件装配和测试夹具；不能用它们实现生产 `attach` 或 Server 恢复。初始化必须显式调用 `deployment init`，启动过程本身不登记 System 或业务来源，不隐式收养工作区配方。

正式部署的并行读取通过 `ReadView` 隔离每个请求的 Reader、过程账身份和 登记表 已接受状态，不修改进程共享的 journal 或 Writer stamp。视图借用 Store、Writer、命令账本与 Index，保留回执查询能力，调用方不得关闭共享资源。登记表 视图不重新加载 Snapshot 权威、不订阅 Snapshot 事件，也不能持久化变更。只有保证读取不会隐式收养配方的正式部署路径使用该视图；组件夹具的显式装配语义保持独立。

其它共享 Home 上的请求走 `RequestView`：Store、命令账本、登记表已接受状态、库存与插件共享，已接受的写对其他请求立即可见；journal、Writer、TreeWriter、Reader 与 Dataset 服务的身份印记归本请求所有。并发写因此不需要进程级锁：版本化写由预期提交核对裁决，命令账本按命令 ID 串行，登记文档由 Dataset 服务串行读改写，本机策略文件按路径串行读改写。借用视图的 `Close`、`StopMaintenance` 是空操作，也不能装配插件；进程独占的 Home（本地 CLI 单次打开）仍直接盖印。

## 部署插件与运行装配

插件实例只来自部署配置的顶层 `plugins`（严格解码，本机夹具 `stores.yaml` 用同名字段）：

```yaml
plugins:
  declarative-index:
    instances:
      main: {catalog: declarative-index/opensearch@2026-10}
  liveaccess:
    instances:
      main: {protocol: resource-access/v1}
```

索引实例声明它解释的能力清单名 `catalog`；LiveAccess 实例声明 `protocol` 与可选
`runtimes`，按取值描述的 `runtime` + `protocol` 认领。不写 `plugins` 时安装编译默认
（LiveAccess `main` 认领该协议全部 runtime，索引后端不为 `none` 时再装声明式索引 `main`）；
写成空映射则什么都不装，声明照常读写，取值或搜索明确报没有能力。同一清单名对应多个实例、
同一 runtime 被多个访问实例认领、未知插件或实例名、实例里的未知字段（包括不支持的 `endpoint`），
以及残留的按仓 `kc.plugins` 选择，部署都拒绝启动。格式与贡献合同归
[`discovery/plugin`](../discovery/plugin/README.md)。Home 构造真实插件入口，固定 Object 读取、
Writer 和正文缓存保持独立。

自有连接的冷恢复只重建来源句柄，不读取来源上的任何插件配置。Server 在激活后台维护前，
由根 Home 调用一次 `VerifyIndexPlugins`，按当时已恢复的全部来源重新打开部署实例并替换
冷态运行视图。Handler 构造不触发这一步；显式 `ManagedHTTPHandler.Start`，或嵌入式第一条
需要运行资源的 typed 任务，完成装配并激活同一个根。静态插件说明保持冷态。
请求借用的 `ReadView` 无权装配；运行视图不随请求、凭据轮换或来源 metadata 更新重新加载，
运行实例激活后拒绝重新装配。部署配置变更在下一次根装配生效。

`ExplainPlugin` 只读取实际入口登记的静态说明，不打开 Home、来源或引擎，不验证就绪。
Server 的 `BeginShutdown` 先拒绝新工作，再调用 `StopMaintenance` 取消并等待维护；此时保留查询资源，
让正在执行的业务 HTTP 完成。排空后调用根 Home 的 `Close`，释放一次所属资源。
借用视图不关闭根资源；说明缺失或扩展配置失败不改变基础固定读取的合同。
维护使用同一可取消来源视图完成 HEAD、固定版本读取与缓存预热；系统仓包装保留只读限制，
自有连接把取消传入来源打开、初始提交核验及实际读取，仍在每次操作时读取当前凭据。
取消不记为维护失败，也不推进未完成的目标；重启后按保留的目标继续准备。

## 首次准入与受限分享

`AdmissionConfig` 只声明可选的外部申请入口 `requestURL`。KC 的 admission 查询汇总本人
当前 grants 与 grant 管理者，不承载申请、审批或自动发权策略。

登录不应用准入。当前可信人类用户请求 `admission request` 后，应用在 `stateDir/allow.json` 中一次原子保存准入回执和规则。Agent/service 不能以 authenticatedUsers 策略申请；显式 local 测试身份除外。后续重试、重启和策略增补不重新应用该用户已完成的准入；已撤销规则保持撤销。`AdmissionResult` 同时报告原始决定与当前仍存在的动作。

仓的 `ShareActions` 是供给时冻结的消费动作白名单，与 `CreatorActions` 分开。Server 还检查发起人当前拥有的整仓能力，只有两者交集可以分享。分享管理权自身不在可分享动作中。带 `ShareID`/`SharedBy` 的仓级规则与管理员规则共存；按仓和 share ID 撤销，不删除其他仓或无分享来源的规则。

托管请求的 principal 不重写，owner 即规范用户名。原 command 或原 name 的重试仍使用原请求摘要，保留仓身份、管理地址、allocation 与初始发权回执；其他字段变化及同名歧义拒绝。归属解析不发权，实际业务请求继续验证当前 grant。

## 自有 Snapshot 连接

`connections` 显式开放已有 lakeFS 仓的自助接入。`allowedOrigins` 是部署批准的 lakeFS origin，不能带凭证、查询或 fragment。用户不能据此探测任意网络地址。该入口只接受 lakeFS。

```yaml
connections:
  allowedOrigins: [https://lakefs.example.org]
  creatorActions: [repository.connections.manage, repository.metadata.read, writer.preview, writer.commit, object.read]
  shareActions: [object.read, dataset.resolve, file.read]
```

`repositories.connect` 授予服务的连接准入。`ConnectRepository` 只读验证已存在的非空 Snapshot，不建仓、不初始化分支、不写 stamp。固定 lakeFS 仓库名与初始 commit 和逻辑 identity 一起进入 Server 私有 `connections.db`；登记表 只登记 identity。`creatorActions` 必须显式包含 `repository.connections.manage`，其余动作沿用窄创建动作白名单。初始 grant 使用稳定 allocation 回执只应用一次，重放连接不能补回撤销的权限。`shareActions` 在连接时冻结；分享仍要求发起人当前拥有相应整仓能力。

`GetConnection` 返回 owner、管理 URL、初始 HEAD、状态与 revision，不返回凭证；`CheckConnection` 只读验证并报告当前 HEAD；`RotateConnectionCredential` 只接受新凭证，先验证同 provider identity 与初始 commit，成功才原子替换秘密。三个管理动作都要求当前仓级管理权以及 durable owner 相符；知道一个仓的 identity 不意味着可以替换它的凭证。旧绑定和可用凭证在失败时保留。

用户显式授权的逐仓 credential 是部署凭证环境注入规则的窄例外：仅在模式 `0600` 的私有 `connections.db` credential bucket 保存，和公开 binding 分开，只有具体 adapter 打开时读取。它不进入配置、登记表、对象、日志、trace 或客户端 pin。备份必须包括该私有账本及初始化回执；回执存在后账本缺失或损坏会失败，不重新创建空账。

冷恢复从私有账复原固定 lakeFS authority 句柄，不联系 provider。普通 Snapshot 操作读取当前 credential；显式检查与轮换才按各自合同核验仓库绑定、初始 commit 和凭据，失败保持原绑定。管理查询与轮换继续遵守当前仓级权限和 durable owner。恢复不写入外部仓，也不自动更换 authority；上层只消费 Store 与明确可选能力合同。

## 同版本正文缓存与预热配置

Home 默认装配同一份 `object/cache` 到 Reader、请求级 ReadView、Workspace Serving 与
Index 回读接缝。缓存持有 Snapshot 声明正文，不持有动态 State observation 或裁剪后的权限结果。
默认预算为 64 MiB 计量字节、4096 条目；该计量不是进程 RSS 上限。关闭 Home 先停止后台消费者，
再清空缓存。一次性 `Open` 不启动维护；Server 在显式启动或嵌入式首次运行任务时启动维护。

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
不新增公开对象查询字段或运维路由。缓存 miss 始终可同版本回源；旧 pin 不跟随预热到新版本。

## 库存发现入口

使用者从 `show` 的消费库存选择 Dataset，再用命名 Dataset 的固定发布修订搜索和回读；库存列举不经过对象索引。仓读权不代替 Dataset 消费权，查询也不依据调用者全部可读来源临时组范围。

`RegistryBinding.discoveryWorkspaceId` 仍可作为既有部署的名称提示，不自动收集来源或提供授权例外。旧 `inventoryDiscovery: true` 请求明确拒绝，不能用库存权限绕过 Dataset 的完整 `file.read`。投影 describe 使用消费范围，sync 要求该 Dataset 的 `dataset.manage`，不由消费动作授予构建成本。

旧单空间耐久回执可以只读恢复：原 deployment marker 必须恰含一个权威并与唯一 `registry.id` 一致；混合格式、多权威和重复别名拒绝。历史 managed allocation 按原 request 字节验证摘要，原 allocation、初始政策与 READY 回执不因公共选择字段退出而重算；重放不会补权或改写记录。未完成创建由显式重试迁到新格式，显式 `deployment init` 才写新版 marker。旧连接记录同样严格核对原登记权威。

已完成的托管回执可以只读核验历史对象动作拼写，原动作和授权不变。未完成分配若仍含历史动作，必须使用原版本完成或显式处置；新版本拒绝恢复，不把失效动作传给发权回调后记成完成。新部署配置仍只接受当前动作拼写。

原控制台托管创建复用当前耐久分配与登记流程。`creatorActions` 允许显式选择 `snapshot/access.IsRepositoryCreatorAction` 的原 Repo 动作；物理删除、import、创建与授权管理不会自动发给创建者。分享只允许 `IsRepositoryReadAction` 的原仓读动作，不自动扩大到写。
