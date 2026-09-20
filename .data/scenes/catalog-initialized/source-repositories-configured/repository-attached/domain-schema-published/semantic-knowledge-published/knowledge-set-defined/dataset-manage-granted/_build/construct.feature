# dataset-manage-granted：manager 持有 scene-set 的 dataset.manage。
# 不在本节点退役 scene-set。主体不用 bot：使用具名主体明确本用例的授权边界。

Feature: dataset-manage-granted

  Scenario: construct
    When I run `kc grant add --principal manager --action dataset.manage --catalog kr://scene/catalog --dataset scene-set`
    Then the output has:
      | principal | manager |
      | catalog   | kr://scene/catalog |
      | dataset | scene-set |
      | actions.0 | dataset.manage |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | manager |
      | rules[].dataset | scene-set |
      | rules[].actions.0 | dataset.manage |
    When I run `kc grant list --principal manager --action dataset.manage --catalog kr://scene/catalog --dataset scene-set`
    Then the output has:
      | allow  | true |
      | ruleId | nonempty |
