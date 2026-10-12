# inventory-read-granted：bot 持有该 登记空间 的 inventory.read。
# 本机 Home 无 Deployment，已认证主体仍要 grant 才能看见库存。

Feature: inventory-read-granted

  Scenario: construct
    When I run `kc grant add --principal bot --action inventory.read`
    Then the output has:
      | id        | nonempty |
      | principal | bot |
      | catalog   | absent |
      | actions.0 | inventory.read |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | bot |
      | rules[].actions.0 | inventory.read |
    Then the output has:
      | rules.0.catalog   | absent |
    When I run `kc show --as bot`
    Then the output has:
      | catalogId | absent |
    Then the output includes:
      | repositories[].id | kr://kc/system |
