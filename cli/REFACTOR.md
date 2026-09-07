# CLI 命令体系与验收说明

本文说明当前命令分工、选定理由与验收方式，替代此前的阶段实施计划。公开路径以
[`surface.go`](surface.go) 为准，帮助以 [`help.go`](help.go) 为准；本文不维护另一份
完整命令清单，也不为已退役入口提供兼容承诺。

设计依据是文档图中拥有 Client、部署与文件网关主题的
[`SERVICE_ARCHITECTURE.md`](../docs/SERVICE_ARCHITECTURE.md)，以及其直接依赖
[`COMPOSITION.md`](../docs/COMPOSITION.md)。CLI、应用操作与 HTTP 分别注册；HTTP
合同由 [`httpsurface`](../httpsurface/README.md) 和 typed handler 共同验证。
[`HTTP_REFACTOR.md`](HTTP_REFACTOR.md) 保留 URL 迁移对照，不能替代当前注册表。

## Goal

用户从命令名和操作数就能判断：操作哪个对象、读取什么、是否写入权威，以及下一步
应使用哪份结果。日常任务从已有 Server 和身份开始；部署资源准备不成为每个知识
任务的前置旅程。一次任务需要稳定数据时，显式固定并重用同一 pin。

## Non-Goals

- 不按 provider、consumer、governor 等岗位建立命令树；help 中的 consume、write、
  compose 是旅程，不是身份或授权主体。
- 不把来源预置、Catalog 准入、知识发布合为一次隐式初始化；业务知识仍只经 Writer。
- 不把 Catalog Git 恢复等同于整个服务恢复；授权、Writer ledger、治理与原始证据
  有独立的耐久状态要求。
- 不用 Server 本机目录扫描替代声明式配置，不把组件测试接缝暴露成产品 transport。
- 不复制 HTTP URL 层级到 argv，不在本文冻结协议字段、错误码全集或路由数量。

## 不变量与命名原则

沿用 [`ARCHITECTURE_INVARIANTS.md`](../docs/ARCHITECTURE_INVARIANTS.md) 中的
`API-01`，以及组合设计的 `WS-01`、`V-01`、`WS-02`、`W-01`：入口独立注册；pin
固定成员数据坐标；一次操作不追随新的 selector；pin 不冻结权限；知识提交保持单仓边界。

| 原则 | 当前应用 |
|---|---|
| 名词对应用户对象，同类动作可以类推 | Catalog、Workspace、Knowledge、Writer、Governance、Admin、Operations 分组；Workspace 不藏在 Catalog 子命令下 |
| 执行位置和副作用可预期 | deployment 管部署，pack 与 overlay 处理客户端文件，业务命令调用 Server |
| 同一原语的便捷形式保持可追溯 | writer put/remove 都编译成单仓 commit；pack 只准备 ChangeSet |
| 操作数和上下文明确 | Catalog/Workspace 支持位置 id；Repository 和知识对象分别用 --repo、--object；不兼容的 basis 在请求前拒绝 |
| 输出适合串联 | 机器结果使用 JSON；pin、pack、overlay 可显式写 --out，文件保存后 stdout 返回对应回执 |
| 帮助和用法错误不依赖服务状态 | 分组 help 在身份、连接和任务绑定前可用；未知或错用参数不能静默丢弃 |
| 日常用户不承担部署细节 | consume/write 旅程不要求本机 Home、adapter 或数据源凭证；SEARCH 不可用不被解释为零命中 |
| 版本与授权分别判断 | 保存 pin 是保存数据依据；每次重放仍使用当前凭证和当前规则 |
| 退役入口不再参与产品解析 | 没有公开 local 或独立 catalog repo register，也不保留别名 |

## 当前分工与选定方案

| 入口 | 作用与边界 | 未采用的方式 |
|---|---|---|
| `deployment init --config` | 显式初始化声明的 Catalog ref 与初始耐久控制状态 | 启动服务时发现目录并补出一套空状态 |
| `deployment status --config`、`serve --config` | 从配置恢复并验证既有权威与耐久状态；实例缓存可重建 | 把缓存内容当作 Catalog 或来源清单 |
| `deployment system publish --config` | 显式发布内置 System 信任根 | 让业务 Writer 任意修改 System，或借此发布业务知识 |
| `catalog repo attach --repo` | 使用部署配置中的 binding，只读验证既有 Snapshot 后原子提交 Catalog 成员登记 | 先执行宿主挂仓，再要求用户额外执行一个登记命令 |
| `workspace define` | 向 Catalog 发布命名配方 | 为每次临时组合都创建长期服务端对象 |
| `workspace overlay` | 在客户端将 recipe 与 overlay 合成为临时定义或便携配方文件 | 在 Server Home 保存个人 overlay 并改变共享配方 |
| `workspace pin` | 将命名配方、显式来源或便携配方固定成任务依据；可导出供后续重放 | 每条 SEARCH/READ 都自动前进到新 HEAD |
| `pack` | 只读客户端草稿，生成 ChangeSet 与 diagnostics；需要固定写依据时显式传 --base | 隐式连接 Server 取 HEAD，或把打包称为已经发布 |
| `writer commit/put/remove` | 经 Server 在一个 Repository 上提交，保留 CAS 与 command-id 幂等 | 绕过 Writer 直接改 Git 或把 Workspace 当作可提交的仓 |
| `knowledge access`、`knowledge invoke` | 分别消费 Binding 与 ResourceDescriptor 上声明的调用 | 用一个含义不明的读命令混合取声明、读正文和调用 live 资源 |

