# cli/

KC Client、KC Server 与部署管理的 **transport**：公开 argv、typed HTTP handler、授权与遥测。读取部署配置、恢复权威与耐久状态在 [`home/`](../home/README.md)；HTTP method+pattern 闭集在 [`httpsurface/`](../httpsurface/README.md)。协议实现仍在各自包。

应然设计见 [CLI 交互](../docs/reviewed/cli.md)。公开命令的操作语义（审查用）见 [`SURFACE.md`](SURFACE.md)。落地迁移记录见 [`REFACTOR.md`](REFACTOR.md)，HTTP 迁移记录见 [`HTTP_REFACTOR.md`](HTTP_REFACTOR.md)。路径权威仍是 `surface.go`；HTTP 路由分母是 `httpsurface.Patterns()`，与 mux 登记对账。

三张表互相不读，`deployment` 与 `serve` 读取显式 `--config`；业务 CLI 只经 Server：

| 表 | 文件 | 用途 |
|---|---|---|
| 分组 CLI | `surface.go` | 公开命令路径 → 内部操作名；产品命令经 `client/` 调 typed API |
| 应用操作 | `command.go` | Server 与测试接缝共用的内部 handler 表；键通常是公开路径把空格换成连字符。知识面 argv 无 `knowledge` 前缀，操作名仍是 `knowledge-search` 等 |
| HTTP namespace | `service_routes.go` | 显式登记 typed route；不解析 argv、不读 CLI 表、不读 `httpsurface` |

```text
surface.go  ──产品命令──►  remote_*.go  ──►  client/  ──►  HTTP
command.go  ──应用/测试──►  verbs_*.go  ──►  home.Open / Writer / Reader / Index
service_routes.go  ──typed HTTP──►  同一组应用操作（不经 CLI parser）
httpsurface.Patterns()  ──测试对账──►  mux 已登记的 method+pattern
```

`home_alias.go` 把 `home` 的类型和打开函数留在本包名字下，避免每个 verb 文件都写 import。

## 文件按簇

按前缀读，不要按「每个动词一个文件」找。

### CLI 传输

`run.go` 入口；`parse.go` / `flags.go` / `flags_operand.go` / `flags_identity.go`
把 argv 收成 flags；`surface.go` 解析分组路径；`help.go` 是帮助文本。

### 远程 Client

`remote.go` 与 `remote_*.go` 把公开 CLI 编成 `client/` 调用。`remote_login.go`
只管理本机登录态；`remote_task_context.go` 是 DSH 任务坐标，不是服务端 Session。

### 部署与应用过程（仍在本包）

| 文件 | 负责 |
|---|---|
| `home_alias.go` | `Home` 类型别名与打开函数 |
| `home_audit.go` | stateDir 下的 `audit.jsonl` / `system.jsonl` 耐久过程账 |
| `home_hookrun.go` | 动词 pre/post 出站 Hook |

恢复独立 Catalog Snapshot 权威、耐久控制状态、预配置 Snapshot binding 和派生缓存见 `home/`。`serve` 不初始化；`catalog repo attach` 在服务端验证既有 authority 后原子登记。

普通使用者运行 `kc create --name <显示名> [--store <允许池>]` 供给托管
Repository，或运行 `kc create --url <地址> --credential-file <文件>` 连接获准来源上的自有
Repository。当前 Catalog 来自 `catalog use`。LakeFS 上 `--name` 就是协议 `--repo`，与
Graveler 仓库名相同；Server 仍生成恢复命令，产品 argv 不接收 `--catalog`、`--repo` 或
`--command-id`。Gitea 中文名才生成逻辑坐标。低层显式坐标创建仍只服务 typed HTTP 与测试。
create 只让 KC 能打开该 Repository，不登记 Catalog；用户随后显式 `attach --repo`。

管理 API 为 `POST/GET /catalog/v1/repositories`、`GET /catalog/v1/repositories/{repository}`。
普通创建与库存输出由 `ManagedRepositorySummary` 拥有，自动分配 command 留在服务内部；详情
另含当前发布与检索准备状态 `RepositoryReadiness`。显式协议坐标创建仍保留
`home.ManagedRepositoryResult` 的调用者 command 回执。`GET /repositories/{repository}` 只
交付不带仓数据的网页外壳，所有内容仍经同源 typed API 认证授权。

`RepositoryReadiness` 分别报告 `publication/publishedCommit`、可查询时的 `schemaCount` 与 `search/searchBasis/reason`。README 是仓内知识对象，不进入这份就绪结构。检索状态需要另有当前仓的 `projection.read`；只有固定发布版本通过 `index.CheckSearchProjectionAt` 的 READY、AccessDigest 与物理提供方校验，才返回 `READY`。已观察到的 `BUILDING/UPDATING` 表示处理中，`FAILED/RETIRED` 表示失败或停用，`NOT_READY` 表示尚无可用投影；都不改变已经发布的版本。`NOT_CONFIGURED` 表示没有索引实例，`NOT_AUTHORIZED` 只表示明确拒绝，提供方或授权存储异常为 `UNAVAILABLE` 并带错误原因。此只读状态不会触发查询或重建，也不代替具体查询的能力与权限检查。

