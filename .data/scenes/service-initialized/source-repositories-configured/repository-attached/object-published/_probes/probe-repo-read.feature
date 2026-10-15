# 在 object-published 上：维护口 HEAD 已前进，--repo 精确读回正文。

Feature: probe repo read

  Scenario: read published object
    When I run `kc writer head --repo kr://scene/object`
    Then the output has:
      | repository | kr://scene/object |
      | commit     | nonempty |
    When I run `kc read urn:kc:object:note/hello`
    Then the output has:
      | objectId | note/hello |
      | repository          | kr://scene/object |
      | value.text          | hi |
    When I run `kc resolve --repo kr://scene/object --object note/hello`
    Then the output has:
      | status     | RESOLVED |
      | objectId   | note/hello |
      | repository | kr://scene/object |
      | address    | absent |
    When I run `kc log --repo kr://scene/object --object note/hello`
    Then the output has:
      | continuation | absent |
      | logs         | nonempty |
    When I run `kc provenance --repo kr://scene/object --object note/hello`
    Then the output has:
      | objectId   | note/hello |
      | repository | kr://scene/object |
    When I run `kc relations --repo kr://scene/object --object note/hello`
    Then error CAPABILITY_UNSATISFIED
