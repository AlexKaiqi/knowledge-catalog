# detach 回执、Catalog 中的 System 成员与仓 HEAD 均保留观察；不声称已验证写入拒绝或全部 Snapshot 内容。

Feature: 解除成员登记后 System 仍可见且该仓 HEAD 仍可读

  Scenario: 解除成员登记后 System 仍可见且该仓 HEAD 仍可读
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

    When I run `kc show`
    Then the output includes:
      | repositories[].id | kr://kc/system |
    When I run `kc writer head --repo kr://scene/knowledge`
    Then the output has:
      | repository | kr://scene/knowledge |
      | commit     | nonempty |
