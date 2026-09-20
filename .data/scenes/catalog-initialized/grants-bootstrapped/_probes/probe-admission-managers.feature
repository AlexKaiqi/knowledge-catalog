# 在 grants-bootstrapped 上：未获权的人仍能看见至少一个发权管理者。

Feature: probe admission managers

  Scenario: bootstrap manager is visible without caller grants
    When I run `kc admission show --as reader`
    Then the output has:
      | principal | reader |
      | grants    | [] |
    Then the output includes:
      | request.administrators[] | admin |
