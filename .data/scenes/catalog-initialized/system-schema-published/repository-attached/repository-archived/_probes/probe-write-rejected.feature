# 在 repository-archived 上：detach 后该仓不再出现在当前 Catalog show 中；Snapshot 仍可写。

Feature: probe write rejected

  Scenario: detached repository leaves catalog inventory
    When I run `kc show`
    Then the output includes:
      | repositories[].id | kr://kc/system |
    When I run `kc writer head --repo kr://scene/knowledge`
    Then the output has:
      | repository | kr://scene/knowledge |
      | commit     | nonempty |
