# 在 repository-attached 上：写入不需要 Catalog 承认该源。

Feature: probe write without register

  Scenario: put does not require catalog admit
    When I run `kc writer put --command-id probe-unregistered-note --repo kr://scene/knowledge --object note/orphan --value '{"text":"orphan"}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc knowledge read --repo kr://scene/knowledge --object note/orphan`
    Then the output has:
      | knowledgeRef.object | note/orphan |
      | value.text          | orphan |
    When I run `kc catalog show`
    Then the output has:
      | repositories.1.id | absent |
