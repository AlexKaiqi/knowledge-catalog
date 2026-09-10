# workspace-retired：scene-set 已退役。

Feature: workspace-retired

  Scenario: construct
    When I run `kc workspace retire --workspace scene-set`
    Then the output has:
      | workspace | scene-set |
      | retired   | true |
    When I run `kc show`
    Then the output includes:
      | workspaces[].workspaceId | scene-set |
      | workspaces[].retired     | true |
