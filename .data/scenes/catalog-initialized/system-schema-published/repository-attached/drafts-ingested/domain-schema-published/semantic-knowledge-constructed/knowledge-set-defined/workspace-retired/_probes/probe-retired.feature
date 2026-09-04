# 在 workspace-retired 上：命名知识集不再可用。

Feature: probe retired

  Scenario: retired workspace
    When I run `kc knowledge read --workspace scene-set --object metric/gmv`
    Then error WORKSPACE_INVALID
