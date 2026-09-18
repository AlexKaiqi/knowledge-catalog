# 在 access-handle-published 上：access 调 Schema origin；runtime 未监听则 TEMPORARY_UNAVAILABLE。

Feature: probe access without runtime

  Scenario: wall runtime is not configured
    When I run `kc access --repo kr://scene/knowledge --object Service:orders --aspect health`
    Then error TEMPORARY_UNAVAILABLE
