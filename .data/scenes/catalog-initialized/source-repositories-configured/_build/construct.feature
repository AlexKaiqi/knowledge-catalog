Feature: source repositories configured

  Scenario: prepare existing sources without Catalog admission
    Given existing repository kr://scene/knowledge
    Given existing repository kr://scene/graph
    When I run `kc show`
    Then the output has:
      | catalogId | kr://scene/catalog |
      | repositories.1.id | absent |
    Then the output includes:
      | repositories[].id | kr://kc/system |
