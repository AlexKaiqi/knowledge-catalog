# 在 http-served 上：访问账可查；空窗不是错误。

Feature: probe access log

  Scenario: access and hitmap pages
    When I run `kc operations audit access`
    Then the output has:
      | source | access |
    When I run `kc operations audit hitmap`
    Then the output has:
      | source | access |
