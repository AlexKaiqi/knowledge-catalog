# observation-refreshed：接入方通知 Bound State 变化；平台按固定 Binding 拉取。

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
