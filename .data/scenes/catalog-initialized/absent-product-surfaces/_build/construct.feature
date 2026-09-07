Feature: absent-product-surfaces

  Scenario: construct
    When I run `kc catalog show`
    Then the output has:
      | catalogId | kr://scene/catalog |
      | workspaces | [] |
