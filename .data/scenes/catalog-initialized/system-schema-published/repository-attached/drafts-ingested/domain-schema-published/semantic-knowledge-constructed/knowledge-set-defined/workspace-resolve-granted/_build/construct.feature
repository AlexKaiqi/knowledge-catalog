# workspace-resolve-granted：bot 持有 workspace.resolve。

Feature: workspace-resolve-granted

  Scenario: construct
    When I run `kc admin grant add --principal bot --action workspace.resolve --catalog kr://scene/catalog --workspace scene-set`
    Then the output has:
      | principal | bot |
      | catalog   | kr://scene/catalog |
      | workspace | scene-set |
      | actions.0 | workspace.resolve |
    When I run `kc admin grant list`
    Then the output includes:
      | rules[].principal | bot |
      | rules[].workspace | scene-set |
      | rules[].actions.0 | workspace.resolve |
