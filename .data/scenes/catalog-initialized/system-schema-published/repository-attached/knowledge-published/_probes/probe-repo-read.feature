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
