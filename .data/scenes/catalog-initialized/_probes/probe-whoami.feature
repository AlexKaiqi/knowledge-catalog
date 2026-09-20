# 在 catalog-initialized 上：CLI whoami 是当前请求身份，不列 grant。
# 此节点用 reader（已认证、无 grant）；bot 是 catalog-read-granted 的消费者。

Feature: probe whoami

  Scenario: asserted principal
    When I run `kc whoami --as reader`
    Then the output has:
      | principal | reader |
      | grants    | absent |
