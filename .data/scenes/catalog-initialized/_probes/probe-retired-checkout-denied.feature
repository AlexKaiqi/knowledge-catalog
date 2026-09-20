Feature: retired checkout is rejected

  Scenario: initialized Catalog rejects checkout
    When I run `kc show`
    Then the output has:
      | catalogId | kr://scene/catalog |
      | datasets | [] |
    When I run `kc checkout`
    Then error USAGE_INVALID
