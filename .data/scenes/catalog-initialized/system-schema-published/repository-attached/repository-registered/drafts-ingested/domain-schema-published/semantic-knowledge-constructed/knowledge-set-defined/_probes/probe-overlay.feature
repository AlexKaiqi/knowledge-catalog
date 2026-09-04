# 在 knowledge-set-defined 上：本机 overlay 盖在配方上，不发布 WorkspaceDefinition。

Feature: probe local overlay

  Scenario: overlay show and clear
    When I run `kc local workspace overlay --workspace scene-set`
    Then the output has:
      | workspaceId | scene-set |
    When I run `kc local workspace overlay --workspace scene-set --clear`
    Then the output has:
      | workspaceId | scene-set |
      | cleared     | true |
