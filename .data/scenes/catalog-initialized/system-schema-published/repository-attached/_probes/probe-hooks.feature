# 在 repository-attached 上：出站 hook 是宿主配置，不发权、不改 Snapshot。

Feature: probe hooks

  Scenario: hook add list remove
    When I run `kc operations hook add --on workspace.manage --phase post --url http://127.0.0.1:9/hooks`
    Then the output has:
      | id    | nonempty |
      | on    | workspace.manage |
      | phase | post |
    When I run `kc operations hook remove --id $last.id`
    Then the output has:
      | revoked | nonempty |
    When I run `kc operations hook list`
    Then the output has:
      | bindings | [] |
