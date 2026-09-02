# cli/

KC Client、KC Server 与宿主 bootstrap 的装配层；协议实现仍在各自包。这是唯一
Go 装配根，所以全部文件留在同一 package：公开符号、未导出的 `Home` /
`invocation` 和测试接缝必须能互相看见。组织靠文件前缀，不靠子目录。

三张表互相不读，只有 `kc local` 与 `kc serve` 可以打开 Home：

| 表 | 文件 | 用途 |
|---|---|---|
| 分组 CLI | `surface.go` | 公开命令路径 → 内部操作名；产品命令经 `client/` 调 typed API |
| 应用操作 | `command.go` | Server 与测试接缝共用的内部 handler 表 |
| HTTP namespace | `service_routes.go` | 显式登记 typed route；不解析 argv、不读 CLI 表 |

```text
surface.go  ──产品命令──►  remote_*.go  ──►  client/  ──►  HTTP
command.go  ──local/测试──►  verbs_*.go  ──►  Home / Writer / Reader / Index
service_routes.go  ──typed HTTP──►  同一组应用操作（不经 CLI parser）
```

## 文件按簇

按前缀读，不要按「每个动词一个文件」找。

### CLI 传输

`run.go` 入口；`parse.go` / `flags.go` / `flags_operand.go` / `flags_identity.go`
把 argv 收成 flags；`surface.go` 解析分组路径；`help.go` 是帮助文本。

### 远程 Client

`remote.go` 与 `remote_*.go` 把公开 CLI 编成 `client/` 调用。`remote_login.go`
只管理本机登录态；`remote_task_context.go` 是 DSH 任务坐标，不是服务端 Session。

### 宿主 Home

打开 `.kc`、选 adapter、挂 sidecar。`authority_drivers.go` 是唯一允许 import
具体 Dolt/Gitea 的文件。

| 文件 | 负责 |
|---|---|
| `home.go` | 装配活对象图：Store、Catalog、Writer、Reader、Index |
| `home_discover.go` | 扫磁盘布局；目录即真相，没有第二份清单 |
| `home_mount.go` | Repository attach 编排；不是 `kcfs mount` |
| `home_sidecar.go` | AfterSnapshot → Index、Merge Gate |
| `home_system.go` | 内置 `kr://kc/system` 发布与校验 |
| `home_audit.go` | `.kc/audit.jsonl` / `system.jsonl` 过程账 |
| `home_hookrun.go` | 动词 pre/post 出站 Hook |
| `stores.go` | 配置契约、默认值和路径名 |
| `stores_file.go` | `layout.yaml` / `stores.yaml` 读写及旧格式迁移 |
| `stores_profile.go` | profile、driver、目录归一化和校验 |
| `stores_flags.go` | `store-set` flags / DSN 翻译 |
| `stores_public.go` | 不含密钥的公开状态视图 |

### 应用操作

`verbs_*.go` 按 semantic action 登记内部操作。跨 Catalog / Knowledge / File
Gateway 复用的 Workspace 流程单独放 `workspace_*.go`；`workspace_consume.go`
只留 pin、Serving 与消费授权的共享上下文。

公开文件入口只有 Workspace File Gateway / `kcfs`。`workspace_checkout.go` /
`workspace_sync.go` 等宿主物化 handler 不是产品 CLI。

`search_request.go` 把 flags 编成 `SearchRequest`；真正的 Workspace SEARCH 在
`workspace_search.go`。`allow.go` 是授权求值；访问证据不在这里。

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

`kc serve` 不提供本机 Home/Store/authority attach，也没有 state/blob 工作台路由。
Agent 入口是作为 typed Client 的分组 `kc` CLI；文件读取走由 Workspace File
Gateway 支撑的宿主挂载目录。
