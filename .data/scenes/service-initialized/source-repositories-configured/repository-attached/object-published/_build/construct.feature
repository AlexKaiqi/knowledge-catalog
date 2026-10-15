# object-published：语义仓已 COMMIT 一条普通对象（不绑 metric）。公开 `kc writer put`。

Feature: object-published

  Scenario: construct
    When I run `kc writer put urn:kc:object:note/hello --repo kr://scene/object --command-id publish-note-hello --file $materials/note.hello.json`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/object |
      | result.newCommit    | nonempty |
    When I run `kc read urn:kc:object:note/hello`
    Then the output has:
      | objectId | note/hello |
      | repository          | kr://scene/object |
      | value.text          | hi |
