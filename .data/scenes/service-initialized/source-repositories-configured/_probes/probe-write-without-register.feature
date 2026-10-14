# 在 source-repositories-configured 上：写入不需要 登记空间 承认该源。Writer 打到
# 已配置 Snapshot；库存仍只有 system。此探会改知识仓，执行器只在 home 副本上跑。

Feature: probe write without register

  Scenario: put does not require catalog admit
    When I run `kc writer put urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:note%2Forphan --command-id probe-unregistered-note --value '{"text":"orphan"}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:note%2Forphan`
    Then the output has:
      | objectId | note/orphan |
      | value.text          | orphan |
    When I run `kc show`
    Then the output has:
      | repositories.1.id | absent |
