# LiveAccess

LiveAccess 采用固定对象声明，为对象读取取得外部值。声明解释、取值与认证适用政策归根包；
`service/` 负责值采用和逐值失败，`plugin/` 提供实际入口，`evidence/` 保留原结果，
`discovery/` 向索引提供真实值与原观察。CLI 定义、帮助、参数、结果及专属适配归 `cli/`，
HTTP SDK 归 `client/`。Object Reader 只读固定声明，宿主负责公共装配。

## 采用与读取

Repo 原生 metadata 的 `kc.plugins` 选择 `liveaccess` 实例。Server 启动时固定配置，
Dataset 沿每个来源 Repo 的选择组合；没有选中贡献时，外部值明确失败，不调用旧全局实现。
配置、登记和生命周期合同见 [插件入口](plugin/README.md)与[消费服务](service/README.md)。

`ResolvedBinding`、`ResolveRepoBinding`、`ResolveBinding`、`ResolveBindingAt`、`ResolveDeclaration`
是唯一一条解释路径：binding 单元的 `descriptorRef` 经 `DescriptorScope`（`RepositoryDescriptors` /
`ServingDescriptors`）在请求范围内按 ID 解析并固定版本，origin 与 mode 取自 ResourceDescriptor，
声明摘要组合单元与取值描述两个版本；范围内同一 ID 多处持有为 `OBJECT_ID_CONFLICT`。
声明解释不调用来源，不重新读取兄弟单元或改用 HEAD。

业务 `read` 的 Repo/Dataset 入口均采用[逻辑读取](service/README.md)：静态值保留，
成功外部值在原位置呈现，合法 null 保持 null；失败只在该位置呈现原因，另有
`ObservedValue.Failures` 区分业务值与失败。成功值通过固定 Schema 和原观察依据核验，
失败没有成功观察，不以旧值、句柄或空值掩盖。指定深层位置先取选中单位，再选实际值中的 leaf。
声明 commit 保持固定，每个动态单位另带 `UnitObservation`，不能把它说成 Snapshot 字节。
维护扫描、导出、VFS 与 checkout 使用固定声明视图。

## resource-access/v1

`StateLookup.LookupState(ctx, StateLookupRequest) → (StateObservation, error)`。
请求包含固定 `Binding`、`SchemaRef`、`Origin`、已认证 `Identity`、`Trace`、`RequestID`。
默认 HTTP adapter 向声明的 `{origin}/v1/access` 发送 POST，普通读取优先 `lookup`，
兼容 `read`，未声明 operations 时使用 `lookup`。成功响应必须同时有 `value` 和合法 `basis`：

```json
{
  "value": {"status": "healthy"},
  "basis": {
    "bindingGeneration": "health-runtime-v2",
    "consistency": "bounded",
    "sourceRevision": "health-88",
    "observedAt": "2026-08-27T09:00:00Z"
  }
}
```

`basis` 对应 `object.ObservationBasis`；缺值、非法依据或违反固定值 Schema 都是取值失败。
State 与 Stream 沿已有声明区分，普通读取不将 Stream 隐式数组化。

认证证明来自本次 Server 已验证调用，`HTTPStateLookupOptions.CredentialOrigins` / 部署配置
`liveaccess.credentialOrigins` 指定其可信适用 HTTP origin。声明不能扩充这个范围；
携带证明访问范围外目标会在出站前拒绝，跨 origin 重定向也拒绝。
证明只进入认证头，主体、request/trace 保留关联；源返回值、basis 和错误不得夹带证明。
后台维护使用显式服务身份，不借用消费者请求。凭证不存入对象、日志或审计。

## 原结果审计与索引

消费请求先可靠保存实际采用的外部值或失败、固定声明、原观察及请求关联，再交付。
过程账只保存结果引用；普通读取不附完整证据。经既有独立授权审计入口按
`filter-request-id`（CLI）或 `requestId`（AuditQueryRequest）定位原请求，源更新、重启或离线
仍使用保留的原结果。Snapshot 沿固定版本/位置复核。保留期内丢失返回
`TEMPORARY_UNAVAILABLE`，正常到期返回 `CAPABILITY_UNSATISFIED`；不重取最新值补证。
介质及完整性合同见 [evidence](evidence/README.md)。

带 LiveAccess 声明的 Member 沿已有值 Schema/access 进入索引；维护使用严格取值，
任何失败都阻止该观察发布。查询按固定 StateRevision 匹配，并从该观察读取命中正文，
来源变化后仍返回原值和原 basis，不索引句柄、失败说明或伪造的空值。

## 已声明操作与说明

已有 `binding show`、`access`、`invoke` 保持公开任务与 wire。LiveAccess 实现由所选 Repo
实例匹配固定合同及实现 Schema；其他合同没有已安装插件处理，调用明确拒绝。
处理器合同与调用任务是本插件的内部机制，见 [handler](handler/README.md) 与 [dispatch](dispatch/README.md)。

`kc plugin liveaccess help` 返回 [CLI 同源说明](cli/README.md)，不读取仓配置或调用来源。
说明不是当前目标已启用或来源可用的证明。
