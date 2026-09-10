# 在 workspace-retired 上：命名知识集不再可用。

Feature: probe retired

  Scenario: retired workspace
    When I run `kc workspace pin --workspace scene-set`
    Then error WORKSPACE_INVALID
