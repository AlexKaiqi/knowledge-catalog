@cli @mysql
Feature: 第一次接触的数据消费方通过 Workspace 使用数仓知识

  @DW-CLI-03
  Scenario: 消费方组合两个 Repository 并读取表、作业、语义和来源
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

    When I run `kc ingest --home "$KC_HOME" --repo kr://dw/physical --dir "$FIXTURE/knowledge/physical" --out "$RUN/physical-schema.changeset.json" --origin-kind DEFINITION --actor-ref data-warehouse-domain-model --source-ref knowledge://data-warehouse/physical-aspects/v1`
    Then stdout JSON satisfies:
      | path                       | matcher    | expected |
      | diagnostics.schemaObjects  | equals     | 9        |
      | changeSet.operations       | has length | 9        |

    When I run `kc commit --home "$KC_HOME" --command-id dw-cli-03-physical-schema --changeset "$RUN/physical-schema.changeset.json" | tee "$RUN/physical-schema.receipt.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | APPLIED          |
      | result.repositoryId | equals       | kr://dw/physical |
      | result.commitId     | is non-empty |                  |

    When I run `printf '%s\n' '{"checkpoint":{},"signal":{"kind":"bootstrap-full"}}' | "$PYTHON" "$FIXTURE/connector/collector.py" | tee "$RUN/mysql.observation.json"`
    Then stdout JSON satisfies:
      | path                      | matcher    | expected |
      | observation.coverage.kind | equals     | FULL     |
      | desired                   | has length | 101      |

    When I run `"$CONNECTOR_PREVIEW" --manifest "$FIXTURE/connector/connector.yaml" --observation "$RUN/mysql.observation.json" --base "$(jq -r '.result.commitId' "$RUN/physical-schema.receipt.json")" --out "$RUN/mysql.preview.json"`
    Then the command succeeds
    And JSON file "$RUN/mysql.preview.json" satisfies:
      | path          | matcher | expected |
      | empty         | equals  | false    |
      | summary.added | equals  | 101      |

    When I run `jq '.changeSet' "$RUN/mysql.preview.json" > "$RUN/mysql.changeset.json"`
    Then the command succeeds
    And JSON file "$RUN/mysql.changeset.json" satisfies:
      | path             | matcher    | expected         |
      | targetRepository | equals     | kr://dw/physical |
      | operations       | has length | 101              |

    When I run `kc commit --home "$KC_HOME" --command-id dw-cli-03-mysql --changeset "$RUN/mysql.changeset.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | APPLIED          |
      | result.repositoryId | equals       | kr://dw/physical |
      | result.commitId     | is non-empty |                  |

    When I run `kc ingest --home "$KC_HOME" --repo kr://dw/semantic --dir "$FIXTURE/knowledge/semantic" --out "$RUN/semantic.changeset.json" --origin-kind DEFINITION --actor-ref semantic-sales --source-ref knowledge://finance/tpch-sales`
    Then stdout JSON satisfies:
      | path                       | matcher    | expected |
      | diagnostics.schemaObjects  | equals     | 7        |
      | diagnostics.knowledgeUnits | equals     | 9        |
      | changeSet.operations       | has length | 16       |

    When I run `kc commit --home "$KC_HOME" --command-id dw-cli-03-semantic --changeset "$RUN/semantic.changeset.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | APPLIED          |
      | result.repositoryId | equals       | kr://dw/semantic |
      | result.commitId     | is non-empty |                  |

    When I run `kc define-workspace --home "$KC_HOME" --catalog kr://dw/catalog --workspace warehouse-agent --revision 1 --source kr://dw/physical=refs/heads/main --source kr://dw/semantic=refs/heads/main`
    Then stdout JSON satisfies:
      | path        | matcher    | expected        |
      | workspaceId | equals     | warehouse-agent |
      | revision    | equals     | 1               |
      | sources     | has length | 2               |

    When I run `kc resolve --home "$KC_HOME" --catalog kr://dw/catalog --workspace warehouse-agent | tee "$RUN/v1.pin.json"`
    Then stdout JSON satisfies:
      | path                              | matcher      | expected        |
      | workspaceId                       | equals       | warehouse-agent |
      | revision                          | equals       | 1               |
      | pinId                             | is non-empty |                  |
      | repositories.kr://dw/physical     | is non-empty |                  |
      | repositories.kr://dw/semantic     | is non-empty |                  |

    When I run `kc read --home "$KC_HOME" --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068`
    Then stdout JSON satisfies:
      | path                              | matcher    | expected |
      | $                                 | has length | 1        |
      | [0].value.properties.name         | equals     | lineitem |
      | [0].value.schema.columnCount      | equals     | 16       |

    When I run `kc read --home "$KC_HOME" --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-data-job-2da1aa95c4226ac7a681db63`
    Then stdout JSON satisfies:
      | path                           | matcher | expected              |
      | [0].value.properties.name      | equals  | inspect_urgent_orders |
      | [0].value.definition.language  | equals  | SQL                   |
      | [0].value.definition.enabled   | equals  | false                 |

    When I run `kc read --home "$KC_HOME" --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-semantic-sales-semantic-model-d40acd4665b1011643d74d5a`
    Then stdout JSON satisfies:
      | path                                     | matcher    | expected                                         |
      | [0].value.definition.baseTableRef        | equals     | dw-mysql-tpch-table-c02fedc564bba85c8d5d1068    |
      | [0].value.dimensions                     | is non-empty |                                                  |
      | [0].value.measures                       | is non-empty |                                                  |

    When I run `kc read --home "$KC_HOME" --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-semantic-sales-metric-7630439d2660b81de165d124`
    Then stdout JSON satisfies:
      | path                         | matcher | expected                |
      | [0].value.properties.name    | equals  | Gross merchandise value |

    When I run `kc describe-schema --home "$KC_HOME" --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068`
    Then stdout JSON satisfies:
      | path | matcher    | expected |
      | $    | has length | 1        |
      | [0].schemas | has length | 2  |

    When I run `kc relations --home "$KC_HOME" --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068 --relation-type models --role base`
    Then stdout JSON satisfies:
      | path | matcher    | expected |
      | $    | has length | 1        |

    When I run `kc provenance --home "$KC_HOME" --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068`
    Then stdout JSON satisfies:
      | path                    | matcher | expected |
      | [0].chain[0].originKind | equals  | SOURCE   |

    When I run `kc provenance --home "$KC_HOME" --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-semantic-sales-metric-7630439d2660b81de165d124`
    Then stdout JSON satisfies:
      | path                    | matcher | expected   |
      | [0].chain[0].originKind | equals  | DEFINITION |
