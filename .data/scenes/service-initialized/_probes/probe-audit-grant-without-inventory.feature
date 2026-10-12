Feature: audit grant does not reveal inventory

  Scenario: grant audit access then reject inventory discovery
    When I run `kc grant add --principal auditor --action registry.audit.read`
    Then the output has:
      | id        | nonempty |
      | principal | auditor |
      | catalog   | absent |
      | actions.0 | registry.audit.read |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | auditor |
      | rules[].actions.0 | registry.audit.read |
    Then the output has:
      | rules.0.catalog   | absent |
    When I run `kc registry audit --as auditor`
    Then the output has:
      | source | registry |
      | catalogId | absent |
      | entries   | nonempty |
    When I run `kc show --as auditor`
    Then error FORBIDDEN
    When I run `kc show --as auditor`
    Then error FORBIDDEN
