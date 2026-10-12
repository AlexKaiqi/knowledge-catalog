# 在 knowledge-published 上：维护口 HEAD 已前进，--repo 精确读回正文。

Feature: probe repo read

  Scenario: read published object
    When I run `kc writer head --repo kr://scene/knowledge`
    Then the output has:
      | repository | kr://scene/knowledge |
      | commit     | nonempty |
    When I run `kc read --repo kr://scene/knowledge --object note/hello`
    Then the output has:
      | objectId | note/hello |
      | repository          | kr://scene/knowledge |
      | value.text          | hi |
    When I run `kc resolve --repo kr://scene/knowledge --object note/hello`
    Then the output has:
      | status     | RESOLVED |
      | objectId   | note/hello |
      | repository | kr://scene/knowledge |
      | address    | absent |
    When I run `kc log --repo kr://scene/knowledge --object note/hello`
    Then the output has:
      | continuation | absent |
      | logs         | nonempty |
    When I run `kc provenance --repo kr://scene/knowledge --object note/hello`
    Then the output has:
      | objectId   | note/hello |
      | repository | kr://scene/knowledge |
    When I run `kc relations --repo kr://scene/knowledge --object note/hello`
    Then error CAPABILITY_UNSATISFIED
