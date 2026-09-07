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
  creatorActions: [writer.preview, writer.commit, knowledge.read, knowledge.provenance]
```

Dolt 使用独立耐久 root；Gitea 使用部署声明的 owner URL `dsn` 与服务凭证，客户端不接触凭证。创建动作不要求目标仓预先出现在 repositories。creatorActions 必须显式选择允许的窄动作：`writer.preview`、`writer.commit`、`knowledge.read`、`knowledge.provenance`、`knowledge.history.read`、`knowledge.schema.read`、`knowledge.search`、`knowledge.relations`；不提供默认发权，不允许 Catalog/Workspace/Admin 等管理动作；创建者获得的普通 grant 可由授权管理面撤销。重复创建请求返回原结果，重启与重试不得补回已撤销权限。新仓 binding、创建身份与分阶段进度保存在 `stateDir/managed.db`；RESERVED → OWNED → REGISTERED → READY 逐步记录分配、拥有来源、Catalog 准入与创建完成。已完成结果保留初始 HEAD，重复请求不把它替换成当前 HEAD；Catalog 仍只保存 Git 成员与配方。

`repositories` 只是运营者批准的连接绑定，不代表 Catalog 成员关系。`catalog repo attach --repo <id>` 使用静态绑定或已供给托管仓的耐久绑定，只读检查已有 Snapshot，再通过一次 Catalog Git 提交完成接入；托管仓可据此加入另一 Catalog。没有单独手工 register，也没有持久化的第二份 attached 状态。Dolt 完全没有 Knowledge 原生表时可作为纯 Snapshot 接入；已有但不兼容的原生表必须报错，不能在接入或恢复时迁移。

System 的二进制信任根默认以不可变内置发布提供。若要在外部 Snapshot 发布，将 `kr://kc/system` 加到 `repositories` 后显式执行 `kc deployment system publish --config`；该操作使用 System Writer，来源绑定仍只存在于配置中。

## 权威与恢复

| 内容 | 所有者与存储 | 实例替换 |
|---|---|---|
| Catalog 身份、成员、Workspace 配方、归档与历史 | 配置指定的独立 Git remote/ref | 拉取恢复；写成功以远端接受 CAS 为准 |
| Snapshot 内容、版本与候选分支 | 静态连接或托管仓记录指定的 authority | 只读打开；不得初始化、stamp 或迁移 |
| 托管仓连接、创建请求与分阶段进度 | `stateDir/managed.db`；Snapshot 仍在托管 authority | 恢复新仓连接及原创建结果，不改静态配置、不补发已撤销 grant |
| grants、hooks、gates | `stateDir` 内应用数据 | 持久卷保留，不从 bootstrap 配置重建 |
| Writer command-id 回执、提案、Preview、验证记录 | `stateDir` 内账本和 control state | 保持幂等重放与维护流程连续性 |
| 过程证据、访问证据、待投递 outbox | `stateDir` | 保留；队列清空后允许删除空 outbox 文件 |
| Catalog 工作副本、投影与 checkout | `cacheDir`；检索投影也可由配置的外部 provider 保存 | 可丢弃并按明确版本重建 |
| endpoint、driver、路径、认证模式、首个管理员 | 声明配置 | 由部署系统恢复；bootstrap 只在显式首次初始化使用 |
| token、password | 外部凭证提供方 | 重新注入，不能写入配置或 Catalog |

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
