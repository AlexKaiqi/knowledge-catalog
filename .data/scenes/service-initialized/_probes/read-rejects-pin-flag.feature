Feature: read 拒绝已退役的 pin 参数

  Scenario: read 拒绝已退役的 pin 参数
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fobject:metric%2Fgmv --as consumer --pin $home/consume-pin.json`
    Then error USAGE_INVALID
