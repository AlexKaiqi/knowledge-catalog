# invoke/

transport 无关的调用词汇表与调用观察——flag 读取读法、规范 JSON 信封、Stage 阶段词汇、BindingScope
选择三元组与调用观察口，由 CLI 前端（`cli`）与 HTTP 服务面共享；QUALITY-05 S3' 落地，CAP-ORG-01 R1 收窄。

## 边界

拥有：

- flag 词汇表：`FlagValue` 与读取器（`FlagString` / `FlagBool` / `FlagStrings` / `RequireFlag` / `RequireDataset` / `DatasetOf`）；
- 规范输出与信封：`JSONOut`（唯一 JSON 规范化输出）、`WriteJSON` / `WriteInvoke` / `DecodeJSONBody`（标准库 `net/http` 信封读写，不含任何 kc transport 语义）；
- 执行词汇：`RunResult`、`Stage` 三档（Home / Open / Governed）、`BindingScope`（+`Matches`）；
- 调用观察：`ObservationTelemetry`（与 `NoObservationTelemetry`）及 `AuthorizationObserver` / `EvidenceTelemetryObserver` / `ProjectionBacklogObserver` 观察口。

不拥有：工作区载体 `Invocation` / `Command` / `Handler`（由 `invocation/` 持有，见
`invocation/invocation.go`；它携带 transport 打开的 `*home.Home`，不负责打开或关闭）、
argv 闭集与帮助文案（`cli/surface.go`）、HTTP route
注册与授权强制（拆分后 `server`）、业务规则与授权判定（`objectapp` / `object/writer`）、wire 协议
语义（`client`）。本包读 flag 值、不解析 argv；写 JSON 信封、不注册 route、不打开 Home。

## 最小用法

```go
flags := map[string]invoke.FlagValue{"dataset": "org/ds"}
ds, err := invoke.RequireDataset(flags) // 缺 flag 时返回 usage error
out := invoke.JSONOut(map[string]any{"dataset": ds, "ok": err == nil})
```

## 依赖方向

可以依赖（现值）：`kernel`、`hook`、`internal/telemetry` 与标准库（含 `net/http`
信封原语）。
禁止 import：`kc/cli`、`kc/httpsurface`、`kc/client`、`kc/home`——transport 中立性即本能力的存在意义
（R1 起四者全部可钉，B2 落 denylist 钉死）。

## 验证

- arch guard：`internal/arch` denylist `invoke ↛ {cli, httpsurface, client, home}`（B2 落地）；
  `TestApplicationPackageBoundaries`（R1 起绿）；
- coupling unit：`invoke`（R1 收紧：instability 0.88→0.80，propagation 38.22%→37.86%）；
- 行为：`invoke/http_test.go`；CLI 与 HTTP 共用 typed executor 的 `API-01` 合同
  （`TestCLIAndHTTPUseSameTypedApplicationExecutor`）。
