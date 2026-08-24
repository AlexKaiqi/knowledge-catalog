# ResourceDescriptor + Collector：DSH 多角色闭环

日期：2026-08-24

场景：Payment API 运行知识与生产 Trace

结果：**PASS**

## 用户真正如何使用

这个功能对业务用户只暴露两件事：

1. 把需要长期消费的说明和访问句柄写进 Knowledge Repository；
2. 配一个 Collector，把外部变化持续更新为知识。

ResourceDescriptor 是 Agent 看的访问句柄。integration repo 是业务与平台共建的运行代码仓。身份、授权和 trace 由 DSH 与运行平台统一提供，不要求每个业务重新设计。

| 角色 | 在 DSH 对话中做什么 | 不需要做什么 |
|---|---|---|
| 知识 Owner | 创建 Catalog/Repo/Workspace；发布 Runbook 与 ResourceDescriptor；给角色发权 | 不开发源客户端，不保存源凭证 |
| 集成开发者 | 用 `integration-development` Skill 在 integration Git Repo 编写 Collector、`status/window/lookup` 访问实现和测试，提交并推送 | 不直写知识 Git，不改 Catalog 协议 |
| Runtime Operator | 同步目标 ref，验证测试，预览 ChangeSet，激活固定 generation，启动平台 supervisor | 不手工代替计划任务提交知识 |
| 外部源 Owner | 只修改源系统当前态与 trace | 不调用 Writer，不碰知识仓 |
| 消费者 | 读 Workspace 中的 Runbook/状态；用 `resource` 工具按 Descriptor 查 live status/trace | 不知道 endpoint/token，不直接连源 |
| 审计者 | 查知识 provenance/log、调度 run 和 resource access trace | 不需要读取 live payload 全文 |

## 实际运行顺序

```text
Owner 发布 Runbook + ResourceDescriptor
  → Developer 在 integration repo 实现并 push
  → Operator sync / validate / preview / activate
  → 平台 schedule 首次采集，Writer COMMIT healthy / ops-r1
  → Consumer 读知识，并调用 status + lookup(trace-001)
  → Source Owner 只把外部源改成 degraded / ops-r2，加入 trace-002
  → 平台 schedule 自动检测变化，Writer COMMIT 更新
  → 新 Consumer 重新解析 Workspace，读到 ops-r2，并 lookup(trace-002)
  → Auditor 验证来源、两个知识 commit、两个自动 run 和四条访问 trace
```

## 可执行证据

验收由真实 headless DSH Agent 分别扮演七个角色完成。每个角色有独立 workspace/session，并留下 Agent trace。独立只读 oracle 的结果位于：

```text
/private/tmp/kc-resource-e2e.DI8sS1/oracle.json
```

关键断言：

- 首次计划任务 `run-c571130b994b8204`，`trigger=schedule`，新增 1 个知识单元，commit `66e7aed1c5a8c8f7d91e6605dee09f64c952ae0f`；
- 外部源修改后，计划任务 `run-2e3504d80b125039` 自动更新 1 个知识单元，commit `df56035e5e351b22948f463f53537fcb224a7deb`；
- 知识从 `healthy / payments-platform / ops-r1` 变为 `degraded / payments-sre / ops-r2`；
- 两个 Consumer session 共发起四次资源访问，分别在旧、新 Descriptor commit 上调用 `status` 与 `lookup`；
- 每条访问 trace 都包含 principal、DSH session、request ID、Descriptor object/repository/commit、runtime generation、输入/结果 digest 和耗时；
- DSH trace 证明 Owner/Operator/Auditor 使用 `kc`，Developer 使用 bundled `integration-development` Skill，Consumer 使用 bundled `knowledge-catalog` Skill 与 `resource` 工具；
- 外部源 Owner 没有调用 KC 写接口；第二次知识写入来自 schedule，不是人工 run。

## 复现

从 scene 根目录运行：

```bash
./validation/connectorhost/scripts/e2e-resource-roles.sh
```

脚本创建全新的临时 KC_HOME、integration bare Git Repo、开发 checkout、外部源、Integration Host 和七个 DSH 角色目录。最后运行离线 oracle；只有角色 marker、Agent tool trace、知识内容、来源、自动调度和访问 trace 全部匹配才通过。

## 实现边界

这是 scene 中的 Integration Host 参考实现，不是 `kc connector-run`，也没有给 Catalog 新增运行概念。通用协议根只保留：

- ResourceDescriptor 是知识文件；
- Collector 输出仍走既有 Writer COMMIT/APPEND；
- `connector.Preview` 只是 Address 对账 helper。

生产环境仍需把参考 Host 换成正式的多租户运行平台，补齐凭证保险库、网络隔离、构建签名、资源配额和高可用；Descriptor 与 Agent 访问方式无需因此变化。
