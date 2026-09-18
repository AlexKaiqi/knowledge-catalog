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
