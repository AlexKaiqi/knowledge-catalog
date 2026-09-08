# cli/

KC Client、KC Server 与部署管理的 **transport**：公开 argv、typed HTTP handler、授权与遥测。读取部署配置、恢复权威与耐久状态在 [`home/`](../home/README.md)；HTTP method+pattern 闭集在 [`httpsurface/`](../httpsurface/README.md)。协议实现仍在各自包。

公开命令的操作语义（审查用）见 [`SURFACE.md`](SURFACE.md)。CLI 重构方案见 [`REFACTOR.md`](REFACTOR.md)，HTTP 重构方案见 [`HTTP_REFACTOR.md`](HTTP_REFACTOR.md)。路径权威仍是 `surface.go`；HTTP 路由分母是 `httpsurface.Patterns()`，与 mux 登记对账。

三张表互相不读，`deployment` 与 `serve` 读取显式 `--config`；业务 CLI 只经 Server：

| 表 | 文件 | 用途 |
|---|---|---|
| 分组 CLI | `surface.go` | 公开命令路径 → 内部操作名；产品命令经 `client/` 调 typed API |
| 应用操作 | `command.go` | Server 与测试接缝共用的内部 handler 表；键 = 公开路径把空格换成连字符 |
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

恢复独立 Catalog Git、耐久控制状态、预配置 Snapshot binding 和派生缓存见 `home/`。`serve` 不初始化；`catalog repo attach` 在服务端验证既有 authority 后原子登记。

`catalog repo create --catalog <id> --repo <id> --command-id <id>` 通过独立 typed API 申请平台托管仓。必须显式指定 Catalog，因此不依赖 `catalog.read` 来发现默认值。`catalog.repositories.create` 只准入创建操作；服务按部署声明的初始策略授予认证 principal 新仓范围的能力。客户端不提交 driver、目录、DSN、受益人或 grant。创建成功后可使用 `pack → writer commit → knowledge read --repo`；失败重试使用同一 command-id，不能用 create 接管既有仓。

普通使用者采用 `catalog repo create --name <显示名> [--store <允许池>] [--catalog <id>]`，不提供基础设施坐标或 command-id。此路径只发一次 typed POST，不依赖 Catalog discovery；Server 按用户名、Catalog、名称和 Store 生成耐久恢复身份。`catalog repo list --mine [--repo <id>]` 通过本人库存找回管理地址，每仓仍检查当前 `repository.metadata.read`。低层显式坐标形式继续服务协议自动化。

管理 API 为 `POST/GET /catalog/v1/repositories`、`GET /catalog/v1/repositories/{repository}`。创建 body 只接受 `name`、可选 `store` 与 `catalog`。普通创建与库存输出由 `ManagedRepositorySummary` 拥有，自动分配 command 留在服务内部；详情另含当前发布、说明与检索准备状态 `RepositoryReadiness`。显式协议坐标创建仍保留 `home.ManagedRepositoryResult` 的调用者 command 回执。`GET /repositories/{repository}` 只交付不带仓数据的网页外壳，所有内容仍经同源 typed API 认证授权；网页不会直接打开 Dolt 或 Gitea 存储。浏览器可使用配置的组织登录，也可使用部署明确启用的 local 测试身份。

`RepositoryReadiness` 分别报告 `publication/publishedCommit`、源说明 `profile`、可查询时的 `schemaCount` 与 `search/searchBasis/reason`。检索状态需要另有当前仓的 `projection.read`；只有固定发布版本通过 `index.CheckSearchProjectionAt` 的 READY、AccessDigest 与物理提供方校验，才返回 `READY`。已观察到的 `BUILDING/UPDATING` 表示处理中，`FAILED/RETIRED` 表示失败或停用，`NOT_READY` 表示尚无可用投影；都不改变已经发布的版本。`NOT_CONFIGURED` 表示没有索引实例，`NOT_AUTHORIZED` 只表示明确拒绝，提供方或授权存储异常为 `UNAVAILABLE` 并带错误原因。此只读状态不会触发查询或重建，也不代替具体查询的能力与权限检查。

### 应用操作

`verbs_*.go` 按 semantic action 登记内部操作。跨 Catalog / Knowledge / File
Gateway 复用的 Workspace 流程单独放 `workspace_*.go`；`workspace_consume.go`
只留 pin、Serving 与消费授权的共享上下文。

公开文件入口只有 Workspace File Gateway / `kcfs`。宿主 git checkout / sync
由 `catalog/worktree` 持有，不是产品 CLI。

