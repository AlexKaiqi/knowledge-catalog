# 在 grant-revoked 上：被收回 catalog.read 的主体看不见库存。

Feature: probe revoked

  Scenario: show after revoke
    When I run `kc show --as revoke-probe`
    Then error FORBIDDEN
