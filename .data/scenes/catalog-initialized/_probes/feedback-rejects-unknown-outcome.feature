Feature: 记录反馈时拒绝未知 outcome

  Scenario: 记录反馈时拒绝未知 outcome
    When I run `kc operations feedback record --dataset scene-set --trace-id trace-x --outcome UNKNOWN`
    Then error USAGE_INVALID
