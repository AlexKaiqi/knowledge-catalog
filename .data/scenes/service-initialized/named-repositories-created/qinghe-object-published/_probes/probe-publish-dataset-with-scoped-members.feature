Feature: publish scoped Dataset and read admitted members

  Scenario: published Dataset includes tables and semantic objects but excludes jobs
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
    When I run `kc read urn:kc:object:table-meta:table%2Fshop.orders --dataset qinghe-sales`
    Then the output has:
      | objectId | table/shop.orders |
      | repository | table-meta |
      | commit | nonempty |
    When I run `kc read urn:kc:object:sales-semantic:semantic-model%2Fshop.sales --dataset qinghe-sales`
    Then the output has:
      | objectId | semantic-model/shop.sales |
      | repository | sales-semantic |
      | commit | nonempty |
    When I run `kc read urn:kc:object:sales-semantic:metric%2Fshop.gmv --dataset qinghe-sales`
    Then the output has:
      | objectId | metric/shop.gmv |
      | repository | sales-semantic |
      | commit | nonempty |
    When I run `kc read urn:kc:object:table-meta:data-job%2Fshop.refresh_sales_mart --dataset qinghe-sales`
    Then error FORBIDDEN
