Feature: read 拒绝已退役的 pin 参数

  Scenario: read 拒绝已退役的 pin 参数
    When I run `kc read --as consumer --pin $home/consume-pin.json --object metric/gmv`
    Then error USAGE_INVALID
