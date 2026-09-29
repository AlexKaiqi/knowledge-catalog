# server/

服务接入面：method+pattern 路由闭集、typed handler 注册、授权强制与遥测的宿主。
CAP-ORG-01 B4 先立契约（`routes.go` 与 `httpsurface.Patterns()` 双向对账）；B6 把 cli 的
serve/service_routes/handlers/授权/遥测/就绪探测搬入（B6/B7）。CLI 与 HTTP 继续运行同一批
typed application executor（API-01）——动词操作体在 `invocation` 缝（见该包 README），
本包负责 transport 形状：路由、wire DTO、授权强制、遥测注入。

公开入口：`HTTPHandler`/`HTTPHandlerWithTelemetry`（handler 装配）、
`HTTPHandlerFromConfig`/`HTTPServerOptionsFromFlags`（配置驱动）。cli 的 serve verb 以
组合根身份调用它们（`cli → server` 合法边；cmd/kc 只 import cli）。

## 边界

拥有：HTTP 路由闭集（`routes.go`）、typed handler 与授权强制（B6 已迁入）、遥测注入（B6
已迁入，`serve_facade.go`/`serve_telemetry`）、gitea/配对身份（B6 已迁入）、
就绪探测（B7 已迁入：`server/internal/readiness` 持探测/缓存/指纹与证据写探针；search 后端经 `Providers.SearchBackend` 端口注入——探测函数由装配根 `home` 的 `NewSearchBackendProbe` 绑定具体 adapter，本包与 readiness 包都不 import 具体 provider，A-01 守卫 `TestProviderAdaptersImportedOnlyAtAssembly` 钉住）。

不拥有：业务规则与执行器（`knowledgeapp`）、装配与 provider 选择（`home`）、argv 词汇与
产品输出（`cli` / `invoke`）、wire 协议语义（`client`）、声明本身（Knowledge ②）。

## 最小用法

```go
for _, r := range server.Routes() {
	_ = r // 闭集只读；handler 注册由本包内部装配完成（B5–B7 迁入）
}
```

## 依赖方向

可以依赖：`knowledgeapp`、`home`（B6 起：transport 打开 Home 装配执行载体）、`invocation`、
`delivery`、`gate`、`hook`、`identity`、`observability`、`controlplane`、`invoke`（§3.3）。
禁止 import：`kc/cli`（B6 已落 denylist，`TestForbiddenDependencies`）。`kc/client` 暂允许，
仅作为 B6 前既有 wire-shape 家（sharing/connection 负载：本地面填充 typed client 解码的同一
批 JSON 形状）；该归属必须在 B7 重新裁决（httpsurface 升级为共享 DTO 家或 server 自持形状），
在此之前不得新增形状进 `client`。

## 验证

- `routes_anchor_test.go`：与 `httpsurface.Patterns()` 双向对账 + 无重复 + 字面表有序；
- arch：`TestEveryProductionPackageHasAnAllowedLayer`（app 层注册）、
  `TestEveryTopLevelCapabilityHasAReadme`；
- `make check-surface`、`make test-contracts`、`make test`（B5 起承载行为回归）。
