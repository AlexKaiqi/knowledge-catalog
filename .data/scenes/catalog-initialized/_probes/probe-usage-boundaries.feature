# 在 catalog-initialized 上：缺操作数的公开命令是 USAGE_INVALID，不是未知命令。

Feature: probe usage boundaries

  Scenario: incomplete public argv
    When I run `kc local workspace overlay`
    Then error USAGE_INVALID
    When I run `kc operations projection notice`
    Then error USAGE_INVALID
    When I run `kc operations audit trace`
    Then error USAGE_INVALID
    When I run `kc local system publish`
    Then error USAGE_INVALID
    When I run `kc local store set --repository filegit --index none`
    Then error USAGE_INVALID
