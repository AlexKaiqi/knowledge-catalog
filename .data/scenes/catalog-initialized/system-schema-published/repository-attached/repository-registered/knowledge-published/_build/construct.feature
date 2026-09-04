# knowledge-published：语义仓已 COMMIT 一条普通知识（不绑 metric）。公开 `kc writer put`。

Feature: knowledge-published

  Scenario: construct
    When I run `kc writer put --command-id publish-note-hello --repo kr://scene/knowledge --object note/hello --file $materials/note.hello.json`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc knowledge read --repo kr://scene/knowledge --object note/hello`
    Then the output has:
      | knowledgeRef.object | note/hello |
      | repository          | kr://scene/knowledge |
      | value.text          | hi |
