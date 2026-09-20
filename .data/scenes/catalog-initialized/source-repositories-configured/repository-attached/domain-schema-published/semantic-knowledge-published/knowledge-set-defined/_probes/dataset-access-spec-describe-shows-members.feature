Feature: 发现 Dataset 并查看成员访问声明

  Scenario: 发现 Dataset 并查看成员访问声明
    When I run `kc show`
    Then the output includes:
      | datasets[].id | scene-set |
    When I run `kc operations access-spec describe --dataset scene-set`
    Then the output has:
      | setId | scene-set |
      | specs       | nonempty |
