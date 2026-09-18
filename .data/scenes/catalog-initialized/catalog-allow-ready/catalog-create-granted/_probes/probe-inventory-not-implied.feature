# 在 catalog-create-granted 上：建仓准入不放行库存发现。

Feature: probe inventory not implied

  Scenario: catalog.repositories.create does not imply catalog.read
    When I run `kc show --as creator`
    Then error FORBIDDEN
    When I run `kc catalog list --as creator`
    Then error FORBIDDEN
