Feature: source repositories configured

  Scenario: prepare existing sources without 登记空间 admission
    Given existing repository kr://scene/knowledge with LiveAccess instance main
    Given existing repository kr://scene/graph
    When I run `kc show`
    Then the output has:
      | catalogId | absent |
      | repositories.1.id | absent |
    Then the output includes:
      | repositories[].id | kr://kc/system |
