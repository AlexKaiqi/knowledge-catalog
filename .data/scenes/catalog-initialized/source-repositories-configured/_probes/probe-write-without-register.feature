# 在 source-repositories-configured 上：写入不需要 Catalog 承认该源。Writer 打到
# 已配置 Snapshot；库存仍只有 system。此探会改知识仓，执行器只在 home 副本上跑。

Feature: probe write without register

  Scenario: put does not require catalog admit
    When I run `kc writer put --command-id probe-unregistered-note --repo kr://scene/knowledge --object note/orphan --value '{"text":"orphan"}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc read --repo kr://scene/knowledge --object note/orphan`
    Then the output has:
      | objectId | note/orphan |
      | value.text          | orphan |
    When I run `kc show`
    Then the output has:
      | repositories.1.id | absent |
