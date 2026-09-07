# workspace-consume-granted：bot 持有 scene-set 的 workspace.consume。

Feature: workspace-consume-granted

  Scenario: construct
    When I run `kc admin grant add --principal bot --action workspace.consume --catalog kr://scene/catalog --workspace scene-set`
    Then the output has:
      | principal | bot |
      | catalog   | kr://scene/catalog |
      | workspace | scene-set |
      | actions.0 | workspace.consume |
    When I run `kc admin grant list`
    Then the output includes:
      | rules[].principal | bot |
      | rules[].workspace | scene-set |
      | rules[].actions.0 | workspace.consume |
