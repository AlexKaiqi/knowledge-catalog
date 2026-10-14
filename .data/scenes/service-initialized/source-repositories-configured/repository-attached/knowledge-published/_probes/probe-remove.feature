# 在 knowledge-published 上：remove 糖撤销一条对象；其它对象仍在。

Feature: probe remove

  Scenario: remove unpublishes one object
    When I run `kc writer put urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:note%2Ftmp --command-id publish-note-tmp --value '{"text":"tmp"}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
    When I run `kc writer remove urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:note%2Ftmp --command-id drop-note-tmp`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:note%2Ftmp`
    Then error KNOWLEDGE_REF_UNRESOLVED
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:note%2Fhello`
    Then the output has:
      | value.text | hi |
