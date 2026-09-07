# 在 permissions-aspect-published 上：Aspect 不放行 kc 读。

Feature: probe not a gate

  Scenario: source grant is not allow
    When I run `kc knowledge read --as bob --repo kr://scene/knowledge --object Table:orders`
    Then error FORBIDDEN
