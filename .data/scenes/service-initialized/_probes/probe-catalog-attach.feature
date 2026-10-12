Feature: a service has one registration space

  Scenario: retired selection cannot change inventory
    When I run `kc catalog list`
    Then error USAGE_INVALID
    When I run `kc catalog use kr://scene/docs`
    Then error USAGE_INVALID
    When I run `kc show`
    Then the output has:
      | catalogId | absent |
      | datasets | [] |
    Then the output includes:
      | repositories[].id | kr://kc/system |
