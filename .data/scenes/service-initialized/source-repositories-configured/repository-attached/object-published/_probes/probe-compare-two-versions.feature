# 在 object-published 上：两版对象按双侧固定视图比较，不跟随最新。
# 对象比较是服务端语义：目录预览与它同名但不同动作，本地目录模式不能代替。

Feature: probe compare two versions

  Scenario: compare two fixed versions keeps both sides' coordinates
    Given local HTTP server
    When I run `kc grant add --principal compare-reader --action object.read`
    Then the output has:
      | principal | compare-reader |
      | actions.0 | object.read |
    When I run `kc read urn:kc:object:note/hello`
    Then the output has:
      | value.text | hi |
      | commit     | nonempty |
      | basis      | nonempty |
    And I remember basis as first-basis
    And I remember commit as first-commit
    When I run `kc writer put urn:kc:object:note/hello --repo kr://scene/object --command-id publish-note-hello-compared --value '{"text":"second"}'`
    Then the output has:
      | disposition      | APPLIED |
      | result.changed   | true |
      | result.newCommit | nonempty |
    And I remember result.newCommit as second-commit
    When I run `kc diff urn:kc:object:note/hello --from '{"repository":"kr://scene/object","commit":"$first-commit"}' --to '{"repository":"kr://scene/object","commit":"$second-commit"}' --as compare-reader --server $server`
    Then the output has:
      | reference                      | urn:kc:object:note/hello |
      | scopeChanged                   | false |
      | equal                          | false |
      | from.basis.candidates.0.commit | $first-commit |
      | to.basis.candidates.0.commit   | $second-commit |
      | changes.0.address.objectId     | note/hello |
      | changes.0.presence             | absent |
      | changes.0.valueChanged         | true |
      | changes.0.sourceChanged        | absent |
      | changes.0.before.value.text    | hi |
      | changes.0.after.value.text     | second |
    When I run `kc diff urn:kc:object:note/hello --from '{"basis":"$first-basis"}' --to '{"repository":"kr://scene/object","commit":"$second-commit"}' --as compare-reader --server $server`
    Then the output has:
      | reference    | urn:kc:object:note/hello |
      | scopeChanged | true |
      | equal        | absent |
    Then the output includes:
      | from.basis.candidates[].commit | $first-commit |
      | to.basis.candidates[].commit   | $second-commit |
    When I run `kc diff urn:kc:object:note/hello --from '{"repository":"kr://scene/object","commit":"$first-commit"}' --to '{"repository":"kr://scene/object","commit":"$second-commit"}' --as compare-reader --server $server --dir $home/drafts`
    Then error USAGE_INVALID
    When I run `kc diff urn:kc:object:note/hello --from '{"repository":"kr://scene/object"}' --to '{"repository":"kr://scene/object","commit":"$second-commit"}' --as compare-reader --server $server`
    Then error USAGE_INVALID
