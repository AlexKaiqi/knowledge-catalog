# repository-archived：该仓在本 Catalog 生命周期结束。不删 Snapshot 对象。

Feature: repository-archived

  Scenario: construct
    When I run `kc catalog repo archive --repo kr://scene/knowledge`
    Then the output has:
      | repositoryId | kr://scene/knowledge |
      | archived     | true |
    When I run `kc local status`
    Then the output has:
      | home      | absent |
      | namespace | absent |
    Then the output includes:
      | repos[].id | kr://scene/knowledge |
