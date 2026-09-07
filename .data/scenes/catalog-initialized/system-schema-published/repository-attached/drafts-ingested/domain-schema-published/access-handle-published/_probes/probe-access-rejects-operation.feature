# 在 access-handle-published 上：K10 只 hydrate Binding；Descriptor 操作走 knowledge invoke。

Feature: probe access rejects invoke flags

  Scenario: knowledge access does not take descriptor operations
    When I run `kc knowledge access --repo kr://scene/knowledge --object resource/orders-sql --operation query --input '{}'`
    Then error USAGE_INVALID
