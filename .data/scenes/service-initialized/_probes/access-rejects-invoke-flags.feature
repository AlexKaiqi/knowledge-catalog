Feature: access 拒绝仅属于 invoke 的操作参数

  Scenario: access 拒绝仅属于 invoke 的操作参数
    When I run `kc access --repo kr://scene/knowledge --object resource/orders-sql --operation query --input '{}'`
    Then error USAGE_INVALID