`search_request.go` 把 flags 编成 `SearchRequest`；真正的 Workspace SEARCH 在
`workspace_search.go`。`allow.go` 是授权求值；命名知识集 SEARCH 要 `workspace.consume`
与 `knowledge.search`，正文走 `delivery/` 按仓 `knowledge.read` 屏蔽。访问证据不在这里。

`AllowFile.initialGrants` 保存 allocation → 请求 digest 的一次策略应用回执，与初始仓级规则原子持久化。普通 `grant remove` 保留这份回执，create retry 不会重建已撤销规则。策略可明确授予仓级 `writer.receipt.read`；receipt 授权从耐久 command entry 取得真实 Repository/ref，不信任调用者指定的仓。托管创建的过程回执不进入 Catalog Git 或 Writer command ledger。

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

`kc serve --config` 只恢复；配置缺失不生成新部署。`workspace overlay` 是客户端临时配方合成，不保存 Server Home overlay。
Agent 入口是作为 typed Client 的分组 `kc` CLI；文件读取走由 Workspace File
Gateway 支撑的宿主挂载目录。

## 认证 adapter 与客户端登录

设计边界见 [`docs/PERMISSIONS.md`](../docs/PERMISSIONS.md) 与 [`docs/DEPLOY_AUTH.md`](../docs/DEPLOY_AUTH.md)。公开配对发现为 `GET /identity/v1/auth`，身份查询为 `GET /identity/v1/whoami`；路径和 DTO 由 HTTP registry 与 `client/` 维护。

| Server 模式 | 业务凭证 | 拒绝 |
|---|---|---|
| `local` | `X-Kc-As` | `Authorization`、自报委托、空身份 |
| `taihu` / `gitea` | `Authorization`（Taihu 可经已验证网关头） | 自报 `X-Kc-As` / `X-Kc-On-Behalf-Of`、空身份 |

产品混装凭证返回 `FORBIDDEN`；缺少匹配凭证返回 `UNAUTHENTICATED`。进程内空 `HTTPServerOptions` / `HTTPHandler(home)` 是 local 测试接缝；正式部署必须显式选定模式。

Taihu 资源方配置使用 `KC_SERVICE_CLIENT_SECRET` 作 introspection 应用密钥，`KC_TAIHU_HMAC_SECRET` 作网关 hex 验签密钥；两者不是调用方身份。`--auth-hmac-secret` / `--service-client-secret` 可以覆盖环境值，但不得把真实秘密作为 argv 字面量传入。精确配置见 `auth_taihu.go`、`auth_service.go` 与帮助文本。

已验证人类身份的 `principal` 是 `identity.CanonicalUsername` 接受的用户名，例如 `kaiqidong`；Gitea numeric ID、Taihu subject 和认证 issuer 只进入内部可信绑定，不作为公开身份或授权键。Taihu 有 actor 的委托保留 `agent:<id>`，`onBehalfOf` 为用户名；无用户且只有 client 时保留 `service:<client_id>`。Gitea/Taihu 认证器构造 `HTTPIdentity.User`，Server 在授权前通过 `identity/` 核对 `stateDir/identities.json`。同名不同 subject/provider/issuer、同 subject 改名均拒绝，不通过重新登录接管权限。网关必须提供 HMAC 签名、用户名与稳定 subject；裸 `X-Tai-User` 和未签名身份头不再是认证方式。

新部署初始化身份账，旧部署显式 `deployment init` 才建立该账；已有初始化回执后的缺失或损坏只能恢复。旧 `gitea:<id>` / `taihu:<username>` 授权不在登录时自动搬迁。运营者停服核实可信绑定后执行 `kc deployment identity migrate --config deployment.yaml --file migration.json`；文件为 `{"legacyPrincipal":"gitea:42","user":{"username":"kaiqidong","provider":"gitea","issuer":"https://id.example","subject":"42"}}`。迁移只更名仍存在的规则，并与一次迁移回执原子保存；重放不恢复撤销、不改初始授权回执、不能改为另一身份。此入口不支持自动改名或替换已占用的可信绑定。

同一显式迁移在身份账中保留旧主体到已核验用户的归属别名。历史托管仓的 mine/get/原命令重试据此恢复新用户名的归属，原仓身份、后台地址、Request/digest、分配与首次发权回执不改写。尚未发放的首次规则可以写给新主体，但完成过的规则绝不重发。别名先于当前授权迁移原子落盘；中间崩溃不因归属查找而取得权限，重跑同一迁移完成后半段。纯 Taihu 网关模式同样必须配置可信 `authURL`；签名身份没有可信 issuer 时拒绝绑定。

