# cli/

KC Client、KC Server 与宿主 bootstrap 的装配层；协议实现仍在各自包。`surface.go` 只登记分组 CLI，公开业务命令经 `client/` 调 typed API；`command.go` 登记 Server application operation 和测试接缝。HTTP namespace 在 `service_routes.go` 显式登记，三者不共享命令表。只有 `kc local` 与 `kc serve` 可以打开 Home。

复杂但跨多个服务复用的 Workspace 流程单独放 `workspace_*.go`。公开文件入口只有 Workspace File Gateway / `kcfs`；checkout、sync/status 等宿主物化 handler 不是产品 CLI。`consume.go` 只保留 pin、Serving 与消费授权的共享上下文。

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
| `auth.go` | 可注册 Authenticator 合同与 factory registry |
| `serve_auth.go` | Authenticator 端口、Gitea 身份验证与 admin 判定 |
| `service_routes.go` | 正式 namespace、typed DTO 与 handler；不解析 argv |
| `workspace_file_service.go` | 固定 pin 的只读 mount、单目录分页与 range read |
| `serve_response.go` | 统一错误信封 → HTTP status |

`kc serve` 不提供本机 Home/Store/authority attach，也没有 state/blob 工作台路由。Agent 入口是作为 typed Client 的分组 `kc` CLI；文件读取走由 Workspace File Gateway 支撑的宿主挂载目录。
