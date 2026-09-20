# 在 catalog-initialized 上：空规则不放行托管建仓准入。
# 不跑 kc create：产品 create 拒绝 --home，场景执行器会注入它。

Feature: probe ungranted create

  Scenario: authenticated without catalog.repositories.create is forbidden
    When I run `kc grant list --principal bot --action catalog.repositories.create --catalog kr://scene/catalog`
    Then error FORBIDDEN
