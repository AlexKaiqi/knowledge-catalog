Feature: Dataset 管理权不放行文件或仓级读搜

  Scenario: Dataset 管理权不放行文件或仓级读搜
    When I run `kc read urn:kc:object:metric/gmv --as manager --dataset scene-set`
    Then error FORBIDDEN
    When I run `kc search --as manager --dataset scene-set --query merchandise`
    Then error FORBIDDEN
    When I run `kc read urn:kc:object:metric/gmv --as manager`
    Then error FORBIDDEN
