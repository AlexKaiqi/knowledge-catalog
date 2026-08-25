# cli/

应用装配与 facade；协议实现仍在各自包。`command.go` 是唯一命令表，`verbs_*.go` 按 write/read/index/catalog/control/home/allow/vfs 变化轴组织动词。

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
| `serve.go` | 服务生命周期、路由、认证后的并发读写边界 |
| `serve_auth.go` | Authenticator 端口、Gitea 身份验证与 admin 判定 |
| `serve_request.go` | JSON body → CLI flags；临时 ChangeSet 文件生命周期 |
| `serve_response.go` | 统一错误信封 → HTTP status |
| `serve_state.go` | 本机工作台 state/blob 只读内省 |
