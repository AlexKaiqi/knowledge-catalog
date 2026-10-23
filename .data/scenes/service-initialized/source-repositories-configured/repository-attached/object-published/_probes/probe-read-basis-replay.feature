# 在 object-published 上：取得固定依据后 HEAD 前进，回读仍按原坐标。

Feature: probe read basis replay

  Scenario: read keeps its fixed basis after the head advances
    When I run `kc read urn:kc:object:note/hello`
    Then the output has:
      | objectId       | note/hello |
      | repository     | kr://scene/object |
      | value.text     | hi |
      | commit         | nonempty |
      | basis          | nonempty |
      | state.digest   | nonempty |
    And I remember basis as first-basis
    And I remember commit as first-commit
    And I remember state.digest as first-digest
    When I run `kc writer put urn:kc:object:note/hello --repo kr://scene/object --command-id publish-note-hello-second --value '{"text":"second"}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.changed      | true |
      | result.oldCommit    | $first-commit |
      | result.newCommit    | nonempty |
    When I run `kc read urn:kc:object:note/hello --basis $first-basis`
    Then the output has:
      | objectId      | note/hello |
      | repository    | kr://scene/object |
      | value.text    | hi |
      | commit        | $first-commit |
      | state.digest  | $first-digest |
    When I run `kc read urn:kc:object:note/hello`
    Then the output has:
      | value.text   | second |
      | commit       | nonempty |
      | basis        | nonempty |
