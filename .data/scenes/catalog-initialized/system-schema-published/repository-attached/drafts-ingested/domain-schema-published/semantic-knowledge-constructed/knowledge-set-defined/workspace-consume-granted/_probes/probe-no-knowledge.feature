# 在 workspace-consume-granted 上：consume 不放行 knowledge.*。

Feature: probe no knowledge

  Scenario: consume is not search or read
    When I run `kc knowledge search --as bot --repo kr://scene/knowledge --query merchandise`
    Then error FORBIDDEN
    When I run `kc knowledge read --as bot --repo kr://scene/knowledge --object metric/gmv`
    Then error FORBIDDEN
