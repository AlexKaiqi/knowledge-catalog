@cli
Feature: 数仓知识提供方发布 MySQL 物理知识与语义知识
  用例只调用仓库真实提供的 kc、MySQL Adapter、Collector 和 connector-preview。
  fixture 文件通过 $FIXTURE 引用；运行证据写入 $RUN。

  @DW-CLI-01 @mysql
  Scenario: 物理知识提供方首次接入并验证重复采集为空
    When I run `kc init --home "$KC_HOME" --catalog kr://dw/catalog`
    Then stdout JSON satisfies:
      | path    | matcher | expected          |
      | catalog | equals  | kr://dw/catalog   |

    When I run `kc repo-add --home "$KC_HOME" --catalog kr://dw/catalog --repo kr://dw/physical`
    Then stdout JSON satisfies:
      | path         | matcher     | expected         |
      | repositoryId | equals      | kr://dw/physical |
      | head         | is non-empty |                  |

    When I run `kc ingest --home "$KC_HOME" --repo kr://dw/physical --dir "$FIXTURE/knowledge/physical" --out "$RUN/physical-schema.changeset.json" --origin-kind DEFINITION --actor-ref data-warehouse-domain-model --source-ref knowledge://data-warehouse/physical-aspects/v1`
    Then stdout JSON satisfies:
      | path                             | matcher   | expected |
      | diagnostics.schemaObjects        | equals    | 9        |
      | diagnostics.knowledgeUnits       | equals    | 0        |
      | diagnostics.files                | equals    | 9        |
      | changeSet.operations             | has length | 9       |

    When I run `kc commit --home "$KC_HOME" --command-id dw-cli-01-physical-schema --changeset "$RUN/physical-schema.changeset.json" | tee "$RUN/physical-schema.receipt.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | APPLIED          |
      | result.repositoryId | equals       | kr://dw/physical |
      | result.commitId     | is non-empty |                  |

    When I run `printf '%s\n' '{"operation":"listTables","arguments":{}}' | "$PYTHON" "$FIXTURE/connector/adapter.py"`
    Then stdout JSON satisfies:
      | path      | matcher    | expected   |
      | operation | equals     | listTables |
      | result    | has length | 8          |

    When I run `printf '%s\n' '{"operation":"describeSchema","arguments":{"table":"lineitem"}}' | "$PYTHON" "$FIXTURE/connector/adapter.py"`
    Then stdout JSON satisfies:
      | path      | matcher    | expected       |
      | operation | equals     | describeSchema |
      | result    | has length | 16             |

    When I run `printf '%s\n' '{"operation":"listJobs","arguments":{}}' | "$PYTHON" "$FIXTURE/connector/adapter.py"`
    Then stdout JSON satisfies:
      | path      | matcher    | expected |
      | operation | equals     | listJobs |
      | result    | has length | 1        |

    When I run `printf '%s\n' '{"checkpoint":{},"signal":{"kind":"bootstrap-full"}}' | "$PYTHON" "$FIXTURE/connector/collector.py" | tee "$RUN/mysql-v1.observation.json"`
    Then stdout JSON satisfies:
      | path                           | matcher    | expected |
      | mode                           | equals     | reconcile |
      | observation.coverage.kind      | equals     | FULL      |
      | desired                        | has length | 101       |
      | nextCheckpoint.observed        | has length | 101       |
      | nextCheckpoint.sourceKeyMap    | is non-empty |         |

    When I run `"$CONNECTOR_PREVIEW" --manifest "$FIXTURE/connector/connector.yaml" --observation "$RUN/mysql-v1.observation.json" --base "$(jq -r '.result.commitId' "$RUN/physical-schema.receipt.json")" --out "$RUN/mysql-v1.preview.json"`
    Then the command succeeds
    And JSON file "$RUN/mysql-v1.preview.json" satisfies:
      | path              | matcher    | expected |
      | empty             | equals     | false    |
      | summary.added     | equals     | 101      |
      | summary.updated   | equals     | 0        |
      | summary.removed   | equals     | 0        |
      | changeSet.operations | has length | 101   |

    When I run `jq '.changeSet' "$RUN/mysql-v1.preview.json" > "$RUN/mysql-v1.changeset.json"`
    Then the command succeeds
    And JSON file "$RUN/mysql-v1.changeset.json" satisfies:
      | path             | matcher    | expected         |
      | targetRepository | equals     | kr://dw/physical |
      | operations       | has length | 101              |

    When I run `kc commit --home "$KC_HOME" --command-id dw-cli-01-mysql-v1 --changeset "$RUN/mysql-v1.changeset.json" | tee "$RUN/mysql-v1.receipt.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | APPLIED          |
      | result.repositoryId | equals       | kr://dw/physical |
      | result.commitId     | is non-empty |                  |

    When I run `jq '{checkpoint:.nextCheckpoint,signal:{kind:"explicit-full-reconcile"}}' "$RUN/mysql-v1.observation.json" | "$PYTHON" "$FIXTURE/connector/collector.py" | tee "$RUN/mysql-repeat.observation.json"`
    Then stdout JSON satisfies:
      | path                      | matcher    | expected |
      | observation.coverage.kind | equals     | FULL     |
      | desired                   | has length | 101      |
      | observed                  | has length | 101      |

    When I run `"$CONNECTOR_PREVIEW" --manifest "$FIXTURE/connector/connector.yaml" --observation "$RUN/mysql-repeat.observation.json" --base "$(jq -r '.result.commitId' "$RUN/mysql-v1.receipt.json")" --out "$RUN/mysql-repeat.preview.json"`
    Then the command succeeds
    And JSON file "$RUN/mysql-repeat.preview.json" satisfies:
      | path              | matcher | expected |
      | empty             | equals  | true     |
      | summary.added     | equals  | 0        |
      | summary.updated   | equals  | 0        |
      | summary.removed   | equals  | 0        |
      | summary.unchanged | equals  | 101      |

  @DW-CLI-02
  Scenario: 语义知识提供方发布可直接入库的 Aspect Schema 与 OKF
    When I run `kc init --home "$KC_HOME" --catalog kr://dw/catalog`
    Then stdout JSON satisfies:
      | path    | matcher | expected        |
      | catalog | equals  | kr://dw/catalog |

    When I run `kc repo-add --home "$KC_HOME" --catalog kr://dw/catalog --repo kr://dw/physical`
    Then stdout JSON satisfies:
      | path         | matcher      | expected         |
      | repositoryId | equals       | kr://dw/physical |
      | head         | is non-empty |                  |

    When I run `kc repo-add --home "$KC_HOME" --catalog kr://dw/catalog --repo kr://dw/semantic`
    Then stdout JSON satisfies:
      | path         | matcher      | expected         |
      | repositoryId | equals       | kr://dw/semantic |
      | head         | is non-empty |                  |

    When I run `kc ingest --home "$KC_HOME" --repo kr://dw/semantic --dir "$FIXTURE/knowledge/semantic" --out "$RUN/semantic.changeset.json" --origin-kind DEFINITION --actor-ref semantic-sales --source-ref knowledge://finance/tpch-sales`
    Then stdout JSON satisfies:
      | path                       | matcher    | expected |
      | diagnostics.schemaObjects  | equals     | 7        |
      | diagnostics.knowledgeUnits | equals     | 9        |
      | diagnostics.files          | equals     | 16       |
      | changeSet.operations       | has length | 16       |

    When I run `kc commit --home "$KC_HOME" --command-id dw-cli-02-semantic --changeset "$RUN/semantic.changeset.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | APPLIED          |
      | result.repositoryId | equals       | kr://dw/semantic |
      | result.commitId     | is non-empty |                  |
