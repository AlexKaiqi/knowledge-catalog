# 从 repository-attached 出发维护合并 Hook；审批由业务通过 writer.merge Hook 接入，本用例不依赖分支、Dataset 或其它 probe。

Feature: probe merge hooks

  Scenario: merge hook add list remove
    When I run `kc operations hook add --on writer.merge --phase post --repo kr://scene/object --url http://127.0.0.1:9/hooks`
    Then the output has:
      | id    | nonempty |
      | on    | writer.merge |
      | phase | post |
    When I run `kc operations hook remove --id $last.id`
    Then the output has:
      | revoked | nonempty |
    When I run `kc operations hook list --on writer.merge`
    Then the output has:
      | bindings | [] |