Catalog 接入成功表示成员登记已由独立 Git 权威接受。失败不得创建业务 Snapshot、
移动其 ref，或留下可见的半个登记；成员身份不自动授予知识读权。来源位置与 driver
属于部署配置，不能通过 attach 的调用参数临时改写。

部署初始化跨越 Catalog Git 与耐久状态介质，不承诺跨介质事务。已有 Catalog 而
状态卷缺失时必须恢复或核对未完成部署，不能重新发一套空授权。System 发布是显式
信任根操作，其边界由 [`home`](../home/README.md) 与 Writer 合同拥有。

## 一次任务的时间局部

默认入口是已有环境与身份。用户先发现可见 Catalog/知识集，或者直接指定已知对象；
需要组合时再选择命名 Workspace、来源参数或便携配方。还没有绑定知识的任务不能
阻断 help、whoami、Catalog 发现和纯客户端预处理。

需要跨命令保持依据时，先 `workspace pin --out`，后续 SEARCH、READ 和验收重放
同一份 pin。临时 pin 的客户端文档同时保存未发布配方与 Catalog 选择，使退出重开
后仍能校验原来的成员与路径；它不创建服务端 Workspace，也不携带授权能力。

上游更新只影响下次显式解析。token 刷新或重新登录不改变已经保存的数据依据；撤权
阻止新的请求。已绑定任务中的显式冲突参数不能悄悄换身份、Workspace 或 pin。
失败后的修复与重试必须保持这些边界：知识提交仍检查 CAS 与幂等键，消费请求仍检查
当前权限，不能以改用最新版本或另一身份掩盖失败。

需要文件时，由 `kcfs` 通过 Workspace File Gateway 使用同一固定依据。网关借用
配置恢复的服务实例，按当前规则校验每次请求；共享实例的关闭等待在途请求结束。
目录视图、挂载生命周期与实例缓存都不成为新的知识权威。

## 接口与证据位置

| 合同 | 实现与公开接口 | 验证 |
|---|---|---|
| 公开命令、帮助和入口参数 | `surface.go`、`help.go`、`run.go`、`flags.go` | `command_test.go`、`deployment_contract_test.go` |
| 声明式部署与恢复 | `home.DeploymentConfig`、`ReadDeployment`、`InitializeDeployment`、`OpenDeployment` | `home/deployment_test.go`、`deployment_recovery_test.go` |
| Catalog 持久权威与提交原子性 | `catalog.OpenRemoteRegistry`、`CreateRemoteRegistry`、Catalog 生命周期 API | `catalog/registry_remote_test.go`、`catalog/registry_atomic_test.go` |
| 离线 pack 与 overlay | `deployment.go`、Catalog recipe/overlay 类型 | `client_preprocessing_test.go`、`remote_test.go` |
| 临时定义与任务 pin 重放 | `client/knowledge.go`、`workspace_task_pin.go`、`remote_task_context.go` | `temporary_knowledge_test.go`、`remote_task_context_test.go`、`server_client_only_test.go` |
| 文件网关与进程生命周期 | `workspace_file_service.go`、`telemetry_vfs.go`、`serve_facade.go` | `workspace_file_deployment_test.go`、`workspace_file_service_test.go`、Linux/FUSE 主线 |
| CLI 与 HTTP 独立注册且语义对应 | `client/`、typed routes、`httpsurface.Patterns()` | `remote_dispatch_internal_test.go`、`http_surface_coverage_test.go`、架构守卫 |

表中的测试是持续合同，不是一次性完成勾选。当前是否通过由本次测试结果确认，不能
沿用旧重构计划里的绿灯结论。组件夹具只用于建立测试前置状态；产品旅程必须能通过
公开 argv、正式 HTTP 配置入口及相应真实适配器执行。

## 覆盖与验收方式

命令覆盖以当前 `surface.go` 为分母，分别统计调用、断言成功、独立失败场景；不能用
调用次数替代可观察断言。状态修改类操作保持更高的边界证据要求。HTTP 分母来自
正式注册表，并与 mux、typed Client 和 HTTP-only 证据分区对账，不在本文写死数量。

场景树负责可复用的前置状态与分支，组织规则见
[`.data/scenes/README.md`](../.data/scenes/README.md)。树上有节点并不证明时间局部
完整：还要检查已有环境起步、任务固定依据、上游推进、撤权、退出重开、失败修复与
重试是否被真正串联，并核实过程中没有意外发布、重新初始化或换依据。

最终验收使用仓库规定的文档、组件、边界、应用/transport 与适用的真实适配器检查。
不通过删断言、增加 skip、保留隐藏别名或让夹具绕过产品入口来满足覆盖分母。

## 历史引用边界

旧 argv、旧 HTTP URL 可以保留在退役入口拒绝测试、明确标注的迁移对照以及测试专用
夹具中。历史 Git 记录保留原实施过程。它们不定义当前命令，也不构成恢复旧 local
分组、额外 register 步骤或兼容别名的理由。协议层 `RegisterRepository` 等公开 API
名称仍按各自包合同理解，不能仅因 CLI 已改名就推断协议能力被删除。
