Feature: retired checkout is rejected

  Scenario: initialized 登记空间 rejects checkout
    When I run `kc show`
    Then the output has:
      | catalogId | absent |
      | datasets | [] |
    When I run `kc checkout`
    Then error USAGE_INVALID
