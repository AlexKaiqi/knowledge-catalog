# sales-dataset-defined：把两仓的表层对象打成 qinghe-sales。
# 物理仓只挂 tables/；语义仓只挂 semantic-models/ 与 metrics/。
# Define 冻结 selector→commit。消费走 --dataset；作业和列不在清单里。

Feature: sales-dataset-defined

  Scenario: construct
    When I run `kc dataset define --dataset qinghe-sales --revision 1 --source table-meta=refs/heads/main@tables@tables --source sales-semantic=refs/heads/main@semantic-models@semantic-models --source sales-semantic=refs/heads/main@metrics@metrics`
    Then the output has:
      | setId | qinghe-sales |
      | revision    | 1 |
      | sources.0.repository | table-meta |
      | sources.0.selector | refs/heads/main |
      | sources.0.subPath | tables |
      | sources.0.commit | nonempty |
      | sources.1.repository | sales-semantic |
      | sources.1.selector | refs/heads/main |
      | sources.1.subPath | semantic-models |
      | sources.1.commit | nonempty |
      | sources.2.repository | sales-semantic |
      | sources.2.selector | refs/heads/main |
      | sources.2.subPath | metrics |
      | sources.2.commit | nonempty |
    When I run `kc show`
    Then the output includes:
      | datasets[].id | qinghe-sales |
      | datasets[].revision | 1 |
      | repositories[].id | table-meta |
      | repositories[].id | sales-semantic |
    When I run `kc read --dataset qinghe-sales --object table/shop.orders`
    Then the output has:
      | 0.objectId | table/shop.orders |
      | 0.repository | table-meta |
      | 0.commit | nonempty |
    When I run `kc read --dataset qinghe-sales --object semantic-model/shop.sales`
    Then the output has:
      | 0.objectId | semantic-model/shop.sales |
      | 0.repository | sales-semantic |
      | 0.commit | nonempty |
    When I run `kc read --dataset qinghe-sales --object metric/shop.gmv`
    Then the output has:
      | 0.objectId | metric/shop.gmv |
      | 0.repository | sales-semantic |
      | 0.commit | nonempty |
    When I run `kc read --dataset qinghe-sales --object data-job/shop.refresh_sales_mart`
    Then the output has:
      |  | [] |
