# 可复用部署前态；用户任务从入口节点的 _bundles.yaml 开始。
Feature: catalog-initialized

  Scenario: construct
    Given deployment fixture
    When I run `kc show`
    Then the output has:
      | catalogId  | kr://scene/catalog |
      | datasets   | [] |
      | home       | absent |
      | namespace  | absent |
      | title      | absent |
      | summary    | absent |
    Then the output includes:
      | repositories[].id | kr://kc/system |
    Then the output has:
      | repositories.0.schemaCount | 4 |
      | repositories.1.id | absent |
