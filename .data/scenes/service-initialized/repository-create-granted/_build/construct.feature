# repository-create-granted：creator 持有 repositories.create。
# 主体不用 bot：其它分叉会给 bot 叠加权。真正建仓不在本机空 Deployment 上做。

Feature: repository-create-granted

  Scenario: construct
    When I run `kc grant add --principal creator --action repositories.create`
    Then the output has:
      | id        | nonempty |
      | principal | creator |
      | catalog   | absent |
      | actions.0 | repositories.create |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | creator |
      | rules[].actions.0 | repositories.create |
    Then the output has:
      | rules.0.catalog   | absent |
    When I run `kc grant list --principal creator --action repositories.create`
    Then the output has:
      | allow  | true |
      | ruleId | nonempty |
