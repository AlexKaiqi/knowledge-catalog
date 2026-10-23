# observation-refreshed：迁移后的参考说明

此文件保留原说明，不是状态、可执行 scene 或验证证据。实际 Oracle 由宿主 `_meta.yaml` 中的具名 Go 测试或独立 probe 承接。

# observation-refreshed

接入方 Observer 发出 source changed（notice 可丢、可乱序）。容器在 `repository-attached/_materials/accessor/observer.py`。平台按**固定 Binding** 向 Resource Access lookup，刷新可丢**动态索引**。Repository HEAD 不变。未成功观察的 Binding 不能进索引当 MISSING。

这不是声明式 Snapshot 投影（见 `projection-synced`）。两条车道共用显式索引声明编译出的 AccessSpec。

Hook 是出站，不能冒充这条入站通知。

构建与探：`TestProjectionControllerNoticePullsStateWithoutChangingSnapshot`、`TestChangeNoticeRejectsBody`、`TestProjectionNotifyPullsBoundStateWithoutChangingHEAD`。仓级公开通知入口 `operations projection notice` / `POST /operations/v1/projections:notice` 已退役；当前维护者入口是按 Dataset 修订显式刷新 `kc index prepare --dataset <id> --revision <N>`，来源内部通知与对账合同保留。

## 原参考 _build/construct.feature（已退役，仅存档）

```gherkin
# observation-refreshed：接入方通知 Bound State 变化；平台按固定 Binding 拉取。
# 未配置 resource-access/v1 runtime 时 notice 是 CAPABILITY_UNSATISFIED；本节点走 go-test，不在无 runtime 的 live 仓上当 construct。
# 原命令 kc operations projection notice 已随仓级公开入口退役，下面保存的是历史形状。

Feature: observation-refreshed

  Scenario: construct
    When I run `kc index prepare --dataset scene-set --revision 1`
    Then the output has:
      | dataset | scene-set |
      | ready   | true |
    When I run `kc writer head --repo kr://scene/object`
    Then the output has:
      | repository | kr://scene/object |
      | commit     | nonempty |
```

## 原参考 _probes/probe-head-unchanged.feature

```gherkin
Feature: observation-refreshed probe

  Scenario: notice does not move HEAD
    When I run `kc writer head --repo kr://scene/object`
    Then the output has:
      | repository | kr://scene/object |
      | commit     | nonempty |
```
