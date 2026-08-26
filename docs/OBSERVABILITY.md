# 知识访问可观测性

范围：谁在什么时候访问了哪个固定版本的知识，以及规范 Agent 如何把多次访问和反馈关联成一条可审计 trace。认证提供方不在本协议内。

本文的 trace 是知识访问证据视图，不等于可采样的分布式诊断遥测。系统 metric、日志、distributed trace、健康检查与 SLO 见 [`SYSTEM_OBSERVABILITY.md`](SYSTEM_OBSERVABILITY.md)。

## 身份

每次调用携带一个已经由上游建立的身份上下文：

```json
{
  "principal": "agent:finance-analyst-v3",
  "onBehalfOf": "user:kaiqidong"
}
```

- `principal` 是实际执行调用的主体，必填；CLI 兼容入口仍是 `--as`。
- `onBehalfOf` 是该主体所代理的用户，可选。
- 用户直接访问时，`principal=user:...`，省略 `onBehalfOf`。
- KC 当前只校验字段形状并原样传播，不验证 token、委托签名或 delegation chain。共享服务后续可替换认证器，但不能改变这两个字段的含义。
- 当前授权仍按 `principal` 求值；`onBehalfOf` 是可信上游断言和审计事实，尚不参与授权交集计算。

HTTP 本地模式使用 `X-Kc-As` 与 `X-Kc-On-Behalf-Of`。认证模式由认证器注入 `principal`，`onBehalfOf` 暂按请求上下文透传。生产接入委托认证后，网关必须只接受经过验证的 `onBehalfOf`。

## 访问账

`read/list/search/resolve/provenance/log/describe-schema/stream/vfs-read` 等消费动作完成后，facade 追加 `.kc/access.jsonl`。成功、失败和拒绝都记录；成功返回的每条知识记录为：

```json
{
  "evidenceId": "ev_01...",
  "occurredAt": "...",
  "identity": {
    "principal": "agent:finance-analyst-v3",
    "onBehalfOf": "user:kaiqidong"
  },
  "action": "read",
  "requestId": "req-42",
  "workspace": "finance-board",
  "pinId": "...",
  "decision": "ALLOW",
  "result": "RESOLVED",
  "knowledge": [{
    "knowledgeRef": {
      "repository": "kr://acme/org/semantics",
      "commit": "<fixed commit>",
      "object": "Metric:gmv"
    },
    "address": {
      "objectId": "Metric:gmv",
      "aspectName": "definition"
    }
  }]
}
```

`evidenceId` 由 Recorder 生成，调用方不得提供。只有 event 完整写入并完成 `fsync` 后 Recorder 才把
它作为内部 delivery ack 返回给 facade；覆盖率对账使用 `evidenceId`，不能使用可能重复的
`requestId` 作为唯一键。

访问账是过程证据，不进成员 Repository，不是 provenance，也不改变 Canonical 知识。批量 checkout 除逐对象命中外，还记录 `repository + commit` 快照范围；不会把 mount 路径冒充成文件命中。记录失败会使成功的 facade 响应失败，避免把“已返回但没有访问账”伪装成合规成功。

查询：

```bash
kc access-log --trace-id trace-42
kc access-log --filter-principal agent:finance-analyst-v3
kc access-log --repo kr://acme/org/semantics --object Metric:gmv
```

查询访问账、trace 和 hitmap 复用 `audit` 权限。

## Agent trace 与反馈

规范 Agent 用 trace/span 关联同一任务中的 KC 调用：

```text
traceId trace-42
spanId span-read-1
parentSpanId span-search-1
```

当前 facade 使用 `--trace-id`、`--span-id`、`--parent-span-id` 及对应
`X-Kc-Trace-Id`、`X-Kc-Span-Id`、`X-Kc-Parent-Span-Id`；目标服务入口使用
`traceparent` / `tracestate`。`kc trace --trace-id trace-42` 按时间合并实际知识访问和反馈：

```bash
kc record-feedback --workspace finance-board --trace-id trace-42 \
  --as agent:finance-analyst-v3 --on-behalf-of user:kaiqidong \
  --outcome helpful --message "answer accepted"
```

`record-feedback` 只接受已经存在知识访问的 trace id，避免产生无法关联到任何访问的孤立反馈。

这里的“完整 trace”是知识系统边界内的完整调用证据：请求身份、关联 id、授权结果、固定知识版本、访问结果和显式反馈。它不保存模型隐式推理或 chain-of-thought。Agent 绕过 facade 直接读成员 Git、checkout 文件或索引介质时，KC 无法观察逐条访问；`kcfs` 的文件 reader 会在返回 bytes 时写同一类 `vfs-read` 文件访问证据。其它宿主投影必须走等价的 `observability.Recorder` 接缝。

## Hitmap

`kc hitmap` 从成功的 `ALLOW + RESOLVED` 访问事件实时派生，不单独维护权威计数。分组键是：

```text
repository + commit + object + Address
```

因此同一对象的两个版本是两条 hit，不会把“访问过 GMV”模糊成未版本化统计。结果包含次数、首次/最后访问时间，以及按 `principal`、`onBehalfOf` 的计数。删除 `.kc/access.jsonl` 后 hitmap 可重建为空；它从不反向影响授权、搜索排序或知识内容。
