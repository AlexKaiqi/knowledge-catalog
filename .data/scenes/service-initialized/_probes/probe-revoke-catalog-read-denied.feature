Feature: revoked 登记空间 grant is denied

  Scenario: grant and revoke 登记空间 read before the next request
    When I run `kc grant add --principal revoke-probe --action inventory.read`
    Then the output has:
      | principal | revoke-probe |
      | catalog   | absent |
      | actions.0 | inventory.read |
      | id        | nonempty |
    When I run `kc grant remove --id $last.id`
    Then the output has:
      | revoked | nonempty |
    When I run `kc show --as revoke-probe`
    Then error FORBIDDEN
    When I run `kc show --as revoke-probe`
    Then error FORBIDDEN
