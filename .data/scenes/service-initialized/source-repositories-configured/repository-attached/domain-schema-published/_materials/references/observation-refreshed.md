# observation-refreshed：迁移后的参考说明

此文件保留原说明，不是状态、可执行 scene 或验证证据。实际 Oracle 由宿主 `_meta.yaml` 中的具名 Go 测试或独立 probe 承接。

# observation-refreshed

接入方 Observer 发出 source changed（notice 可丢、可乱序）。容器在 `repository-attached/_materials/accessor/observer.py`。平台按**固定 Binding** 向 Resource Access lookup，刷新可丢**动态索引**。Repository HEAD 不变。未成功观察的 Binding 不能进索引当 MISSING。

这不是声明式 Snapshot 投影（见 `projection-synced`）。两条车道共用 Schema AccessHints 编译出的 AccessSpec。

Hook 是出站，不能冒充这条入站通知。

构建与探：`TestProjectionControllerNoticePullsStateWithoutChangingSnapshot`、`TestChangeNoticeRejectsBody`、`TestProjectionNotifyPullsBoundStateWithoutChangingHEAD`。公开入口是 `operations projection notice` / `POST /operations/v1/projections:notice`。

## 原参考 _build/construct.feature

```gherkin
# observation-refreshed：接入方通知 Bound State 变化；平台按固定 Binding 拉取。
# 未配置 resource-access/v1 runtime 时 notice 是 CAPABILITY_UNSATISFIED；本节点走 go-test，不在无 runtime 的 live 仓上当 construct。

Feature: observation-refreshed

  Scenario: construct
    When I run `kc operations projection notice --repo kr://scene/knowledge --object Service:orders --aspect health`
    Then the output has:
      | repository  | kr://scene/knowledge |
      | basisCommit | nonempty |
      | revision    | nonempty |
    When I run `kc writer head --repo kr://scene/knowledge`
    Then the output has:
      | repository | kr://scene/knowledge |
      | commit     | nonempty |
```

## 原参考 _probes/probe-head-unchanged.feature

```gherkin
Feature: observation-refreshed probe

  Scenario: notice does not move HEAD
    When I run `kc writer head --repo kr://scene/knowledge`
    Then the output has:
      | repository | kr://scene/knowledge |
      | commit     | nonempty |
```
