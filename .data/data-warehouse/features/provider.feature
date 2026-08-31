@cli
Feature: 数仓知识提供方发布 MySQL 物理知识与语义知识
  用例只调用仓库真实提供的 kc、MySQL Adapter、Collector 和 connector-preview。
  fixture 文件通过 $FIXTURE 引用；运行证据写入 $RUN。

  @DW-CLI-01 @mysql
  Scenario: 物理知识提供方首次接入并验证重复采集为空
    When I run `kc local init --home "$KC_HOME" --catalog kr://dw/catalog`
    Then stdout JSON satisfies:
      | path    | matcher | expected          |
      | catalog | equals  | kr://dw/catalog   |

    When I run `kc local repository attach --home "$KC_HOME" --catalog kr://dw/catalog --repo kr://dw/physical`
    Then stdout JSON satisfies:
      | path         | matcher     | expected         |
      | repositoryId | equals      | kr://dw/physical |
      | head         | is non-empty |                  |

    When I run `kc local grant bootstrap --home "$KC_HOME" --principal service:e2e`
    Then the command succeeds

    When I run `kc writer ingest --repo kr://dw/physical --dir "$FIXTURE/knowledge/physical" --out "$RUN/physical-schema.changeset.json" --origin-kind DEFINITION --actor-ref data-warehouse-domain-model --source-ref knowledge://data-warehouse/physical-aspects/v1`
    Then stdout JSON satisfies:
      | path                             | matcher   | expected |
      | diagnostics.schemaObjects        | equals    | 9        |
      | diagnostics.knowledgeUnits       | equals    | 1        |
      | diagnostics.files                | equals    | 10       |
      | changeSet.operations             | has length | 10      |

    When I run `kc writer commit --command-id dw-cli-01-physical-schema --changeset "$RUN/physical-schema.changeset.json" | tee "$RUN/physical-schema.receipt.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | APPLIED          |
      | result.repositoryId | equals       | kr://dw/physical |
      | result.commitId     | is non-empty |                  |

    When I run `kc writer commit --command-id dw-cli-01-physical-schema --changeset "$RUN/physical-schema.changeset.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | REPLAYED         |
      | result.repositoryId | equals       | kr://dw/physical |
      | result.commitId     | is non-empty |                  |

    When I run `jq -e --arg head "$(kc writer head --repo kr://dw/physical | jq -r '.commit')" '.result.commitId == $head' "$RUN/physical-schema.receipt.json"`
    Then the command succeeds

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

    When I run `kc writer put --command-id dw-cli-01-concurrent --repo kr://dw/physical --object note/concurrent --aspect properties --value '{"name":"concurrent target advance"}' --origin-kind DEFINITION`
    Then stdout JSON satisfies:
      | path             | matcher      | expected |
      | disposition      | equals       | APPLIED  |
      | result.newCommit | is non-empty |          |

    When I run `kc writer commit --command-id dw-cli-01-stale-mysql-v1 --changeset "$RUN/mysql-v1.changeset.json"`
    Then the command fails with stdout error code "NON_FAST_FORWARD"

    When I run `"$CONNECTOR_PREVIEW" --manifest "$FIXTURE/connector/connector.yaml" --observation "$RUN/mysql-v1.observation.json" --base "$(kc writer head --repo kr://dw/physical | jq -r '.commit')" --out "$RUN/mysql-v1-retry.preview.json"`
    Then the command succeeds
    And JSON file "$RUN/mysql-v1-retry.preview.json" satisfies:
      | path                 | matcher    | expected |
      | empty                | equals     | false    |
      | summary.added        | equals     | 101      |
      | summary.updated      | equals     | 0        |
      | summary.removed      | equals     | 0        |
      | changeSet.operations | has length | 101      |

    When I run `jq '.changeSet' "$RUN/mysql-v1-retry.preview.json" > "$RUN/mysql-v1-retry.changeset.json"`
    Then the command succeeds

    When I run `kc writer commit --command-id dw-cli-01-mysql-v1 --changeset "$RUN/mysql-v1-retry.changeset.json" | tee "$RUN/mysql-v1.receipt.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | APPLIED          |
      | result.repositoryId | equals       | kr://dw/physical |
      | result.commitId     | is non-empty |                  |

    When I run `kc writer commit --command-id dw-cli-01-mysql-v1 --changeset "$RUN/mysql-v1-retry.changeset.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | REPLAYED         |
      | result.repositoryId | equals       | kr://dw/physical |
      | result.commitId     | is non-empty |                  |

    When I run `jq -e --arg head "$(kc writer head --repo kr://dw/physical | jq -r '.commit')" '.result.commitId == $head' "$RUN/mysql-v1.receipt.json"`
    Then the command succeeds

    When I run `kc writer commit --command-id dw-cli-01-mysql-v1 --changeset "$RUN/physical-schema.changeset.json"`
    Then the command fails with stdout error code "IDEMPOTENCY_CONFLICT"

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
    When I run `kc local init --home "$KC_HOME" --catalog kr://dw/catalog`
    Then stdout JSON satisfies:
      | path    | matcher | expected        |
      | catalog | equals  | kr://dw/catalog |

    When I run `kc local repository attach --home "$KC_HOME" --catalog kr://dw/catalog --repo kr://dw/physical`
    Then stdout JSON satisfies:
      | path         | matcher      | expected         |
      | repositoryId | equals       | kr://dw/physical |
      | head         | is non-empty |                  |

    When I run `kc local repository attach --home "$KC_HOME" --catalog kr://dw/catalog --repo kr://dw/semantic`
    Then stdout JSON satisfies:
      | path         | matcher      | expected         |
      | repositoryId | equals       | kr://dw/semantic |
      | head         | is non-empty |                  |

    When I run `kc local grant bootstrap --home "$KC_HOME" --principal service:e2e`
    Then the command succeeds

    When I run `kc writer put --command-id dw-cli-02-invalid-schema --repo kr://dw/semantic --object invalid/metric --aspect properties --schema-ref schema/missing --value '{"name":"must not be committed"}' --origin-kind DEFINITION`
    Then the command fails with stdout error code "SCHEMA_REVISION_UNRESOLVED"

    When I run `kc writer ingest --repo kr://dw/semantic --dir "$FIXTURE/knowledge/semantic" --out "$RUN/semantic.changeset.json" --origin-kind DEFINITION --actor-ref semantic-sales --source-ref knowledge://finance/tpch-sales`
    Then stdout JSON satisfies:
      | path                       | matcher    | expected |
      | diagnostics.schemaObjects  | equals     | 7        |
      | diagnostics.knowledgeUnits | equals     | 8        |
      | diagnostics.files          | equals     | 15       |
      | changeSet.operations       | has length | 15       |

    When I run `kc writer commit --command-id dw-cli-02-semantic --changeset "$RUN/semantic.changeset.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | APPLIED          |
      | result.repositoryId | equals       | kr://dw/semantic |
      | result.commitId     | is non-empty |                  |

    When I run `kc knowledge read --repo kr://dw/semantic --ref refs/heads/main --object invalid/metric`
    Then the command fails with stdout error code "KNOWLEDGE_REF_UNRESOLVED"

  @DW-CLI-05 @mysql
  Scenario: 数据源故障和非法定向信号不产生伪观察且修正后可恢复
    When I run the command
      """
      env KC_MYSQL_CONTAINER=missing-kc-dw-source "$PYTHON" "$FIXTURE/connector/collector.py" <<'JSON'
      {"checkpoint":{},"signal":{"kind":"bootstrap-full"}}
      JSON
      """
    Then the command fails
    And stderr contains "mysql-tpch collector"
    And stdout is empty

    When I run `printf '%s\n' '{"checkpoint":{},"signal":{"kind":"bootstrap-full"}}' | "$PYTHON" "$FIXTURE/connector/collector.py" | tee "$RUN/source-recovery.observation.json"`
    Then stdout JSON satisfies:
      | path                      | matcher    | expected |
      | observation.coverage.kind | equals     | FULL     |
      | desired                   | has length | 101      |
      | nextCheckpoint.observed   | has length | 101      |

    When I run `jq '{checkpoint:.nextCheckpoint,signal:{kind:"invalidation",keys:["mysql:fixture:column:tpch.orders.o_orderkey"]}}' "$RUN/source-recovery.observation.json" | "$PYTHON" "$FIXTURE/connector/collector.py"`
    Then the command fails
    And stderr contains "unsupported targeted invalidation key"
    And stdout is empty

    When I run `jq '{checkpoint:{version:2,observed:.nextCheckpoint.observed,sourceKeyMap:.nextCheckpoint.sourceKeyMap},signal:{kind:"invalidation",keys:["mysql:fixture:table:tpch.orders"]}}' "$RUN/source-recovery.observation.json" | "$PYTHON" "$FIXTURE/connector/collector.py"`
    Then the command fails
    And stderr contains "targeted invalidation requires a v3 checkpoint"
    And stdout is empty

    When I run `jq '{checkpoint:.nextCheckpoint,signal:{kind:"invalidation",keys:["mysql:fixture:table:tpch.orders"]}}' "$RUN/source-recovery.observation.json" | "$PYTHON" "$FIXTURE/connector/collector.py" | tee "$RUN/targeted-recovery.observation.json"`
    Then stdout JSON satisfies:
      | path                           | matcher      | expected |
      | observation.coverage.kind      | equals       | KEYS     |
      | observation.coverage.keys      | has length   | 1        |
      | desired                        | has length   | 12       |
      | observed                       | has length   | 12       |
      | nextCheckpoint.observed        | has length   | 101      |
      | nextCheckpoint.sourceKeyMap    | is non-empty |          |
