# 在 object-published 上：remove 糖撤销一条对象；其它对象仍在。

Feature: probe remove

  Scenario: remove unpublishes one object
    When I run `kc writer put urn:kc:object:note/tmp --repo kr://scene/object --command-id publish-note-tmp --value '{"text":"tmp"}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/object |
    When I run `kc writer remove urn:kc:object:note/tmp --repo kr://scene/object --command-id drop-note-tmp`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/object |
      | result.newCommit    | nonempty |
    When I run `kc read urn:kc:object:note/tmp`
    Then error OBJECT_REF_UNRESOLVED
    When I run `kc read urn:kc:object:note/hello`
    Then the output has:
      | value.text | hi |
