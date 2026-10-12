# knowledge/serving/

面向消费者的逻辑 Knowledge value serving。它骑在一次固定的 `reader.Serving`/Workspace pin 上；
exact READ 和 SEARCH 命中正文复用同一 hydrate：

```text
Snapshot unit
  └── 原值
Bound State Schema (origin in frontmatter)
  ├── 普通 READ / access → StateLookup → Aspect 观察值
  └── stream mode → CAPABILITY_UNSATISFIED（普通 READ 不吞 Stream）
```

`ReadResult.commit` 始终是声明所在的 Repository commit。每个动态单元另带
`observations[]`，其中同时保存 `declarationCommit/declarationDigest` 和独立的
`ObservationBasis(bindingGeneration/consistency/sourceRevision/watermark/observedAt)`；调用方不得把动态值说成被 Workspace pin 冻结。

本包只拥有 `StateLookup` 端口与编排，不实现凭证、缓存、限流、运行 generation 或源协议。`resource-access/v1` 原点来自固定 commit 上 Domain Schema Canonical frontmatter 的 `origin`（ResourceDescriptor 操作用来自描述的 `origin`）。参考 HTTP 装配按该原点 `POST {origin}/v1/access`，坐标是实体 `object_id`。Schema 未声明 origin 时，Bound State 明确返回 `CAPABILITY_UNSATISFIED`。出站调用由应用层转发已经通过 Server 认证边界的调用方证明（`Authorization`、Taihu `X-Tai-Identity`）和已验证 principal；本包不持有 token。local 配对没有 token，只带 principal。凭证不进 Schema 或 observation envelope。

HTTP runtime 接收 `POST /v1/access`。State 普通 READ 优先使用声明中的
`lookup` operation，并兼容 `read`；Schema 未声明 operations 时默认 `lookup`。请求携带 pinned Binding、
`schemaRef`、principal/onBehalfOf、request/trace 关联信息，以及调用方认证头（Taihu 为 `Authorization` 与/或 `X-Tai-Identity`）。成功响应必须是：

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

runtime 只返回任意 JSON body、缺少 `value` 或给不出合法 basis 都会失败关闭。

观察保留与失败关闭（resource-access.md §恢复与观察保留 / U2 / U4）：latest-only 来源的平台
保留记录是历史重读的唯一依据——保留期内重读返回**原观察**（原 basis，不是最新值）；记录丢失
是恢复故障（`TEMPORARY_UNAVAILABLE` 类），正常到期是生命周期结束（`CAPABILITY_UNSATISFIED` 类），
两者必须可区分且都明确失败，不得以最新值或空历史掩盖。业务空值带合法 basis 是成功观察；
同样的空但缺合法 basis 是占位，必须失败关闭。State 取值失败只传播错误，不降级为声明扫描、
不制造 MISSING、不返回占位空值。

`schema/*` 冻结 Entity/Aspect、pattern、字段类型及 required/additionalProperties 值约束，
逻辑 Serving 按固定 Schema 校验观察正文及 envelope。Schema 仍是平台 Schema 合同，
不承诺完整 JSON Schema。查询字段能力唯一从固定版本 Schema 的字段 access 编译；结构验证与查询能力分别处理。

VFS、checkout、Repository maintainer READ 继续读取固定 Snapshot/声明，不经过本包。

公开消费面不提供 Workspace LIST。维护扫描、显式 export 和宿主文件投影保持固定声明视图，不调用 runtime。

Workspace SEARCH 对纯 Snapshot 字段继续使用固定 Snapshot projection；涉及 State Binding 字段时，
`index` 控制链在同一声明 commit 上调用本包的 hydrate 语义，建立独立动态 projection。动态候选从
同 revision Serving State 回读，并在 `SearchView` 与 `KnowledgeVersion.observations` 携带 basis。

## 对象 Interface 与 LiveAccess

`OpenInterfaces` 在同一个固定 `reader.Serving` 上接受 `InterfaceRequest`，选择对象的
`interfaces` Member，并按合同坐标、合同摘要与实现 Schema 的结构摘要匹配 `InterfaceRegistry`。
注册字段 `ImplementationSchemaStructureDigest` 使用 `knowledge.SchemaStructureDigest`，只排除
字段 `access`；索引声明变化不要求重新安装处理器，其余归一化 Schema 语义仍参与匹配。
`InterfaceHandler` 独立注册，拥有声明校验与共同处理逻辑。操作输入及返回值使用合同中可选的
同仓 Schema；缺处理器、未支持版本或不存在的操作均明确失败。`InterfaceAuthorizer` 在
执行前生效，write 操作必须注入此端口；注册和声明不授予调用权。回执中的
`InterfaceExecutionBasis` 保留声明、合同、实现 Schema 与处理器版本依据，其中
`ImplementationSchemaDigest` 保留包含字段 `access` 的完整摘要，结构摘要单独记录支持匹配依据。
合同坐标仍匹配精确仓和 commit。本仓对象 ID 形式的 `interfaceRef` 随本次仓 pin 解释；
仓版本前进后部署需显式安装该合同依据。独立合同仓的固定跨仓引用可使合同采用版本与
对象、实现 Schema 的更新分开；辅助读取仍受本次 Workspace 的固定范围约束。

参考 `LiveAccessHandler` 使用已配置的 `ResourceOperationAccessor`，runtime/protocol/origin
和 operation→call 映射来自固定实现声明，输入保持业务 JSON。`read` 要求 `value` 与合法
`ObservationBasis`，回执的 observation 与知识 basis 分开；`invoke` 的源端结果原样返回。
`RegisterLiveAccess` 只安装与 system 出版物合同摘要、实现 Schema 结构摘要一致的版本；其它能力由
宿主安装自己的注册。旧 State Binding 与 ResourceDescriptor 的 resource-access/v1 wire
继续兼容；新请求另带实现 Member 地址与声明摘要，不把旧 wire 名称当成新知识类型。

Server 宿主通过 `HTTPServerOptions.ObjectInterfaces` 注入扩展注册表。
`WithLiveAccess` 在本次固定依据上补齐内置处理器，保留已安装扩展且不修改共享注册表。
消费者使用
`kc invoke --implementation <member-key> --operation <name> --input <json>`，客户端只传选择与
输入，不持有处理器。跨仓合同必须在本次 Workspace 范围中明确钉住，辅助读取不扩大范围。
