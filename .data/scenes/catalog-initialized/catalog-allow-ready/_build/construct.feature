# Catalog 授权面已就绪：allow 策略存在且为空。后续发权分叉都从这里出去。

Feature: catalog-allow-ready

  Scenario: construct
    When I run `kc grant list`
    Then the output has:
      | rules | [] |
