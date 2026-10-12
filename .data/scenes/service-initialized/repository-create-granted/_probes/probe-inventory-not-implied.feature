# 在 repository-create-granted 上：建仓准入不放行库存发现。

Feature: probe inventory not implied

  Scenario: repositories.create does not imply inventory.read
    When I run `kc show --as creator`
    Then error FORBIDDEN
    When I run `kc show --as creator`
    Then error FORBIDDEN
