# invocation/

服务执行载体与适配缝：一次已解析调用的 `Invocation`、`Command`/`Handler` 及其读取方法。
KC 应用的 typed handler 通过本包连接授权、固定范围、证据与所属任务服务；生产 CLI 经
各模块 SDK 请求 Server，不在本机运行 Invocation。CLI 的组件测试可在 `_test.go` 中直接
装配同一操作，嵌入测试路径不进入产品二进制。B5a/B6 的迁移记录保留原批次范围。

`InvokeApplicationWithTelemetryAtHome` 是 Server 的中央执行入口，缺少或空白主体在打开 Home、
授权与留证之前拒绝。仅本地组件装配使用 `ExecuteApplicationOperation` 的既有 owner 语义。

## 边界

拥有：`Invocation` 载体与其 Flag/Require/TargetRef/
BindingScope/ObjectRef 读取器；`Command`（Stage + Run）与 `Handler` 类型；授权判定闸
（allow.json 模型 + `Authorize`/`AuthorizeInventory`/`AuthorizationFlags`，B5b 迁入）；
Inventory discovery 请求验证标记；任务 pin 上下文归一（`--pin`/`--dataset-file` → 内部坐标）；
Dataset 修订固定（`ResolveAtRevision`：按已接受版本与定义生成 pin，再按发布记录复核，未发布修订
`VERSION_UNRESOLVED`）；Dataset 维度的 Schema 报告与分页（`DatasetSchemaReport`、`DatasetSchemaPage`，
字段用途取自本 Dataset 选入的索引声明，并给出成员检索就绪）；
**动词操作体（B6 迁入）**：读/写/索引/资源/目录/观测/授权域的 `Verb*` 函数
（`verbs_read.go`/`verbs_write.go`/`verbs_index.go`/`verbs_resource.go`/`verbs_dataset.go`/
`stores_observe.go`/`object_log.go`/`schema_browse.go` 等）——HTTP typed handler 调用这些适配；
根 CLI 的内部测试登记以 `Run: inv.VerbX` 复用同一操作（API-01）。

不拥有：flag/JSON/观测词汇（`invoke`，保持纯）；模块叶参数、帮助与产品结果（各模块 `cli`）；
全局 argv 与会话（根 `cli`/`client`）；路由闭集与 wire 形状（`server`）；
业务规则本身（`object/service`、`discovery/service`、`discovery/datasets`、
`liveaccess/dispatch`、`liveaccess/service`、`snapshot/service` 等所属能力）；
任何 transport 打开的 Home 的生命周期——它只持有被递进来的句柄。

命名 Dataset 的对象读取与搜索也接受完整成员的文件消费权限：独立的 `dataset.resolve` 加上
已接受修订中每个 Repository、固定 Commit 上的 `file.read`，须在当前策略下同时成立。
一次调用复用同一份已接受坐标，完整性检查、来源过滤和正文交付重新求值当前权限；坐标缓存
不保存授权结果。缺失成员、其他 Dataset 或临时定义不能借用该范围，直接 Repository 对象访问
仍按其自身权限判定。
跨仓组合没有单一来源仓，搜索交付和对象读取过滤共用来源向量校验：每个来源须属于同一
固定 pin，其方面完整对应返回内容，再按当前完整消费权限交付正文与观察依据。

## 最小用法

```go
cx := &invocation.Invocation{Command: name, Home: home, WS: opened, Flags: flags, Context: ctx}
result, err := cmd.Run(cx)
```

消费方直接使用本包公开类型和函数；根 CLI 不保留私有类型转发层。

## 依赖方向

可以依赖：`invoke`、`home`（只引用句柄类型，绝不 Open）、`kernel`、`object`、
`discovery`、`interfaces`、`liveaccess`、`snapshot` 的所属公开合同与任务服务，
以及 `controlplane` 和标准库；具体闭集由架构守卫维护。
禁止 import：`cli`、`server`、`httpsurface`、`client`（denylist 已钉，
`TestForbiddenDependencies`）。

## 验证

对象 Interface 的通用消费由 `InvokeObjectInterface` 连接固定 Reader 与装配的注册表，
生产 CLI 经 typed HTTP 使用此路径。`ObserveObjectInterfaceCall` 将执行回执映射到既有对象访问
记录，保留实现 Member、合同及实现 Schema 固定引用；LiveAccess 读取另保留独立观察。
审计记录保存合同/声明/Schema 摘要及处理器版本，不从任意业务结果里猜测这些身份。

- arch：`TestApplicationPackageBoundaries`（home 引用豁免：载体持句柄、不开句柄）、
  `TestForbiddenDependencies`、`TestEveryProductionPackageHasAnAllowedLayer`（app 层）；
- 棘轮：`scripts/coupling` 以独立单元入基线（B6：26 单元，38.62%；缝增容为设计代价，基线显式重写）；
- 行为门：`make test`、`make test-contracts`（API-01：CLI 与 HTTP 同一批 typed executor）。
