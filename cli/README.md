# cli/

KC Client 与部署管理的 **transport**：公开 argv、远程 typed Client 调用、产品输出及 `serve`
进程入口。typed HTTP handler 在 [`server/`](../server/README.md)，共享应用操作、授权与证据
在 [`invocation/`](../invocation/README.md)。读取部署配置、恢复权威与耐久状态在
[`home/`](../home/README.md)；HTTP method+pattern 闭集由 [`server`](../server/README.md) 持有（`routes.go` 字面表与 `routes_golden.txt` 金样）。
协议实现仍在各自包。

应然设计见 [CLI 交互](../doc/design/affordance.md)。公开命令的操作语义（审查用）见 [`SURFACE.md`](SURFACE.md)。路径闭集由 `surface.go` 静态装配 KC 命令及 Snapshot、Object、Discovery、Declarative Index、LiveAccess 五个模块的 `Commands()`；HTTP 路由分母是 `server/routes_golden.txt` 金样，与 mux 登记对账。

生产 CLI 与 HTTP 的注册表独立，`deployment` 与 `serve` 读取显式 `--config`；业务 CLI 只经 Server：

| 表 | 文件 | 用途 |
|---|---|---|
| 分组 CLI | `surface.go` 与五个模块的 `cli/commands.go` | 静态装配公开命令路径与输入；产品命令经 typed SDK 调 API |
| HTTP namespace | `server/service_routes.go` | 显式登记 typed route；调用相同应用操作体，不解析 argv、不读 CLI 表；闭集由 `server/routes.go` 字面表与 `routes_golden.txt` 金样对账 |

组件测试另外在 `_test.go` 中登记内部 handler 与 stage，通过 `Invocation` 调用同一服务操作。
这份嵌入装配不进入产品二进制；正式 `Run` 没有本机业务执行分支，也不持有第二份业务 registry。

```text
surface.go  ──产品命令──►  remote_*.go  ──►  client/  ──►  HTTP
            ├─Snapshot──► snapshot/cli/ ──► snapshot/client/ ──► HTTP
            ├─Object────► object/cli/ ───► object/client/ ───► HTTP
            ├─Discovery─► discovery/cli/ ──► discovery/client/ ──► HTTP
            ├─索引运维──► declarativeindex/cli/ ──► declarativeindex/client/ ──► HTTP
            └─当前态────► liveaccess/cli/ ──► liveaccess/client/ ──► HTTP
server/service_routes.go  ──typed HTTP──►  同一组应用操作（不经 CLI parser）
server/routes_golden.txt  ──测试对账──►  mux 已登记的 method+pattern
仅 _test.go 内部操作表 ──► invocation/verbs_*.go ──► 同一组应用操作
```

部署直接使用 `home/` 的公开入口；根 CLI 不重导出 Home 或通过私有别名打开组件。

## 文件按簇

按前缀读，不要按「每个动词一个文件」找。

### CLI 传输

`run.go` 入口；`parse.go` 把 argv 收成 flags；`surface.go` 解析分组路径；
`help.go` 保留总览、跨模块旅程与帮助渲染，根总览组合 Object 本地 `ModelHelp()`；
所属 `*_adapter.go` 从模块同源声明绑定
操作数、参数检查、叶说明与帮助组；KC 仓供给帮助仍归宿主；Object 的读取、写入、Schema、历史与来源叶说明归 `object/cli`。
模块的 `HelpTopics()` 同源提供分组引导与概念页，帮助入口可直接访问；这些主题不加入
业务命令表。KC 的 consume/compose 旅程衔接来源发现、选源发布、显式授权与文件消费。
宿主叶说明按 `help_host.go`、`help_writer.go`、
`help_operations.go` 分任务持有，中央只合并并拒绝重复登记。全部公开叶及 serve 说明具体
输入、前提、结果、后果、风险与成本、失败恢复和示例；help 的两种公开写法不执行该动作。
共享 flag 读取与校验分别在 `invoke/flags.go`、
`invocation/flags_hygiene.go`。

### 远程 Client

`remote.go` 与 `remote_*.go` 把公开 CLI 编成 `client/` 调用。`remote_login.go`
只处理参数与显示/浏览器交互；[client/session](../client/session/README.md) 统一管理本机登录态、
服务地址选择、逐请求重读与刷新，kc 与 kcfs 复用同一实现，不设服务端 Session。

