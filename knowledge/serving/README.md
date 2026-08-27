# knowledge/serving/

面向消费者的逻辑 Knowledge value serving。它骑在一次固定的 `reader.Serving`/Workspace pin 上；
exact READ、Workspace LIST 和 SEARCH 命中正文复用同一 hydrate：

```text
Snapshot unit
  ├── value_source=snapshot → 原值
  ├── value_source=binding(state) → StateLookup → observation value
  └── value_source=binding(stream) → CAPABILITY_UNSATISFIED（普通 READ 不吞 Stream）
```

`ReadResult.commit` 始终是声明所在的 Repository commit。每个动态单元另带
`observations[]`，其中同时保存 `declarationCommit/declarationDigest` 和独立的
`ObservationBasis(bindingGeneration/consistency/sourceRevision/watermark/observedAt)`；调用方不得把动态值说成被 Workspace pin 冻结。

本包只拥有 `StateLookup` 端口与编排，不实现 endpoint 发现、凭证、缓存、限流、运行 generation 或源协议。具体 Materialization Runtime 在墙外，由应用装配时注入。`kc serve --resource-access-url <origin>`（或 `KC_RESOURCE_ACCESS_URL`）提供参考 HTTP 装配：Knowledge Server 与 runtime 可以是网络中的两个独立容器，不要求共享进程或文件系统。没有 runtime 时，Bound State READ 明确返回 `CAPABILITY_UNSATISFIED`，不会把仓内 `null` 占位当成知识结果。

HTTP runtime 接收 `POST /v1/access`。State Binding 的普通 READ 优先使用声明中的
`lookup` operation，并兼容 `read`；缺少两者时明确缺能力。请求携带 pinned Binding、选中的
runtime/protocol/call、`schemaRef`、principal/onBehalfOf 和 request/trace 关联信息。成功响应必须是：

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

当前 `schema/*` 冻结的是 Entity/Aspect、pattern 与 typed AccessHints，不是完整 JSON Schema（没有
required、additionalProperties 等约束），因此 Knowledge Server 只能把固定 `schemaRef` 交给
runtime，并校验 observation envelope，不能伪造一套正文结构校验。以后若 Schema 协议加入值约束，
校验应在本逻辑 Serving 边界执行，而不是由 VFS 或物理索引猜测。

VFS、checkout、Repository maintainer READ 继续读取固定 Snapshot/声明，不经过本包。

Workspace LIST 也经过相同逻辑 hydrate；checkout 和 VFS 则刻意绕过它，保持声明视图。

Workspace SEARCH 对纯 Snapshot 字段继续使用固定 Snapshot projection；涉及 State Binding 字段时，
`index` 控制链在同一声明 commit 上调用本包的 hydrate 语义，建立独立动态 projection。动态候选从
同 revision Serving State 回读，并在 `SearchView` 与 `KnowledgeVersion.observations` 携带 basis。