### 应用操作

`verbs_*.go` 按 semantic action 登记内部操作。跨 Catalog / Knowledge / File
Gateway 复用的 Workspace 流程单独放 `workspace_*.go`；`workspace_consume.go`
只留 pin、Serving 与消费授权的共享上下文。

公开文件入口只有 Workspace File Gateway / `kcfs`。宿主 git checkout / sync
由 `catalog/worktree` 持有，不是产品 CLI。

`search_request.go` 把 flags 编成 `SearchRequest`；真正的 Workspace SEARCH 在
`workspace_search.go`。`allow.go` 是授权求值；命名知识集 SEARCH 要 `file.read`
与 `knowledge.search`，正文走 `delivery/` 按仓 `knowledge.read` 屏蔽。访问证据不在这里。

`AllowFile.initialGrants` 保存 allocation → 请求 digest 的一次策略应用回执，与初始仓级规则原子持久化。普通 `grant remove` 保留这份回执，create retry 不会重建已撤销规则。策略可明确授予仓级 `writer.receipt.read`；receipt 授权从耐久 command entry 取得真实 Repository/ref，不信任调用者指定的仓。托管创建的过程回执不进入 Catalog Snapshot 或 Writer command ledger。

### HTTP Server

| 文件 | 负责 |
|---|---|
| `serve.go` | 进程生命周期与认证后的并发读写边界 |
| `serve_facade.go` | `httpFacade`、`/health` `/livez` `/readyz` `/metrics` |
| `service_routes.go` | 正式 namespace、typed DTO 与 handler |
| `service_management_routes.go` | Admin / Governance / Operations / Catalog 写面 |
| `auth.go` | 可注册 Authenticator 合同与 factory registry |
| `serve_auth.go` | 请求入口认证、Gitea 身份验证与 admin 判定 |
| `auth_gitea.go` / `auth_taihu.go` / `auth_service.go` | 具体认证器 |
| `serve_request.go` / `serve_response.go` | 请求解码与统一错误信封 |
| `serve_readiness.go` | 分 Surface readiness |
| `serve_state.go` | `resource-access/v1` StateLookup HTTP adapter |
| `serve_telemetry.go` | HTTP SERVER span 与 completion log |
| `workspace_file_service.go` | 固定 pin 的只读 mount、单目录分页与 range read |

### kcfs

`workspacefs.go` / `workspacefs_daemon.go` / `workspacefs_help.go` 是本机挂载
进程，经 Workspace File Gateway 读字节。不要和服务端 `workspace_file_service.go`
混读。

### 遥测与证据

`telemetry.go` 与 `telemetry_*.go` 是应用操作上的 OTel 适配。
`observability_access.go` / `observability_retrieval.go` 写访问账和检索证据，
不是 `allow.json`。

`kc serve --config` 只恢复；配置缺失不生成新部署。`dataset overlay` 是客户端临时配方合成，不保存 Server Home overlay。
Agent 入口是作为 typed Client 的分组 `kc` CLI；文件读取走由 Workspace File
Gateway 支撑的宿主挂载目录。

## 认证 adapter 与客户端登录

设计边界见 [权限体系](../docs/reviewed/permissions.md) 与 [`docs/reviewed/deploy-auth.md`](../docs/reviewed/deploy-auth.md)。公开配对发现为 `GET /identity/v1/auth`，身份查询为 `GET /identity/v1/whoami`；路径和 DTO 由 HTTP registry 与 `client/` 维护。

| Server 模式 | 业务凭证 | 拒绝 |
|---|---|---|
| `local` | `X-Kc-As` | `Authorization`、自报委托、空身份 |
| `taihu` / `gitea` | `Authorization`（Taihu 可经已验证网关头） | 自报 `X-Kc-As` / `X-Kc-On-Behalf-Of`、空身份 |

产品混装凭证返回 `FORBIDDEN`；缺少匹配凭证返回 `UNAUTHENTICATED`。进程内空 `HTTPServerOptions` / `HTTPHandler(home)` 是 local 测试接缝；正式部署必须显式选定模式。

Taihu 资源方配置使用 `KC_SERVICE_CLIENT_SECRET` 作 introspection 应用密钥，`KC_TAIHU_HMAC_SECRET` 作网关 hex 验签密钥；两者不是调用方身份。`--auth-hmac-secret` / `--service-client-secret` 可以覆盖环境值，但不得把真实秘密作为 argv 字面量传入。精确配置见 `auth_taihu.go`、`auth_service.go` 与帮助文本。

