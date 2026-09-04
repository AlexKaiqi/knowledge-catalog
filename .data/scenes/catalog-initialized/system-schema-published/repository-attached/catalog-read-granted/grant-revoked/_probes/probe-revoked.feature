# 在 grant-revoked 上：库存对 bot 不可见。

Feature: probe revoked

  Scenario: show after revoke
    When I run `kc catalog show --as bot`
    Then error FORBIDDEN