`kc` 静态装配可独立组合的 [`snapshot/cli`](../snapshot/cli/README.md)。模块经
[`snapshot/client`](../snapshot/client/README.md) 的 typed SDK 调用 Snapshot HTTP；
宿主传入既有认证运输与主体上下文，不另建登录或会话。

| 文件簇 | 职责 |
|---|---|
| 五个模块 `*_adapter.go`、`remote_dispatch.go` | 按模块声明装配输入/导航，注入同一 Client、转换请求上下文并分派；不复制模块叶规则或输出整形 |
| `object/cli/commands.go`、`executor.go`、`output.go` | 对象叶声明、统一引用输入、任务执行与产品结果；完整引用传给 Object 服务，宿主不复制规则 |
| `discovery/cli/` | 通用查询、关系、遍历、访问概览及统一插件说明读取的声明、输入、结果和帮助 |
| `declarativeindex/cli/` | 既有投影运维任务及帮助；不持 Index 或 Controller（插件静态 Affordance 与清单在 `declarativeindex/capability/`） |
| `liveaccess/cli/` | 当前态访问、调用、绑定说明的声明、参数、产品结果与帮助；采用宿主注入的固定范围 |
| `snapshot/cli/commands.go`、`executor.go` | 库存、仓登记与登记历史、Dataset 发布/详情/退役与同源 help/flags |
| `snapshot/cli/recipe.go`、`overlay.go` | 配方输入与本机配方合成 |
| `snapshot/cli/files.go`、`file_read.go` | 交付目录分页、字节块与完整单文件下载 |
| `snapshot/cli/access.go`、`grant_update.go` | 显式授权管理、规则列表与观察旧规则的原子修改 |
| `remote_repository.go`、`remote_connections.go`、`flags_managed_repository.go` | KC 仓供给、连接、归属恢复与共享等宿主流程 |

`surface.go` 和各模块 `Commands()` 是可定位的静态来源，`remote_dispatch.go` 负责分派。
模块不依赖 KC `cli/client/invocation/home`，不复制 Snapshot 的发布、授权或文件路由实现；
KC 保留全局 parser、认证会话、统一编码、部署与跨模块装配；模块产品结果由模块拥有。
Snapshot 叶 help 按可供性说明用途、输入、条件、结果、后果、风险与成本、失败恢复及示例，
独立模块、`kc help <命令>` 与 `<命令> --help` 显示同一完整内容。

首次目录发布使用 `kc writer commit --repo <id> --dir <目录> --command-id <id> --bulk`，
CLI 自动生成 ChangeSet。省略 `--base` 时读取目标 ref 的固定 head；服务端明确证明该 commit
的文件树为空后，CLI 省去逐对象 Resolve。已有任意文件、服务端未提供空树信息或显式指定历史
`--base` 时沿用差异对照；空树信息读取失败或格式错误会阻止提交。Writer 独立检查固定基点，
`--bulk` 仍只选择介质传输方式，不影响 Schema 校验、CAS、回执和单次提交边界。

`kc plugin <名称> help` 经同一已认证 Server 读取登记说明；`kc help plugin` 本地显示该任务用法。
对象访问概览沿既有 `operations access-spec describe` 保留逻辑合同并附本次来源的插件帮助入口；
说明不证明支持或就绪，不在 Schema JSON 中附加运行匹配，不为说明读取启动来源或维护。

### 部署与应用过程

| 文件 | 负责 |
|---|---|
| `deployment.go` / `serve.go` | 显式部署管理与 Server 进程入口 |
| `invocation/home_audit.go` | stateDir 下的 `audit.jsonl` / `system.jsonl` 耐久过程账 |
| `invocation/home_hookrun.go` | 动词 pre/post 出站 Hook |

恢复独立登记 Snapshot 权威、耐久控制状态、预配置 Snapshot binding 和派生缓存见 `home/`。
`serve` 不初始化；`attach --repo` 在服务端验证既有 authority 后原子登记。

