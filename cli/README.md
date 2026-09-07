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

### 应用操作

`verbs_*.go` 按 semantic action 登记内部操作。跨 Catalog / Knowledge / File
Gateway 复用的 Workspace 流程单独放 `workspace_*.go`；`workspace_consume.go`
只留 pin、Serving 与消费授权的共享上下文。

公开文件入口只有 Workspace File Gateway / `kcfs`。宿主 git checkout / sync
由 `catalog/worktree` 持有，不是产品 CLI。

`search_request.go` 把 flags 编成 `SearchRequest`；真正的 Workspace SEARCH 在
`workspace_search.go`。`allow.go` 是授权求值；命名知识集 SEARCH 要 `workspace.consume`
与 `knowledge.search`，正文走 `delivery/` 按仓 `knowledge.read` 屏蔽。访问证据不在这里。

`AllowFile.initialGrants` 保存 allocation → 请求 digest 的一次策略应用回执，与初始仓级规则原子持久化。普通 `grant remove` 保留这份回执，create retry 不会重建已撤销规则。托管仓创建策略不发 `writer.receipt.read`；该管理过程的回执不进入 Catalog Git 或 Writer command ledger。

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
