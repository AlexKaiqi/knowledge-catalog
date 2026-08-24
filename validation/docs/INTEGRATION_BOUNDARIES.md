# 数仓集成边界

这里保存数仓场景对通用 Hook、Gate、权限和 Connector 契约的具体配置。协议定义仍以仓库根 `docs/` 和 Go 包为准。

## Hook

- `post commit` 通知索引、血缘或资产门户拉取新的 repository commit。
- `post merge` 通知语义层消费者重新解析 Workspace。
- Hook 只带指针，不携带整份表、指标或权限正文；失败不回滚已经成功的知识提交。

## Gate

语义口径发布可以要求：

```bash
kc gate-add --on merge --repo kr://acme/org/semantics \
  --require validate,suite:metrics-contract,suite:approval:steward
```

`metrics-contract` 和 `approval:steward` 是外部套件证据，不进入底座协议。证据必须绑定当前 Preview；候选内容变化后旧证据失效。

## 权限

- 知识仓权限由 `kc allow` 按 Repository / Workspace 和 `kc` 动词求值。
- Hive、Ranger 或查询引擎的表级权限是数据平面权限，不能放行 `kc read`。
- `permissions` Aspect 只是源系统某时点的 GRANT 快照；真正的 `SELECT` 仍由 Ranger、Unity 或内控系统强制。
- 物理层采集身份只能写物理知识仓；语义 steward 通过 proposal / preview / gate 发布口径。

## Connector

数仓客户端留在 scene：

```text
Hive / StarRocks structure  -> structure Scope
Ranger grants              -> permissions Scope
scheduler definitions      -> job definition Scope
runtime observations       -> APPEND stream
```

客户端负责感知源变化、拉取当前态、翻译 source key、持久化 checkpoint。通用 `connector/` 只执行 Address 级 `Plan -> Preview`；提交仍走 Writer 的 COMMIT / APPEND。
