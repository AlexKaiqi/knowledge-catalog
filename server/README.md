# server/

服务接入面：method+pattern 路由闭集、typed handler 注册、授权强制与遥测的宿主。
CAP-ORG-01 B4 先立契约（`routes.go` 与 `httpsurface.Patterns()` 双向对账）；B5–B7 把 cli 的
serve/service_routes/handlers/授权/遥测/就绪探测搬进来，CLI 与 HTTP 继续运行同一批 typed
application executor（API-01）。

## 边界

拥有：HTTP 路由闭集（`routes.go`）、typed handler 与授权强制（B5–B6 迁入）、遥测注入（B6）、
就绪探测（B7，经装配缝读端口，不 import 具体 provider）。

不拥有：业务规则与执行器（`knowledgeapp`）、装配与 provider 选择（`home`）、argv 词汇与
产品输出（`cli` / `invoke`）、wire 协议语义（`client`）、声明本身（Knowledge ②）。

## 最小用法

```go
for _, r := range server.Routes() {
	_ = r // 闭集只读；handler 注册由本包内部装配完成（B5–B7 迁入）
}
```

## 依赖方向

可以依赖（目标态，随搬移逐批成立）：`knowledgeapp`、`home`、`delivery`、`gate`、`hook`、
`identity`、`observability`、`controlplane`、`invoke`（§3.3）。
禁止 import：`kc/cli`、`kc/client`（B6 落 denylist；前端调用一律经 typed client）。

## 验证

- `routes_anchor_test.go`：与 `httpsurface.Patterns()` 双向对账 + 无重复 + 字面表有序；
- arch：`TestEveryProductionPackageHasAnAllowedLayer`（app 层注册）、
  `TestEveryTopLevelCapabilityHasAReadme`；
- `make check-surface`、`make test-contracts`、`make test`（B5 起承载行为回归）。
