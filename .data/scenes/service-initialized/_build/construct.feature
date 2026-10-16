# 可复用部署前态；用户任务从入口节点的 _bundles.yaml 开始。
Feature: service-initialized

  Scenario: construct
    Given deployment fixture
    When I run `kc show`
    Then the output has:
      | catalogId  | absent |
      | datasets   | [] |
      | home       | absent |
      | namespace  | absent |
      | title      | absent |
      | summary    | absent |
    Then the output includes:
      | repositories[].id | kr://kc/system |
    Then the output has:
      | repositories.0.id | kr://kc/system |
      | repositories.0.schemaCount | 12 |
      | repositories.1.id | absent |
    When I run `kc grant list`
    Then the output has:
      | rules | [] |
