# 在 knowledge-set-defined 上：反馈 outcome 非法。

Feature: probe feedback outcome

  Scenario: invalid feedback outcome
    When I run `kc operations feedback record --workspace scene-set --trace-id trace-x --outcome UNKNOWN`
    Then error USAGE_INVALID
