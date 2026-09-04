# workspace-retired：scene-set 已退役。

Feature: workspace-retired

  Scenario: construct
    When I run `kc catalog workspace retire --workspace scene-set`
    Then the output has:
      | workspace | scene-set |
      | retired   | true |
    When I run `kc catalog workspace show --workspace scene-set`
    Then the output has:
      | workspaceId | scene-set |
      | retired     | true |
