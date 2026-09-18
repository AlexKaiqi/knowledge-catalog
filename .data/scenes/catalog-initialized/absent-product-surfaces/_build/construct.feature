Feature: absent-product-surfaces

  Scenario: construct
    When I run `kc show`
    Then the output has:
      | catalogId | kr://scene/catalog |
      | datasets | [] |
