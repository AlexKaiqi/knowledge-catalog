Feature: audit grant does not reveal inventory

  Scenario: grant audit access then reject inventory discovery
    When I run `kc grant add --principal auditor --action catalog.audit.read --catalog kr://scene/catalog`
    Then the output has:
      | id        | nonempty |
      | principal | auditor |
      | catalog   | kr://scene/catalog |
      | actions.0 | catalog.audit.read |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | auditor |
      | rules[].catalog   | kr://scene/catalog |
      | rules[].actions.0 | catalog.audit.read |
    When I run `kc catalog audit --as auditor`
    Then the output has:
      | source    | catalog |
      | catalogId | kr://scene/catalog |
      | entries   | nonempty |
    When I run `kc show --as auditor`
    Then error FORBIDDEN
    When I run `kc catalog list --as auditor`
    Then error FORBIDDEN
