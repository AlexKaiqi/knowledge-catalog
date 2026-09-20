# catalog-create-granted：creator 持有 catalog.repositories.create。
# 主体不用 bot：其它分叉会给 bot 叠加权。真正建仓不在本机空 Deployment 上做。

Feature: catalog-create-granted

  Scenario: construct
    When I run `kc grant add --principal creator --action catalog.repositories.create --catalog kr://scene/catalog`
    Then the output has:
      | id        | nonempty |
      | principal | creator |
      | catalog   | kr://scene/catalog |
      | actions.0 | catalog.repositories.create |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | creator |
      | rules[].catalog   | kr://scene/catalog |
      | rules[].actions.0 | catalog.repositories.create |
    When I run `kc grant list --principal creator --action catalog.repositories.create --catalog kr://scene/catalog`
    Then the output has:
      | allow  | true |
      | ruleId | nonempty |
