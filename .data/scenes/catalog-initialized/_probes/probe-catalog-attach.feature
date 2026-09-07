# 两间已配置 Catalog 共用部署；初始化/恢复由 deployment 的正式 Server 旅程验证。
Feature: configured catalogs

  Scenario: inventory exposes both configured catalogs
    Given configured catalog kr://scene/docs
    When I run `kc catalog list`
    Then the output includes:
      | catalogs[].id | kr://scene/catalog |
      | catalogs[].id | kr://scene/docs |
    When I run `kc catalog show --catalog kr://scene/docs`
    Then the output has:
      | catalogId | kr://scene/docs |
      | home      | absent |
      | namespace | absent |
