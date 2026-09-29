# invocation/

执行载体缝合包：一次已解析调用的载体（`Invocation`）、操作体类型（`Command`/`Handler`）及其
读取方法。CLI 前端与（CAP-ORG-01 B6 起）服务面各跑一次 `Invocation`；两个 transport 互不可
import，所以载体不能归任何一方所有。与 `internal/telemetry` 同类的共享应用层基础设施，但两
个顶层 transport 都依赖它，按耦合单元模型立顶层单元（CAP-ORG-01 B5a，§2.4 R3）。

## 边界

拥有：`Invocation` 载体与其 Flag/Require/TargetRef/RequireProposal/RequirePreview/
BindingScope/KnowledgeRef 读取器；`Command`（Stage + Run）与 `Handler` 类型；授权判定闸
（allow.json 模型 + `Authorize`/`AuthorizeCatalogInventory`/`AuthorizationFlags`，B5b 迁入）；
Catalog discovery 请求验证标记；任务 pin 上下文归一（`--pin`/`--dataset-file` → 内部坐标）；
**动词操作体（B6 迁入）**：读/写/索引/资源/目录/观测/授权域的 `Verb*` 函数
（`verbs_read.go`/`verbs_write.go`/`verbs_index.go`/`verbs_resource.go`/`verbs_catalog.go`/
`stores_observe.go`/`object_log.go`/`schema_browse.go` 等）——cli 注册表经 `Run: inv.VerbX`
引用，HTTP typed handler 调同一批操作体（API-01）。

不拥有：flag/JSON/观测词汇（`invoke`，保持纯）；argv 词汇与产品输出（cli）；
路由闭集与 wire 形状（server）；业务规则本身（`knowledgeapp`，动词体消费它）；
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
`knowledge/serving`、`knowledgeapp`（B6 起：动词体消费用例核心）、`snapshot`、`controlplane`、
stdlib。
禁止 import：`cli`、`server`、`httpsurface`、`client`（denylist 已钉，
`TestForbiddenDependencies`）。

## 验证

- arch：`TestApplicationPackageBoundaries`（home 引用豁免：载体持句柄、不开句柄）、
  `TestForbiddenDependencies`、`TestEveryProductionPackageHasAnAllowedLayer`（app 层）；
- 棘轮：`scripts/coupling` 以独立单元入基线（B6：26 单元，38.62%；缝增容为设计代价，基线显式重写）；
- 行为门：`make test`、`make test-contracts`（API-01：CLI 与 HTTP 同一批 typed executor）。
