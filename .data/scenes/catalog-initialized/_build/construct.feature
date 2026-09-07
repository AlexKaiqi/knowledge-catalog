# 可复用部署前态；用户任务从 catalog.yaml 的 entry_state 开始。
Feature: catalog-initialized

  Scenario: construct
    Given deployment fixture
    When I run `kc catalog show`
    Then the output has:
      | catalogId  | kr://scene/catalog |
      | workspaces | [] |
      | home       | absent |
      | namespace  | absent |
    Then the output includes:
      | repositories[].id | kr://kc/system |
