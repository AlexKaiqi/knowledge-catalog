# catalog-initialized 已 init：① 登记表出生。construct 观测 init 回执，再读公开 status / show。

Feature: catalog-initialized

  Scenario: construct
    When I run `kc local init --catalog kr://scene/catalog`
    Then the output has:
      | catalog                | kr://scene/catalog |
      | system.repositoryId    | kr://kc/system |
      | system.metaSchema      | schema/meta/schema-definition/v1 |
      | system.commit          | nonempty |
    When I run `kc local status`
    Then the output has:
      | catalog.repositoryId | kr://scene/catalog |
      | repos                | [] |
      | workspaces           | [] |
      | archived             | false |
      | home                 | absent |
      | namespace            | absent |
    Then the output includes:
      | repositories  | kr://kc/system |
      | catalogs[].id | kr://scene/catalog |
    When I run `kc catalog show`
    Then the output has:
      | catalogId  | kr://scene/catalog |
      | workspaces | [] |
    Then the output includes:
      | repositories[].id | kr://kc/system |
