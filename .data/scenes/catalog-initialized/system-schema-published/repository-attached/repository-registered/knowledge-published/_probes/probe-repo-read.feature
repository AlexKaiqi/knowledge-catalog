# 在 knowledge-published 上：维护口 HEAD 已前进，--repo 精确读回正文。

Feature: probe repo read

  Scenario: read published object
    When I run `kc writer head --repo kr://scene/knowledge`
    Then the output has:
      | repository | kr://scene/knowledge |
      | commit     | nonempty |
    When I run `kc knowledge read --repo kr://scene/knowledge --object note/hello`
    Then the output has:
      | knowledgeRef.object | note/hello |
      | repository          | kr://scene/knowledge |
      | value.text          | hi |
    When I run `kc knowledge resolve --repo kr://scene/knowledge --object note/hello`
    Then the output has:
      | status | RESOLVED |
    When I run `kc knowledge log --repo kr://scene/knowledge --object note/hello`
    Then the output has:
      | exhausted | true |
      | logs      | nonempty |
    When I run `kc knowledge provenance --repo kr://scene/knowledge --object note/hello`
    Then the output has:
      | objectId   | note/hello |
      | repository | kr://scene/knowledge |
    When I run `kc knowledge relations --repo kr://scene/knowledge --object note/hello`
    Then error CAPABILITY_UNSATISFIED
    When I run `kc workspace pin --source kr://scene/knowledge`
    Then the output has:
      | pinId | nonempty |
