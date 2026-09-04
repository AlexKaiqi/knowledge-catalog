# 在 access-handle-published 上：knowledge access 调墙外 runtime；未配置则 CAPABILITY_UNSATISFIED。

Feature: probe access without runtime

  Scenario: wall runtime is not configured
    When I run `kc knowledge access --repo kr://scene/knowledge --object Service:orders --aspect health`
    Then error CAPABILITY_UNSATISFIED
