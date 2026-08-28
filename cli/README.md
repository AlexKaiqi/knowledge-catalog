# cli/

应用装配与 facade；协议实现仍在各自包。`surface.go` 只登记分组 CLI，`command.go` 只登记进程内 application operation；HTTP namespace 在 `service_routes.go` 显式登记，三者不共享命令表。

复杂但跨多个动词复用的 Workspace 流程单独放 `workspace_*.go`：search、checkout、sync/status、commit、inspect；`consume.go` 只保留 pin、Serving 与消费授权的共享上下文。

介质配置分为：

| 文件 | 负责 |
|---|---|
| `stores.go` | 配置契约、默认值和路径名 |
| `stores_file.go` | `layout.yaml` / `stores.yaml` 读写及旧格式迁移 |
| `stores_profile.go` | profile、driver、目录归一化和校验 |
| `stores_flags.go` | `store-set` flags / DSN 翻译 |
| `stores_public.go` | 不含密钥的公开状态视图 |

HTTP facade 同样按变化轴拆分：

| 文件 | 负责 |
|---|---|
| `serve.go` | 服务生命周期与认证后的并发读写边界 |
| `serve_auth.go` | Authenticator 端口、Gitea 身份验证与 admin 判定 |
| `service_routes.go` | 正式 namespace、typed DTO 与 handler；不解析 argv |
| `workspace_file_service.go` | 固定 pin 的只读 mount、单目录分页与 range read |
| `serve_response.go` | 统一错误信封 → HTTP status |

`kc serve` 不提供本机 Home/Store/authority attach，也没有 state/blob 工作台路由。Agent 入口是分组后的 `kc` CLI；文件读取走宿主已挂载目录。
