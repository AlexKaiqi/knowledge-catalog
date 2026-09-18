# 在 catalog-allow-ready 上：本机 Home 无 Deployment，已认证无 grant 不能看见库存。

Feature: probe ungranted catalog

  Scenario: authenticated without catalog.read is forbidden
    When I run `kc show --as bot`
    Then error FORBIDDEN
    When I run `kc catalog list --as bot`
    Then error FORBIDDEN
    When I run `kc catalog use kr://scene/catalog --as bot`
    Then error FORBIDDEN
    When I run `kc grant list`
    Then the output has:
      | rules | [] |
