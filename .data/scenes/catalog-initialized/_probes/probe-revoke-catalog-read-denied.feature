Feature: revoked Catalog grant is denied

  Scenario: grant and revoke Catalog read before the next request
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
    When I run `kc show --as revoke-probe`
    Then error FORBIDDEN
