# knowledge-published：语义仓已 COMMIT 一条普通知识（不绑 metric）。公开 `kc writer put`。

Feature: knowledge-published

  Scenario: construct
    When I run `kc writer put urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:note%2Fhello --command-id publish-note-hello --file $materials/note.hello.json`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:note%2Fhello`
    Then the output has:
      | objectId | note/hello |
      | repository          | kr://scene/knowledge |
      | value.text          | hi |
