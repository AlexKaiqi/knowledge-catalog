# workspace-defined：普通知识已发表后定义命名知识集 scene-notes。

Feature: workspace-defined

  Scenario: construct
    When I run `kc workspace define --workspace scene-notes --revision 1 --source kr://scene/knowledge=refs/heads/main`
    Then the output has:
      | workspaceId | scene-notes |
      | revision    | 1 |
    When I run `kc show`
    Then the output includes:
      | workspaces[].workspaceId | scene-notes |
      | workspaces[].revision | 1 |
      | repositories[].id | kr://scene/knowledge |
