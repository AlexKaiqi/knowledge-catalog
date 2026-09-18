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

当前 `schema/*` 冻结的是 Entity/Aspect、pattern 与 typed AccessHints，不是完整 JSON Schema（没有
required、additionalProperties 等约束），因此 Knowledge Server 只能把固定 `schemaRef` 交给
runtime，并校验 observation envelope，不能伪造一套正文结构校验。以后若 Schema 协议加入值约束，
校验应在本逻辑 Serving 边界执行，而不是由 VFS 或物理索引猜测。

VFS、checkout、Repository maintainer READ 继续读取固定 Snapshot/声明，不经过本包。

公开消费面不提供 Workspace LIST。维护扫描、显式 export 和宿主文件投影保持固定声明视图，不调用 runtime。

Workspace SEARCH 对纯 Snapshot 字段继续使用固定 Snapshot projection；涉及 State Binding 字段时，
`index` 控制链在同一声明 commit 上调用本包的 hydrate 语义，建立独立动态 projection。动态候选从
同 revision Serving State 回读，并在 `SearchView` 与 `KnowledgeVersion.observations` 携带 basis。
