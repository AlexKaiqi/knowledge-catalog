# 在 catalog-initialized 上：CLI whoami 是当前请求身份，不列 grant。

Feature: probe whoami

  Scenario: asserted principal
    When I run `kc whoami --as bot`
    Then the output has:
      | principal | bot |
