# 在 knowledge-set-defined 上：发现与 Dataset 消费是组合读，不解 object_id。

Feature: probe compose views

  Scenario: list dataset and access spec
    When I run `kc show`
    Then the output includes:
      | datasets[].id | scene-set |
    When I run `kc operations access-spec describe --dataset scene-set`
    Then the output has:
      | setId | scene-set |
      | specs       | nonempty |
