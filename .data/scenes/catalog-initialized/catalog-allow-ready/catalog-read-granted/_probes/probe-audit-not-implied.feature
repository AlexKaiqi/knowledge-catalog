# 在 catalog-read-granted 上：catalog.read 不放行登记表历史。

Feature: probe audit not implied

  Scenario: catalog.read does not imply catalog.audit.read
    When I run `kc catalog audit --as bot`
    Then error FORBIDDEN
