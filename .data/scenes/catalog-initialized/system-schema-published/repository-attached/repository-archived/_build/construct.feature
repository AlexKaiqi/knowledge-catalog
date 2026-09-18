# repository-archived：该仓在本 Catalog 生命周期结束。不删 Snapshot 对象。
# 共享 live 走查仓留到最后再跑；隔离 TestProductScenes 覆盖本节点。

Feature: repository-archived

  Scenario: construct
    When I run `kc detach --repo kr://scene/knowledge`
    Then the output has:
      | repositoryId | kr://scene/knowledge |
      | detached     | true |
    When I run `kc writer head --repo kr://scene/knowledge`
    Then the output has:
      | home      | absent |
      | namespace | absent |
      | repository | kr://scene/knowledge |
      | commit     | nonempty |
