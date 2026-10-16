Feature: Dataset 查询只采用选入声明允许的字段用途

  Scenario: 未声明条件被明确拒绝并且正常关键词检索保持可用
    When I run `kc search --dataset scene-set --revision 1 --query merchandise`
    Then 1 hit metric/gmv
    When I run `kc search --dataset scene-set --revision 1 --eq undeclared-field=secret`
    Then error CAPABILITY_UNSATISFIED
