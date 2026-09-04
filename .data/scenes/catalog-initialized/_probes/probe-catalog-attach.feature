# 在 catalog-initialized 上：本机再打开一间 Catalog 登记表。不把仓登记进配方。

Feature: probe second catalog

  Scenario: attach another catalog
    When I run `kc local catalog attach --catalog kr://scene/docs`
    Then the output has:
      | catalog | kr://scene/docs |
    When I run `kc local status`
    Then the output has:
      | home      | absent |
      | namespace | absent |
    Then the output includes:
      | catalogs[].id | kr://scene/catalog |
      | catalogs[].id | kr://scene/docs |
