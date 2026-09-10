# 客户端合成临时配方；共享 Workspace 与 Catalog 不因个人布局改变。
Feature: client workspace overlay

  Scenario: compose a personal recipe without publishing it
    When I run `kc workspace overlay --file $materials/workspace-recipe.yaml --overlay $materials/workspace-overlay.yaml`
    Then the output has:
      | workspaceId | personal-task |
      | sources.0.repository | kr://scene/knowledge |
      | sources.0.path | references |
    When I run `kc show`
    Then the output includes:
      | workspaces[].workspaceId | scene-set |
      | workspaces[].revision | 1 |
      | repositories[].id | kr://scene/knowledge |
    When I run `kc workspace pin --workspace personal-task`
    Then error WORKSPACE_INVALID
