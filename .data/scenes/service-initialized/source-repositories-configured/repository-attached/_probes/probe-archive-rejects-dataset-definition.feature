Feature: the registration space has no independent archive lifecycle

  Scenario: retired archive cannot prevent Dataset publication
    When I run `kc catalog archive`
    Then error USAGE_INVALID
    When I run `kc dataset define --dataset later --revision 1 --source kr://scene/object=refs/heads/main`
    Then the output has:
      | setId | later |
      | revision | 1 |
    When I run `kc show`
    Then the output has:
      | catalogId | absent |
      | archived | absent |
    Then the output includes:
      | datasets[].id | later |