已验证人类身份的 `principal` 是 `identity.CanonicalUsername` 接受的用户名，例如 `kaiqidong`；Gitea numeric ID、Taihu subject 和认证 issuer 只进入内部可信绑定，不作为公开身份或授权键。Taihu 有 actor 的委托保留 `agent:<id>`，`onBehalfOf` 为用户名；无用户且只有 client 时保留 `service:<client_id>`。Gitea/Taihu 认证器构造 `HTTPIdentity.User`，Server 在授权前通过 `identity/` 核对 `stateDir/identities.json`。同名不同 subject/provider/issuer、同 subject 改名均拒绝，不通过重新登录接管权限。网关必须提供 HMAC 签名、用户名与稳定 subject；裸 `X-Tai-User` 和未签名身份头不再是认证方式。

新部署初始化身份账，旧部署显式 `deployment init` 才建立该账；已有初始化回执后的缺失或损坏只能恢复。旧 `gitea:<id>` / `taihu:<username>` 授权不在登录时自动搬迁。运营者停服核实可信绑定后执行 `kc deployment identity migrate --config deployment.yaml --file migration.json`；文件为 `{"legacyPrincipal":"gitea:42","user":{"username":"kaiqidong","provider":"gitea","issuer":"https://id.example","subject":"42"}}`。迁移只更名仍存在的规则，并与一次迁移回执原子保存；重放不恢复撤销、不改初始授权回执、不能改为另一身份。此入口不支持自动改名或替换已占用的可信绑定。

同一显式迁移在身份账中保留旧主体到已核验用户的归属别名。历史托管仓的 mine/get/原命令重试据此恢复新用户名的归属，原仓身份、后台地址、Request/digest、分配与首次发权回执不改写。尚未发放的首次规则可以写给新主体，但完成过的规则绝不重发。别名先于当前授权迁移原子落盘；中间崩溃不因归属查找而取得权限，重跑同一迁移完成后半段。纯 Taihu 网关模式同样必须配置可信 `authURL`；签名身份没有可信 issuer 时拒绝绑定。

`kc login` 按入口优先级发现服务模式，再进行配对；已签发 Bearer 可通过 `KC_AUTH_TOKEN`
提供。`--server` 只是覆盖入口，不是登录专属日常操作数。身份验证和本机持久化都成功后才
返回 `{status:"authenticated", principal, …凭证事实}`，回执不重复 Server 地址。当前
Catalog 另存于同一 Server 会话目录的 `catalog.json`，不写入 token 或 pin。

非秘密连接配置保存为 `$KC_CONFIG_DIR/client.json`（默认 `~/.config/kc`），会话位于该目录 `sessions/<Server摘要>/`，以原子替换和 0600 权限保存。地址优先级是 `--server`、`KC_SERVER_URL`、保存的客户端配置；旧单服务登录文件只作匹配 Server 的兼容读取。切换 Server 不发送旧服务凭证；请求不跟随携带凭证的 HTTP 重定向。CLI、文件网关和 DSH 使用同一会话，过期后按同一服务续期，注销不会修改任务 pin。local 身份来源对 Taihu Server 返回 `USAGE_INVALID`。

浏览器组织登录的无状态接口为 `POST /identity/v1/authorize`、`POST /identity/v1/authorize:poll` 和 `POST /identity/v1/token`。前两者只传送固定部署的 PAR 请求/状态；token 接口的严格输入由 `client.TokenRequest` 定义。它们不建立 Server Workspace session、不发权，不接受调用者指定上游、应用 ID 或应用秘密。

Resource Access 参考装配按 Domain Schema（或 ResourceDescriptor）上的 `origin` 调用独立 `resource-access/v1` runtime。`kc serve` 不再接受整台机器一个访问 URL。容器间使用对方能解析的服务网络地址；调用方容器的 `localhost` 不指向另一个服务。出站 `POST {origin}/v1/access` 转发已经通过 Server 认证边界的调用方证明：Taihu/Gitea 的 `Authorization`、Taihu 网关的 `X-Tai-Identity`，以及 `X-Resource-Principal`。local 配对只带 principal。接入方可以忽略这些头；KC 不得省略。凭证不进 Schema、flags、访问账或 JSON body。

## 查权、申请入口与发权

`kc admission show` / `GET /identity/v1/admission` 返回当前主体的 grants，以及部署可选的
`admission.requestURL` 和当前持有 `admin.grants.manage` 的主体。KC 不保存申请或审批状态，
也没有 POST admission 或 `admission request`。

权限统一由 `kc grant add|list|remove` 管理。`grant add` 的 `--repo` 与 `--catalog` 二选一；
Repository 是授权范围，不形成 `repo grant` 子树。底层 share/connection typed HTTP 可继续
服务管理集成，但不属于产品 argv。

已有自有 Gitea 来源通过 `kc create --url --credential-file` 连接。credential 只进入 typed
请求体和 Server 私有耐久账，不进入日志、trace 或公开结果。连接成功不等于 attach，也不发
`knowledge.read`。

知识消费只接受 `--repo` 或 `--dataset`；没有 Catalog 搜索范围，也不接受
`--catalog`、`--source`、`--pin`。Writer、`diff` 与 `schema list` 拒绝 `--dataset`。
