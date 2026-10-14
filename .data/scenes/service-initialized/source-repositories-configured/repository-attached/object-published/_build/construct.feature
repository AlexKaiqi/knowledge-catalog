# object-published：语义仓已 COMMIT 一条普通对象（不绑 metric）。公开 `kc writer put`。

Feature: object-published

  Scenario: construct
    When I run `kc writer put urn:kc:object:kr%3A%2F%2Fscene%2Fobject:note%2Fhello --command-id publish-note-hello --file $materials/note.hello.json`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/object |
      | result.newCommit    | nonempty |
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fobject:note%2Fhello`
    Then the output has:
      | objectId | note/hello |
      | repository          | kr://scene/object |
      | value.text          | hi |