普通使用者运行 `kc create --name <显示名> [--store <允许池>]` 供给托管
Repository，或运行 `kc create --url <地址> --credential-file <文件>` 连接获准来源上的自有
Repository。一套服务只有一个登记空间。LakeFS 上 `--name` 就是协议 `--repo`，与
Graveler 仓库名相同；Server 仍生成恢复命令，产品 argv 不接收 `--catalog`、`--repo` 或
`--command-id`。Snapshot 只支持 lakeFS；低层显式坐标创建仍只服务 typed HTTP 与测试。
create 只让 KC 能打开该 Repository，不登记来源成员；用户随后显式 `attach --repo`。

管理 API 为 `POST/GET /snapshot/v1/connections`、`GET /snapshot/v1/connections/{repository}`。
普通创建与库存输出由 `ManagedRepositorySummary` 拥有，自动分配 command 留在服务内部；详情
另含当前发布与检索准备状态 `RepositoryReadiness`。显式协议坐标创建仍保留
`home.ManagedRepositoryResult` 的调用者 command 回执。`GET /repositories/{repository}` 只
交付不带仓数据的网页外壳，所有内容仍经同源 typed API 认证授权。

`RepositoryReadiness` 分别报告 `publication/publishedCommit`、可查询时的 `schemaCount` 与 `search/searchBasis/reason`。README 是仓内对象，不进入这份就绪结构。检索状态需要另有当前仓的 `projection.read`；只有固定发布版本通过 `index.CheckSearchProjectionAt` 的 READY、AccessDigest 与物理提供方校验，才返回 `READY`。已观察到的 `BUILDING/UPDATING` 表示处理中，`FAILED/RETIRED` 表示失败或停用，`NOT_READY` 表示尚无可用投影；都不改变已经发布的版本。`NOT_CONFIGURED` 表示没有索引实例，`NOT_AUTHORIZED` 只表示明确拒绝，提供方或授权存储异常为 `UNAVAILABLE` 并带错误原因。此只读状态不会触发查询或重建，也不代替具体查询的能力与权限检查。

### 应用操作

生产 CLI 经模块 SDK 请求 Server，服务端由 `invocation/verbs_*.go` 适配所属任务服务。
本包旧 command/verbs 注册与嵌入夹具执行链仅保留在 `_test.go`，用于组件测试；
模块叶参数、帮助、产品结果及回执恢复由所属 CLI/SDK 拥有。
跨 Dataset / Object / File Gateway 复用的固定版本与消费上下文由
`invocation/dataset_access.go`、`invocation/task_pin.go` 持有。

公开原始文件入口归 Snapshot Service；语义 Workspace File Gateway 与 `kcfs` 由应用装配。
旧 `catalog/worktree` Git checkout / sync 已退役，不能作为 Snapshot 合同之外的入口。

`invocation/request_shape.go` 把 flags 编成 `SearchRequest`，共享用例由 `discovery/service/` 执行。
`invocation/allow.go` 是授权求值；Dataset SEARCH 按固定发布版的 `file.read`
消费权限准入，命中在同一 Dataset 修订内回读并保留来源仓与版本。Dataset 消费权不授予来源仓
直接读取权限；按仓直接读取仍检查 `object.read`。访问证据不在这里。

`AllowFile.initialGrants` 保存 allocation → 请求 digest 的一次策略应用回执，与初始仓级规则原子持久化。普通 `grant remove` 保留这份回执，create retry 不会重建已撤销规则。策略可明确授予仓级 `writer.receipt.read`；receipt 授权从耐久 command entry 取得真实 Repository/ref，不信任调用者指定的仓。托管创建的过程回执不进入登记 Snapshot 或 Writer command ledger。

### HTTP Server

| 文件 | 负责 |
|---|---|
| `cli/serve.go` | 进程生命周期；经 `server/` 装配 handler |
| `server/serve_handler.go` / `server/service_invoke.go` | handler 装配与认证后的并发读写边界 |
| `server/serve_facade.go` | `httpFacade`、`/health` `/livez` `/readyz` `/metrics` |
| `server/service_routes.go` | 正式 namespace、typed DTO 与 handler |
| `server/service_management_routes.go` | Admin / Operations / Snapshot 写面 |
| `identity/registry.go` / `identity/request.go` | 可注册认证器与唯一的请求认证判定 |
| `server/serve_auth.go` | 请求入口认证与 admin 判定 |
| `identity/local/` / `identity/taihu/` | 具体认证器 |
| `invoke/http.go` / `invoke/http_write.go` | 请求解码与统一错误信封 |
| `server/internal/readiness/` | 分 Surface readiness |
| `server/serve_state.go` | 保留 StateLookup 公开别名；HTTP adapter 在 `liveaccess/state_lookup.go` |
| `server/serve_telemetry.go` | HTTP SERVER span 与 completion log |
| [`snapshot/service/file_http.go`](../snapshot/service/file_http.go)、[`file_view.go`](../snapshot/service/file_view.go) | 固定发布的文件网关、单目录分页与字节范围交付 |

