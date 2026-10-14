Feature: Dataset 管理权不放行文件或仓级读搜

  Scenario: Dataset 管理权不放行文件或仓级读搜
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:metric%2Fgmv --as manager --dataset scene-set`
    Then error FORBIDDEN
    When I run `kc search --as manager --repo kr://scene/knowledge --query merchandise`
    Then error FORBIDDEN
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:metric%2Fgmv --as manager`
    Then error FORBIDDEN
