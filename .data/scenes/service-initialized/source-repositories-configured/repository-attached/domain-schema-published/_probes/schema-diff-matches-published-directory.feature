Feature: Schema 草稿与已发布 HEAD 一致时差异为空

  Scenario: Schema 草稿与已发布 HEAD 一致时差异为空
    When I run `kc diff --repo kr://scene/knowledge --dir $materials/drafts`
    Then the output has:
      | repository | kr://scene/knowledge |
      | commit     | nonempty |
      | changes    | [] |
