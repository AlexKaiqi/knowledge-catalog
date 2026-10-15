Feature: read 拒绝已退役的 pin 参数

  Scenario: read 拒绝已退役的 pin 参数
    When I run `kc read urn:kc:object:metric/gmv --as consumer --pin $home/consume-pin.json`
    Then error USAGE_INVALID
