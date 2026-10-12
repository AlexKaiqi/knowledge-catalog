# 在 inventory-read-granted 上：inventory.read 不放行登记表历史。

Feature: probe audit not implied

  Scenario: inventory.read does not imply registry.audit.read
    When I run `kc registry audit --as bot`
    Then error FORBIDDEN