`kc login --server <url>` 先发现服务模式，再进行配对；已签发 Bearer 可通过 `KC_AUTH_TOKEN` 提供。Taihu 浏览器登录和续期经部署固定的 Server token broker，不需要客户端持有应用秘密；Server 的公开登录参数见 `client.BrowserLoginConfig`，服务端配置可用 `KC_LOGIN_RESOURCE`、`KC_LOGIN_SCOPE`、`KC_LOGIN_APP_NAME` 补充应用身份。身份验证和本机持久化都成功后才报告已登录。

非秘密连接配置保存为 `$KC_CONFIG_DIR/client.json`（默认 `~/.config/kc`），会话位于该目录 `sessions/<Server摘要>/`，以原子替换和 0600 权限保存。地址优先级是 `--server`、`KC_SERVER_URL`、保存的客户端配置；旧单服务登录文件只作匹配 Server 的兼容读取。切换 Server 不发送旧服务凭证；请求不跟随携带凭证的 HTTP 重定向。CLI、文件网关和 DSH 使用同一会话，过期后按同一服务续期，注销不会修改任务 pin。local 身份来源对 Taihu Server 返回 `USAGE_INVALID`。

浏览器组织登录的无状态接口为 `POST /identity/v1/authorize`、`POST /identity/v1/authorize:poll` 和 `POST /identity/v1/token`。前两者只传送固定部署的 PAR 请求/状态；token 接口的严格输入由 `client.TokenRequest` 定义。它们不建立 Server Workspace session、不发权，不接受调用者指定上游、应用 ID 或应用秘密。

Resource Access 参考装配通过 `--resource-access-url` / `KC_RESOURCE_ACCESS_URL` 指向独立 `resource-access/v1` runtime。容器间使用服务网络地址；调用方容器的 `localhost` 不指向另一个服务。

## 首次使用与分享

用户登录后可以运行 `kc admission show` 查看部署明确提供的首次准入，再用 `kc admission request` 申请。对应 typed HTTP 为 `GET` / `POST /identity/v1/admission`；请求不接受其他用户名或自行选择的发权动作。只有当前可信人类或显式 local 测试本人可以申请，agent/service 不因已认证而被视作人类。每个用户与 Catalog 的决定一次耐久保存；响应 `APPLIED`、`ADMITTED` 或 `REPLAYED` 中的 `currentActions` 才表示当前仍存在的权限，重放不会恢复撤权。

仓维护者用 `kc catalog repo share list --repo <id>` 查看本仓分享及目前允许转授的动作；`share add --repo <id> --principal <username> --action <actions>` 显式选择消费动作；`share remove --repo <id> --id <share-id>` 撤销一份分享。对应 `/catalog/v1/repositories/{repository}/shares` 的 GET/POST 与 `.../shares/{share}` 的 DELETE。三者均需本仓 `repository.shares.manage`，但该动作不赋予全局 grant 管理能力。

Server 只接受仓创建或连接时冻结的 `ShareActions`，并检查分享人现在拥有每个动作的完整仓范围。限制在 Catalog、ref、object、aspect 或 Workspace 的 grant 不能扩大为整仓分享。接收者不会取得再次分享或管理权；移除只匹配本仓、该 share ID 且有分享来源的规则。典型临时组合需要显式选择 `workspace.resolve`、`workspace.consume`，知识读/搜索和可选文件视图分别按其窄动作授予。分享不等同于 Catalog 库存发现或外部 Resource Access。

已有自有 Gitea 来源使用 `catalog repo connect --catalog <id> --repo <id> --url <provider/owner/repo> --credential-file <path>`。Server 先要求 `catalog.repositories.connect`，再验证 deployment 的 `connections.allowedOrigins` 与已有 Snapshot，不初始化或写入外部仓。返回可查询的管理 URL 和初始 HEAD。`catalog repo connection show|check --repo <id>` 查询连接或只读验证；`catalog repo connection rotate --repo <id> --credential-file <path>` 验证同 authority 后原子轮换。管理同时要求仓级 `repository.connections.manage` 和原 owner。credential 只通过 typed 请求体传入私有 Server store，不进入通用 flags、日志、trace 或公开结果。过期凭证不阻止 Server 恢复管理接口。

`knowledge search --catalog <id> --query <text>` 是 Catalog 发现语法糖：先从 `catalog show` 找到显式配置的 `discoveryWorkspaceId`，普通 resolve 固定版本，再走同一 Knowledge SEARCH。显式 Catalog 发现查询不继承 ambient Workspace/任务 pin；另有显式 workspace、repo、pin 或 source 时仍沿该输入的消费合同。发现门槛只有当前 `catalog.read`，不要求命名知识集 consume 或逐仓 search；候选不会因缺正文读权消失，交付链仍屏蔽未获 `knowledge.read` 的正文。不自动纳入其他登记来源。
