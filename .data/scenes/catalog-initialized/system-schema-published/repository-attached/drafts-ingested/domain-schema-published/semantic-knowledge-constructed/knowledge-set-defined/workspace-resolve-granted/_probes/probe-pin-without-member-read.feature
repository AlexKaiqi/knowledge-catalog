# 在 workspace-resolve-granted 上：无成员 knowledge.read 时不能交出 pin 元数据。

Feature: probe pin without member read

  Scenario: resolve does not disclose members
    When I run `kc catalog workspace resolve --as bot --workspace scene-set`
    Then error FORBIDDEN