### kcfs

`kcfs` 是 Snapshot 的外部宿主客户端，不属于上面的 Snapshot CLI 文件分组。
`cmd/kcfs` 是独立进程入口，`cli/datasetfs*.go` 经 Client 消费文件网关，
根目录 `datasetfs/` 承接挂载计划、FUSE 与宿主生命周期；这些代码保持在 `snapshot/` 之外。
直接打开夹具 Home 的 projection、prepare、目录分页与相对路径适配只在 `_test.go` 编译；
正式挂载计划与文件访问都由远程网关返回。
Snapshot 文件网关拥有固定范围、当前授权与文件交付，宿主挂载职责不进入该模块。
help 可以交叉链接独立工具，导航关系不改变模块归属。

### 遥测与证据

`run.go` 接入产品 CLI 调用观察；嵌入 Invocation 的遥测与证据装配仅随 `_test.go` 编译。
服务端共享应用 OTel 适配在 `invocation/telemetry*.go`。
`invocation/observability_access.go` / `invocation/observability_retrieval.go` 写访问账和检索证据，
不是 `allow.json`。

`kc serve --config` 只恢复；配置缺失不生成新部署。`dataset overlay` 是客户端临时配方合成，不保存 Server Home overlay。
Agent 入口是作为 typed Client 的分组 `kc` CLI；文件读取走由 Workspace File
Gateway 支撑的宿主挂载目录。

## 认证 adapter 与客户端登录

设计边界见 [权限体系](../doc/design/permissions.md) 与 [`doc/design/service.md`](../doc/design/service.md)。公开配对发现为 `GET /identity/v1/auth`，身份查询为 `GET /identity/v1/whoami`；路径和 DTO 由 HTTP registry 与 `client/` 维护。

| Server 模式 | 业务凭证 | 拒绝 |
|---|---|---|
| `local` | `X-Kc-As` | `Authorization`、自报委托、空身份 |
| `taihu` | `Authorization`（Taihu 可经已验证网关头） | 自报 `X-Kc-As` / `X-Kc-On-Behalf-Of`、空身份 |

lakeFS 只作存储，原 `lakefs` 认证模式已退役。浏览器请求检查由 [`server/README.md`](../server/README.md) 维护，
认证器的组织与替换方式见 [`identity/README.md`](../identity/README.md)。

认证后的自报身份或委托返回 `FORBIDDEN`；缺少匹配凭证返回 `UNAUTHENTICATED`。
Cookie 防跨站失败返回 `FORBIDDEN`。进程内空 `HTTPServerOptions` / `HTTPHandler(home)` 是 local 测试接缝；正式部署必须显式选定模式。

Taihu 资源方配置使用 `KC_SERVICE_CLIENT_SECRET` 作 introspection 应用密钥，`KC_TAIHU_HMAC_SECRET` 作网关 hex 验签密钥；两者不是调用方身份。`--auth-hmac-secret` / `--service-client-secret` 可以覆盖环境值，但不得把真实秘密作为 argv 字面量传入。精确配置见 `identity/taihu/register.go` 与帮助文本。

已验证人类身份的 `principal` 是 `identity.CanonicalUsername` 接受的用户名，例如 `kaiqidong`；Taihu subject 和认证 issuer 只进入内部可信绑定，不作为公开身份或授权键。Taihu 有 actor 的委托保留 `agent:<id>`，`onBehalfOf` 为用户名；无用户且只有 client 时保留 `service:<client_id>`。Taihu 认证器构造 `HTTPIdentity.User`，Server 在授权前通过 `identity/` 核对 `stateDir/identities.json`。同名不同 subject/provider/issuer、同 subject 改名均拒绝，不通过重新登录接管权限。网关必须提供 HMAC 签名、用户名与稳定 subject；裸 `X-Tai-User` 和未签名身份头不再是认证方式。

