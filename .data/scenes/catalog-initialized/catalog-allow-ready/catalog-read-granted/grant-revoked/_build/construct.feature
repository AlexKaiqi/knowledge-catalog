# grant-revoked：revoke-probe 的 catalog.read 已收回。
# 不要假设 id 是 alw_1，也不要把整份 allow 清空：共享 Catalog 上还有 bootstrap 和管理规则。
# 空 allow 父态上 show --as revoke-probe 是 FORBIDDEN；公开 Catalog 现场仍可能列出 id。

Feature: grant-revoked

  Scenario: construct
    When I run `kc grant add --principal revoke-probe --action catalog.read --catalog kr://scene/catalog`
    Then the output has:
      | principal | revoke-probe |
      | catalog   | kr://scene/catalog |
      | actions.0 | catalog.read |
      | id        | nonempty |
    When I run `kc grant remove --id $last.id`
    Then the output has:
      | revoked | nonempty |
    When I run `kc show --as revoke-probe`
    Then error FORBIDDEN
