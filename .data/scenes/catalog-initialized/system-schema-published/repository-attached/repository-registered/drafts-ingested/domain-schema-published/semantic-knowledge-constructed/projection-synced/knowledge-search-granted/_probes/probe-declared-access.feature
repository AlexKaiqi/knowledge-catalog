# 在 knowledge-search-granted 上：只在 AccessHints 声明面上定位。

Feature: probe declared access

  @P-22 @KC-AGENT-01
  Scenario: declared access faces
    """
    Schema metric.definition 已声明：name 可 MATCH 也可 EQ，expression 只能 MATCH，
    unit 只能 EQ，measureKey 不是检索面。按声明面定位已发布的语义知识。
    """

    """
    Agent as bot (search-only)
    你是 bot。Schema 已声明 name 可 MATCH/EQ，expression 只能 MATCH，unit 只能 EQ，
    measureKey 不是检索面。你只有 knowledge.search，没有 knowledge.read。
    请按业务名称和公式片段定位 GMV；缺读权时命中应无正文，精确读取应被拒绝。
    不要用 measureKey 去搜，不要给自己授权。
    """

    When I run `kc knowledge search --as bot --repo kr://scene/knowledge --query merchandise`
    Then 1 hit metric/gmv
    When I run `kc knowledge search --as bot --repo kr://scene/knowledge --query l_extendedprice`
    Then 1 hit metric/gmv
    When I run `kc knowledge search --as bot --repo kr://scene/knowledge --query unique-measure-token-zz9`
    Then 0 hits
    When I run `kc knowledge search --as bot --repo kr://scene/knowledge --query CNY`
    Then 0 hits
    When I run `kc knowledge search --as bot --repo kr://scene/knowledge --eq unit=CNY`
    Then 1 hit metric/gmv
    When I run `kc knowledge search --as bot --repo kr://scene/knowledge --eq unit=USD`
    Then 0 hits
    When I run `kc knowledge search --as bot --repo kr://scene/knowledge --eq "name=Gross merchandise value"`
    Then 1 hit metric/gmv
    When I run `kc knowledge search --as bot --repo kr://scene/knowledge --eq "expression=SUM(l_extendedprice * (1 - l_discount))"`
    Then error CAPABILITY_UNSATISFIED
    When I run `kc knowledge search --as bot --repo kr://scene/knowledge --eq measureKey=unique-measure-token-zz9`
    Then error CAPABILITY_UNSATISFIED
    When I run `kc knowledge search --as bot --repo kr://scene/knowledge --match unit=CNY`
    Then error CAPABILITY_UNSATISFIED
