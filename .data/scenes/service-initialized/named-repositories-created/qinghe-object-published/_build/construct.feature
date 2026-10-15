# qinghe-object-published：两仓都把本节点材料目录整仓写入。

Feature: qinghe-object-published

  Scenario: construct
    When I run `kc writer commit --command-id walk-sales-semantic --repo sales-semantic --dir $materials/sales-semantic`
    Then the output has:
      | result.repositoryId | sales-semantic |
      | result.newCommit    | nonempty |
    When I run `kc writer commit --command-id walk-table-meta --repo table-meta --dir $materials/table-meta`
    Then the output has:
      | result.repositoryId | table-meta |
      | result.newCommit    | nonempty |
    When I run `kc read urn:kc:object:semantic-model/shop.sales#/properties`
    Then the output has:
      | objectId   | semantic-model/shop.sales |
      | value.name | Qinghe sales mart metric view |
    When I run `kc read urn:kc:object:semantic-model/shop.sales#/dimensions/order-status`
    Then the output has:
      | objectId   | semantic-model/shop.sales |
      | value.name | Order status |
    When I run `kc read urn:kc:object:semantic-model/shop.sales#/measures/gmv`
    Then the output has:
      | objectId   | semantic-model/shop.sales |
      | value.name | Gross merchandise value |
    When I run `kc read urn:kc:object:metric/shop.gmv#/definition`
    Then the output has:
      | objectId   | metric/shop.gmv |
      | value.name | Gross merchandise value |
      | value.measureKey | unit-price |
    When I run `kc read urn:kc:object:table/shop.orders#/properties`
    Then the output has:
      | objectId   | table/shop.orders |
      | value.name | orders |
    When I run `kc read urn:kc:object:column/shop.orders.order_id#/properties`
    Then the output has:
      | objectId   | column/shop.orders.order_id |
      | value.name | order_id |
    When I run `kc read urn:kc:object:table/shop.order_items#/schema`
    Then the output has:
      | value.columnCount | 6 |
    When I run `kc read urn:kc:object:data-job/shop.refresh_sales_mart#/properties`
    Then the output has:
      | objectId   | data-job/shop.refresh_sales_mart |
      | value.name | refresh_sales_mart |
    When I run `kc read urn:kc:object:rel/produces/refresh_sales_mart-sales_mart`
    Then the output has:
      | value.relationType | produces |
    When I run `kc binding show --repo table-meta --object table/shop.orders --aspect stats`
    Then the output has:
      | mode   | state |
      | origin | http://resource-access:7390 |
    When I run `kc binding show --repo table-meta --object table/shop.customers --aspect stats`
    Then the output has:
      | mode   | state |
      | origin | http://resource-access:7390 |
    When I run `kc access --repo table-meta --object table/shop.orders --aspect stats`
    Then the output has:
      | objectId       | table/shop.orders |
      | aspectName     | stats |
      | value.table    | orders |
      | value.rowCount | 6 |
    When I run `kc access --repo table-meta --object table/shop.customers --aspect stats`
    Then the output has:
      | objectId       | table/shop.customers |
      | aspectName     | stats |
      | value.table    | customers |
      | value.rowCount | 4 |
    When I run `kc access --repo table-meta --object table/shop.products --aspect stats`
    Then the output has:
      | objectId       | table/shop.products |
      | value.table    | products |
      | value.rowCount | 4 |
    When I run `kc invoke --repo table-meta --object resource/shop-sql --operation query --input '{"sql":"SELECT COUNT(*) FROM shop.customers"}'`
    Then the output has:
      | result.rows.0 | 4 |
    When I run `kc show`
    Then the output includes:
      | repositories[].id | table-meta |
      | repositories[].id | sales-semantic |
