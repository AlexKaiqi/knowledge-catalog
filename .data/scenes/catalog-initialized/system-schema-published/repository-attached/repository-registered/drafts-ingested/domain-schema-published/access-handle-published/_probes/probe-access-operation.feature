# 在 access-handle-published 上：K11 按 ResourceDescriptor 的 operation/input 调墙外 runtime。

Feature: probe access operation

  Scenario: descriptor operation without runtime
    When I run `kc knowledge access --repo kr://scene/knowledge --object resource/orders-sql --operation query --input '{}'`
    Then error CAPABILITY_UNSATISFIED
