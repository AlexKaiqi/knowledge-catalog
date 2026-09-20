# 在 catalog-initialized 上：访问账与命中图可查；空窗不是错误。二者 source 不同。

Feature: probe access log

  Scenario: access and hitmap pages
    When I run `kc operations audit access`
    Then the output has:
      | source | access |
    When I run `kc operations audit hitmap`
    Then the output has:
      | source | hitmap |