新部署初始化身份账，旧部署显式 `deployment init` 才建立该账；已有初始化回执后的缺失或损坏只能恢复。旧版带提供方前缀的主体（如 `taihu:<username>`）不再被接受，也没有迁移入口。

纯 Taihu 网关模式同样必须配置可信 `authURL`；签名身份没有可信 issuer 时拒绝绑定。

`kc login` 按入口优先级发现服务模式，再进行配对；已签发 Bearer 可通过 `KC_AUTH_TOKEN`
提供。`--server` 只是覆盖入口，不是登录专属日常操作数。身份验证和本机持久化都成功后才
返回 `{status:"authenticated", principal, …凭证事实}`，回执不重复 Server 地址。服务地址和可信身份构成客户端上下文；不保存 Catalog 选择。

非秘密连接配置保存为 `$KC_CONFIG_DIR/client.json`（默认 `~/.config/kc`），会话位于该目录 `sessions/<Server摘要>/`，以原子替换和 0600 权限保存。地址优先级是 `--server`、`KC_SERVER_URL`、保存的客户端配置；旧单服务登录文件只作匹配 Server 的兼容读取。切换 Server 不发送旧服务凭证；请求不跟随携带凭证的 HTTP 重定向。CLI 与文件网关使用同一会话，过期后按同一服务续期，注销不会修改已固定的文件视图。local 身份来源对 Taihu Server 返回 `USAGE_INVALID`。

会话保存、过期拒绝与刷新回归调用现行客户端路径；应用秘密与凭证交换通过实际 Server
broker 验证。`legacy_login_test.go` 仅保留已退役直连 OAuth 交换的历史材料，其断言不代表
当前产品入口。嵌入 admission 夹具调用真实 Snapshot Service，不另写规则展示算法。

浏览器组织登录的无状态接口为 `POST /identity/v1/authorize`、`POST /identity/v1/authorize:poll` 和 `POST /identity/v1/token`。前两者只传送固定部署的 PAR 请求/状态；token 接口的严格输入由 `client.TokenRequest` 定义。它们不建立 Server Workspace session、不发权，不接受调用者指定上游、应用 ID 或应用秘密。

Resource Access 参考装配按 Domain Schema（或 ResourceDescriptor）上的 `origin` 调用独立 `resource-access/v1` runtime。`kc serve` 不再接受整台机器一个访问 URL。容器间使用对方能解析的服务网络地址；调用方容器的 `localhost` 不指向另一个服务。出站 `POST {origin}/v1/access` 转发已经通过 Server 认证边界的调用方证明：Taihu 的 `Authorization`、Taihu 网关的 `X-Tai-Identity`，以及 `X-Resource-Principal`。local 配对只带 principal。接入方可以忽略这些头；KC 不得省略。凭证不进 Schema、flags、访问账或 JSON body。

## 查权、申请入口与发权

`kc admission show` / `GET /identity/v1/admission` 返回当前主体的 grants，以及部署可选的
`admission.requestURL` 和当前持有 `admin.grants.manage` 的主体。KC 不保存申请或审批状态，
也没有 POST admission 或 `admission request`。

权限统一由 `kc grant add|list|remove` 管理。`grant add` 可限定 `--repo` 或 `--dataset`；省略两者为全服务范围。
Repository 是授权范围，不形成 `repo grant` 子树。底层 share/connection typed HTTP 可继续
服务管理集成，但不属于产品 argv。

`grant list` 无 action 时返回 `{rules: [...]}`，按明确的 principal、Repository、
Dataset、ref、object、aspect 过滤，不暴露授权账本私有回执。
提供 action 时保留既有显式规则求值（省略 principal 则问当前主体），需要全局
`admin.grants.read`；scoped read 只能列举明确范围，不能求值全局规则。
远程 CLI 通过同一受控库存获取规则后使用既有求值函数，不另设评价路由，
结果不包含库存发现或仓默认消费声明，也不替代下一次产品请求的服务端授权。
发权与撤权共用明确目标范围检查：撤权范围取自存储规则，不使用客户端默认范围。
新 grant ID 为 opaque 值，原 ID 不迁移；重复 ID 撤权失败关闭，旧撤权 ID 不复用。
`admission show` 保留显式规则的 Dataset/ref/object/aspect 限制，不能把受限规则
展示成整个服务或仓的权限。`manage` 不隐含 `read`，自身或最后管理员撤权仍允许。

