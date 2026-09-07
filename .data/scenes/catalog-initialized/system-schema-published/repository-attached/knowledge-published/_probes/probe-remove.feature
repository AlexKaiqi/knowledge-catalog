# 在 knowledge-published 上：remove 糖撤销一条对象；其它对象仍在。

Feature: probe remove

  Scenario: remove unpublishes one object
    When I run `kc writer put --command-id publish-note-tmp --repo kr://scene/knowledge --object note/tmp --value '{"text":"tmp"}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
    When I run `kc writer remove --command-id drop-note-tmp --repo kr://scene/knowledge --object note/tmp`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc knowledge read --repo kr://scene/knowledge --object note/tmp`
    Then error KNOWLEDGE_REF_UNRESOLVED
    When I run `kc knowledge read --repo kr://scene/knowledge --object note/hello`
    Then the output has:
      | value.text | hi |
