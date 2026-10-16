Feature: Dataset 未选声明不能借用来源仓的字段检索

  Scenario: 清河已发表内容可回读而未声明的搜索不能冒充零命中
    When I run `kc dataset define --dataset qinghe-plain --revision 1 --source table-meta=refs/heads/main@tables@tables --source sales-semantic=refs/heads/main@metrics@metrics`
    Then the output has:
      | setId | qinghe-plain |
      | revision | 1 |
      | sources.0.repository | table-meta |
      | sources.1.repository | sales-semantic |
    When I run `kc read urn:kc:object:table/shop.orders --dataset qinghe-plain --revision 1`
    Then the output has:
      | objectId | table/shop.orders |
      | repository | table-meta |
      | commit | nonempty |
    When I run `kc search --dataset qinghe-plain --revision 1 --query orders`
    Then error CAPABILITY_UNSATISFIED
