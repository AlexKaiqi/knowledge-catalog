# dataset-resolve-granted：resolver 持有 dataset.resolve。
# 主体不用 bot：其它分叉会给 bot 叠加 file.read / knowledge.*，隔离探会假绿。

Feature: dataset-resolve-granted

  Scenario: construct
    When I run `kc grant add --principal resolver --action dataset.resolve --catalog kr://scene/catalog --dataset scene-set`
    Then the output has:
      | principal | resolver |
      | catalog   | kr://scene/catalog |
      | dataset | scene-set |
      | actions.0 | dataset.resolve |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | resolver |
      | rules[].dataset | scene-set |
      | rules[].actions.0 | dataset.resolve |
