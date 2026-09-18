Feature: observation-refreshed probe

  Scenario: notice does not move HEAD
    When I run `kc writer head --repo kr://scene/knowledge`
    Then the output has:
      | repository | kr://scene/knowledge |
      | commit     | nonempty |