已有自有 lakeFS 来源通过 `kc create --url --credential-file` 连接。credential 只进入 typed
请求体和 Server 私有耐久账，不进入日志、trace 或公开结果。连接成功不等于 attach，也不发
`object.read`。

对象消费只接受 `--repo` 或 `--dataset`；没有 Catalog 选择范围，也不接受
`--catalog`、`--source`、`--pin`。Writer、`diff` 与 `schema list` 拒绝 `--dataset`。

`kc dataset define` 可通过配方中的可选 `metadata`，或 `--metadata '<JSON object>'`
提交自定义字符串键值。显式参数提交完整对象并替换配方中的属性，`--metadata '{}'`
清空；省略参数则保留输入配方中的属性。元数据随本次 revision 发布并在库存与历史详情
回读，不检查任何说明文件。HTTP / Client 与控制台使用相同定义字段；原生 Repository
元数据由 Snapshot Client 的原 lakeFS 路由读取、增量设置和按键删除。

## Snapshot 详情、文件与授权 CLI

`kc dataset show <id> [--revision <n>] [--limit <n>] [--continuation <cursor>]`
查看已接受定义与一页历史；`revision` 是所选版，`servingRevision` 是当前服务版。
历史游标在新发布后失效，退役仍可查看定义；详情可见不发放文件消费权。

`kc files list --dataset <id> [--revision <n>] [--path <dir>] [--limit <n>] [--continuation <cursor>]`
枚举交付目录的直接子项，条目的 `path` 可继续传给 `files read`。省略路径从交付根开始；
页面上限默认 50、最多 200，0 使用默认。首次省略发布版只取当前一次，续页须显式沿用返回的
`revision`；派生条目先于来源目录页，带游标的空页不是完成。响应保留 `dataset`、发布版、
`pinId`、来源提交、条目和分页结果，但用户不需要传输 pin 文件。

`kc files read --dataset <id> [--revision <n>] --path <file> [--offset <n>] [--length <n>]`
或 `kc files read --repo <id> --commit <commit> --path <file> [--offset <n>] [--length <n>]`
返回一个 JSON 字节块；`content` 是标准 base64，偏移与总大小按字节，`eof` 表示文件尾。
`--repo` 与 `--dataset` 互斥。按仓读取必须显式指定固定 commit 和相对于原仓根目录的路径，
检查该仓的 `file.read`，不自动选择分支最新版、不要求创建 Dataset。
对象 definition 中的文件地址是普通值；按业务说明取得资源自己的仓、commit 和路径后调用。
资源可在另一个仓、采用独立版本，不能默认套用对象说明自己的 commit；对象读权不产生文件读权。
默认块大小 512 KiB，最大 4 MiB，0 使用默认。不将二进制解释成文本，也不向 stdout 写原始字节。
指定 `--out <file>` 下载完整单文件，与范围参数互斥；同一固定 commit 或 Dataset 发布贯穿全部块，每次仍检查
当前权限。完整成功后才发布目标，拒绝覆盖既有文件或符号链接；失败清理临时文件。
源码端仍可能整文件读取后切片，不承诺源端 I/O 或内存受返回范围约束。

`kc grant update --id <id> --file <replacement-rule.json> --expected-file <observed-rule.json>`
读取两份 `grant list` 中单条规则的规范 JSON；新规则完整替换可编辑字段，省略范围会清空。
适配明确转换规则的 `repo` 为 Admin PUT 的 `repository`；不接受整份库存或 HTTP 包装请求。
规则 ID 与来源不可伪改，expected 始终是此前观察，不在提交前拉新值；旧/新范围管理权、
条件冲突和持久化由现有 Snapshot 原子更新边界判定，失败不留下撤销再新增的中间态。

这些任务的交互由 `snapshot/cli` 拥有，产品入口只经 Server；内部嵌入夹具复用所属服务。
模块单元测试随 owner 保存，根保留 argv、认证上下文、跨模块旅程与实际 Server 集成测试。
独立 `kcfs`、根 `datasetfs/` 的挂载职责不进入 Snapshot 目录。
