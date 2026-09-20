Feature: sync projections and find published knowledge

  Scenario: both repository projections serve their published objects
    When I run `kc operations projection sync --repo table-meta`
    Then the output has:
      | snapshot.repository  | table-meta |
      | snapshot.basisCommit | nonempty |
      | snapshot.objectCount | nonempty |
    When I run `kc operations projection sync --repo sales-semantic`
    Then the output has:
      | snapshot.repository  | sales-semantic |
      | snapshot.basisCommit | nonempty |
      | snapshot.objectCount | nonempty |
    When I run `kc operations projection describe --repo table-meta`
    Then the output has:
      | basisRepository | table-meta |
      | lagBehindHead   | false |
    When I run `kc operations projection describe --repo sales-semantic`
    Then the output has:
      | basisRepository | sales-semantic |
      | lagBehindHead   | false |
    When I run `kc search --repo table-meta --query orders`
    Then the output includes:
      | hits[].objectId | table/shop.orders |
    When I run `kc search --repo table-meta --query order_id`
    Then the output includes:
      | hits[].objectId | column/shop.orders.order_id |
    When I run `kc search --repo sales-semantic --query merchandise`
    Then the output includes:
      | hits[].objectId | metric/shop.gmv |
    When I run `kc search --repo sales-semantic --query "Order status"`
    Then the output includes:
      | hits[].objectId | semantic-model/shop.sales |
