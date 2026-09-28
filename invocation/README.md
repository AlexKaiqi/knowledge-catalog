# invocation/

执行载体缝合包：一次已解析调用的载体（`Invocation`）、操作体类型（`Command`/`Handler`）及其
读取方法。CLI 前端与（CAP-ORG-01 B6 起）服务面各跑一次 `Invocation`；两个 transport 互不可
import，所以载体不能归任何一方所有。与 `internal/telemetry` 同类的共享应用层基础设施，但两
个顶层 transport 都依赖它，按耦合单元模型立顶层单元（CAP-ORG-01 B5a，§2.4 R3）。

## 边界

拥有：`Invocation` 载体与其 Flag/Require/TargetRef/RequireProposal/RequirePreview/
BindingScope/KnowledgeRef 读取器；`Command`（Stage + Run）与 `Handler` 类型；授权判定闸
（allow.json 模型 + `Authorize`/`AuthorizeCatalogInventory`/`AuthorizationFlags`，B5b 迁入）；
Catalog discovery 请求验证标记；任务 pin 上下文归一（`--pin`/`--dataset-file` → 内部坐标）。

不拥有：flag/JSON/观测词汇（`invoke`，保持纯）；派发器与 Stage 机器（cli 现持，B5c 迁入）；
hooks 包装、遥测织造、证据链（cli 现持，B5b 后续分层迁出）；业务规则（`knowledgeapp`）；
任何 transport 打开的 Home 的生命周期——它只持有被递进来的句柄。

## 最小用法

```go
cx := &invocation.Invocation{Command: name, Home: home, WS: opened, Flags: flags, Context: ctx}
result, err := cmd.Run(cx)
```

cli 经 `invoke_aliases.go` 的类型别名消费（`type invocation = invocation.Invocation`），143 处
动词体站点零改动。

## 依赖方向

可以依赖：`invoke`、`home`（只引用句柄类型，绝不 Open）、`kernel`、`knowledge`、
`knowledge/serving`、`snapshot`、`controlplane`、stdlib。
禁止 import：`cli`、`server`、`httpsurface`、`client`、`knowledgeapp`（denylist 已钉，
`TestForbiddenDependencies`）。

## 验证

- arch：`TestApplicationPackageBoundaries`（home 引用豁免：载体持句柄、不开句柄）、
  `TestForbiddenDependencies`、`TestEveryProductionPackageHasAnAllowedLayer`（app 层）；
- 棘轮：`scripts/coupling` 以独立单元入基线（B5a：26 单元，34.62%，-0.22pp）；
- 行为门：`make test`、`make test-contracts`（API-01：CLI 与 HTTP 同一批 typed executor）。
