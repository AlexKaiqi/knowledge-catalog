# knowledge-set-defined：项目知识集 scene-set 已定义（① pin，不是再写一遍知识）。

Feature: knowledge-set-defined

  Scenario: construct
    When I run `kc workspace define --workspace scene-set --revision 1 --source kr://scene/knowledge=refs/heads/main`
    Then the output has:
      | workspaceId | scene-set |
      | revision    | 1 |
    When I run `kc show`
    Then the output includes:
      | workspaces[].workspaceId | scene-set |
      | workspaces[].revision | 1 |
      | repositories[].id | kr://scene/knowledge |
