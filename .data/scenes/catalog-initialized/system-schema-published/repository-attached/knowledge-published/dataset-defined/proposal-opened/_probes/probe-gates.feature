# 在 proposal-opened 上：gate 是 merge 证据清单，不是 hook。

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
