# 从 repository-attached 出发维护 merge gate；本用例不依赖提案、Dataset 或其它 probe。

Feature: probe gates

  Scenario: gate add list remove
    When I run `kc operations gate add --on merge --repo kr://scene/knowledge --require suite:scene-contract`
    Then the output has:
      | id  | nonempty |
      | on  | merge |
    When I run `kc operations gate remove --id $last.id`
    Then the output has:
      | revoked | nonempty |
    When I run `kc operations gate list`
    Then the output has:
      | rules | [] |
