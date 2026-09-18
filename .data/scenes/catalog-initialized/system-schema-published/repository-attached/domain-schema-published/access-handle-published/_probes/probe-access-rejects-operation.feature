# 在 access-handle-published 上：K10 只 hydrate Binding；Descriptor 操作走 invoke。

Feature: probe access rejects invoke flags

  Scenario: access does not take descriptor operations
    When I run `kc access --repo kr://scene/knowledge --object resource/orders-sql --operation query --input '{}'`
    Then error USAGE_INVALID
