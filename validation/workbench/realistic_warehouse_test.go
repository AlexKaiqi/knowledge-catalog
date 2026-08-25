package scenario

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kc/catalog"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/reader"
	"kc/repository"
	"kc/writer"
)

const (
	realisticCatalog   = "kr://acme/validation/warehouse-catalog"
	realisticWorkspace = "finance-analyst-board"
	realisticMySQL     = "mysql://127.0.0.1:13306/tpch"
)

var (
	realisticPhysical = kernel.RepositoryID("kr://acme/validation/warehouse-metadata")
	realisticSemantic = kernel.RepositoryID("kr://acme/validation/warehouse-semantics")
)

// TestRealisticWarehouseKnowledgeGraph proves that the data-warehouse fixture
// is one connected body of knowledge rather than unrelated protocol samples.
// The path is intentionally user-shaped:
//
//	GMV -> measure -> warehouse column -> table -> producing ETL task
//	    -> upstream table/column -> source MySQL asset
//
// It also keeps three easily-confused concerns separate: join evidence is not
// production lineage, a source-system permissions Aspect is not kc allow, and
// dynamic run history is a versioned Stream Binding rather than an ever-growing Entity.
func TestRealisticWarehouseKnowledgeGraph(t *testing.T) {
	store := repository.NewStore()
	t.Cleanup(func() { _ = store.Close() })
	physical := testkit.MakeRepository(t, string(realisticPhysical))
	semantic := testkit.MakeRepository(t, string(realisticSemantic))
	for _, repo := range []repository.Repository{physical, semantic} {
		if err := store.Add(repo); err != nil {
			t.Fatal(err)
		}
	}
	w, err := writer.NewWriter(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := catalog.NewRegistry(testkit.TempDir(t), realisticCatalog)
	if err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.NewCatalog(store, registry)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []kernel.RepositoryID{realisticPhysical, realisticSemantic} {
		if err := cat.RegisterRepository(id); err != nil {
			t.Fatal(err)
		}
	}

	physicalSchemaCommit := realisticCommit(t, w, "warehouse-physical-schemas-v1", realisticPhysical,
		physicalKnowledgeSchemas(), definitionEnvelope("metadata-platform"))
	_ = physicalSchemaCommit
	physicalCommit := realisticCommit(t, w, "warehouse-source-assets-v1", realisticPhysical,
		sourceAssetKnowledge(), &kernel.ProvenanceEnvelope{
			OriginKind: kernel.OriginSource, ActorRef: "mysql-structure-collector",
			SourceRefs: []string{realisticMySQL},
		})
	transformations := append(transformationKnowledge(), realisticRunsBinding())
	physicalCommit = realisticCommit(t, w, "warehouse-scheduler-definitions-v1", realisticPhysical,
		transformations, &kernel.ProvenanceEnvelope{
			OriginKind: kernel.OriginSource, ActorRef: "scheduler-definition-collector",
			SourceRefs: []string{"airflow://warehouse/tpch-daily"},
		})
	physicalCommit = realisticCommit(t, w, "warehouse-ranger-permissions-v1", realisticPhysical,
		governanceKnowledge(), &kernel.ProvenanceEnvelope{
			OriginKind: kernel.OriginSource, ActorRef: "ranger-policy-collector",
			SourceRefs: []string{"ranger://warehouse/policy-snapshot-2026-08-24"},
		})

	semanticSchemaCommit := realisticCommit(t, w, "warehouse-semantic-schemas-v1", realisticSemantic,
		semanticKnowledgeSchemas(), definitionEnvelope("semantic-platform"))
	_ = semanticSchemaCommit
	semanticCommit := realisticCommit(t, w, "warehouse-sales-semantic-model-v1", realisticSemantic,
		semanticKnowledge(), definitionEnvelope("finance-steward"))

	if _, err := cat.DefineWorkspace(realisticWorkspace, 1, []catalog.WorkspaceSource{
		{Repository: realisticPhysical, Selector: MainRef},
		{Repository: realisticSemantic, Selector: MainRef},
	}); err != nil {
		t.Fatal(err)
	}
	resolved, err := cat.ResolveWorkspace(realisticWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	serving := reader.Open(cat.RequireKnowledge, workspacePin(resolved))
	if resolved.Repositories[realisticPhysical] != physicalCommit || resolved.Repositories[realisticSemantic] != semanticCommit {
		t.Fatalf("workspace did not pin published graph: %#v", resolved.Repositories)
	}

	listed, err := serving.List()
	if err != nil {
		t.Fatal(err)
	}
	assertRealisticEntityInventory(t, listed)
	referenceCount := assertRealisticReferenceIntegrity(t, listed)
	lineagePath := assertGMVLineage(t, serving)
	assertJoinEvidenceIsNotLineage(t, serving)
	assertPermissionBoundary(t, serving)
	assertRuntimeBinding(t, reader.NewReader(store), physicalCommit)
	assertRealisticProvenance(t, reader.NewReader(store), physicalCommit, semanticCommit)

	writeRealisticEvidence(t, map[string]any{
		"validation":         "realistic-warehouse-knowledge-graph",
		"outcome":            "PASSED",
		"executedAt":         time.Now().Format(time.RFC3339),
		"workspace":          realisticWorkspace,
		"physicalCommit":     physicalCommit,
		"semanticCommit":     semanticCommit,
		"runBindingCommit":   physicalCommit,
		"resolvedReferences": referenceCount,
		"workspaceObjects":   len(listed),
		"lineagePath":        lineagePath,
		"entityTypes": []string{
			"DataSource", "ResourceDescriptor", "Database", "Schema", "Table", "Column", "ETLJob", "ETLTask",
			"MetricView", "Dimension", "Measure", "Metric", "QualityRule",
		},
		"aspects": []string{
			"structure", "definition", "ownership", "schedule", "inputs", "outputs",
			"columnMappings", "joinEvidence", "profile", "freshness", "classification",
			"permissions", "dependencies", "certification", "qualityTargets",
		},
		"assertions": []string{
			"every referenced entity resolves at one Workspace pin",
			"GMV traces through semantic and column-level ETL dependencies to MySQL source columns",
			"join evidence stays distinct from transformation lineage",
			"source-system grants do not authorize Knowledge Catalog access",
			"ETL run history is declared by a pinned Stream Binding and observed by an upper-layer runtime",
		},
	})
}

func physicalKnowledgeSchemas() []repository.Operation {
	return []repository.Operation{
		realisticSchema("schema/data-source.definition", "DataSource", "definition", "record"),
		realisticSchema("schema/resource-descriptor.definition", "ResourceDescriptor", "definition", "record"),
		realisticSchema("schema/database.structure", "Database", "structure", "record"),
		realisticSchema("schema/schema.structure", "Schema", "structure", "record"),
		realisticSchema("schema/table.structure", "Table", "structure", "record"),
		realisticSchema("schema/table.profile", "Table", "profile", "record"),
		realisticSchema("schema/table.freshness", "Table", "freshness", "record"),
		realisticSchema("schema/table.join-evidence", "Table", "joinEvidence", "keyed_collection"),
		realisticSchema("schema/table.permissions", "Table", "permissions", "keyed_collection"),
		realisticSchema("schema/column.structure", "Column", "structure", "record"),
		realisticSchema("schema/column.classification", "Column", "classification", "record"),
		realisticSchema("schema/etl-job.definition", "ETLJob", "definition", "record"),
		realisticSchema("schema/etl-job.schedule", "ETLJob", "schedule", "record"),
		realisticSchema("schema/etl-job.tasks", "ETLJob", "tasks", "keyed_collection"),
		realisticSchema("schema/etl-task.definition", "ETLTask", "definition", "record"),
		realisticSchema("schema/etl-task.io", "ETLTask", "inputs", "keyed_collection"),
		realisticSchema("schema/etl-task.outputs", "ETLTask", "outputs", "keyed_collection"),
		realisticSchema("schema/etl-task.column-mappings", "ETLTask", "columnMappings", "keyed_collection"),
		realisticSchema("schema/etl-task.runtime-summary", "ETLTask", "runtimeSummary", "record"),
		realisticSchema("schema/ownership", "Any", "ownership", "record"),
		realisticSchema("schema/quality-rule.definition", "QualityRule", "definition", "record"),
		realisticSchema("schema/quality-rule.targets", "QualityRule", "qualityTargets", "keyed_collection"),
	}
}

func semanticKnowledgeSchemas() []repository.Operation {
	return []repository.Operation{
		realisticSchema("schema/metric-view.definition", "MetricView", "definition", "record"),
		realisticSchema("schema/dimension.definition", "Dimension", "definition", "record"),
		realisticSchema("schema/measure.definition", "Measure", "definition", "record"),
		realisticSchema("schema/metric.definition", "Metric", "definition", "record"),
		realisticSchema("schema/semantic.dependencies", "Any", "dependencies", "keyed_collection"),
		realisticSchema("schema/semantic.certification", "Any", "certification", "record"),
		realisticSchema("schema/semantic.ownership", "Any", "ownership", "record"),
	}
}

func sourceAssetKnowledge() []repository.Operation {
	orders := sourceTableID("orders")
	lineitem := sourceTableID("lineitem")
	customer := sourceTableID("customer")
	nation := sourceTableID("nation")
	ops := []repository.Operation{
		realisticAspect("DataSource:mysql-tpch", "definition", "schema/data-source.definition", map[string]any{
			"name": "TPC-H MySQL", "engine": "mysql", "resourceRef": "ResourceDescriptor:mysql-tpch", "environment": "validation",
		}),
		realisticAspect("ResourceDescriptor:mysql-tpch", "definition", "schema/resource-descriptor.definition", map[string]any{
			"protocol": "mysql-query/v1", "locator": "resource://warehouse/mysql-tpch", "credentialRef": "vault://warehouse/mysql-tpch-readonly",
			"capabilities": []any{"describe", "sample"}, "containsLiveContent": false,
		}),
		realisticAspect("Database:tpch", "structure", "schema/database.structure", map[string]any{
			"name": "tpch", "sourceId": "DataSource:mysql-tpch",
		}),
		realisticAspect("Schema:tpch", "structure", "schema/schema.structure", map[string]any{
			"name": "tpch", "databaseId": "Database:tpch",
		}),
		sourceTable(orders, "orders", 15000),
		sourceTable(lineitem, "lineitem", 60175),
		sourceTable(customer, "customer", 1500),
		sourceTable(nation, "nation", 25),
		realisticMember(lineitem, "joinEvidence", "lineitem-orders", "schema/table.join-evidence", map[string]any{
			"purpose": "joinability", "parentObjectId": orders,
			"childColumns":  []any{sourceColumnID("lineitem", "l_orderkey")},
			"parentColumns": []any{sourceColumnID("orders", "o_orderkey")},
			"orphanCount":   0, "evidenceSqlDigest": "sha256:join-lineitem-orders-v1",
		}),
		realisticAspect(lineitem, "profile", "schema/table.profile", map[string]any{
			"rowCount": 60175, "profiledAt": "2026-08-24T02:00:00+08:00",
		}),
	}
	for _, c := range []struct {
		table, name, dataType string
		ordinal               int
	}{
		{"orders", "o_orderkey", "bigint", 1},
		{"orders", "o_custkey", "bigint", 2},
		{"orders", "o_orderdate", "date", 5},
		{"lineitem", "l_orderkey", "bigint", 1},
		{"lineitem", "l_extendedprice", "decimal(15,2)", 6},
		{"lineitem", "l_discount", "decimal(15,2)", 7},
		{"customer", "c_custkey", "bigint", 1},
		{"customer", "c_name", "varchar(25)", 2},
		{"customer", "c_phone", "char(15)", 5},
		{"customer", "c_nationkey", "bigint", 4},
		{"nation", "n_nationkey", "bigint", 1},
		{"nation", "n_name", "char(25)", 2},
	} {
		ops = append(ops, sourceColumn(c.table, c.name, c.dataType, c.ordinal))
	}
	return ops
}

func transformationKnowledge() []repository.Operation {
	job := kernel.ObjectID("ETLJob:tpch-daily")
	syncOrders := kernel.ObjectID("ETLTask:sync-orders")
	syncLineitem := kernel.ObjectID("ETLTask:sync-lineitem")
	buildTrade := kernel.ObjectID("ETLTask:build-trade-order")
	aggregate := kernel.ObjectID("ETLTask:aggregate-sales-daily")
	odsOrders := kernel.ObjectID("Table:ods.orders")
	odsLineitem := kernel.ObjectID("Table:ods.lineitem")
	dwdTrade := kernel.ObjectID("Table:dwd.trade_order")
	dwsSales := kernel.ObjectID("Table:dws.sales_daily")

	ops := []repository.Operation{
		realisticAspect(job, "definition", "schema/etl-job.definition", map[string]any{
			"name": "tpch-daily", "engine": "airflow", "dagId": "tpch_daily", "description": "Daily TPC-H ingestion and governed sales marts",
		}),
		realisticAspect(job, "schedule", "schema/etl-job.schedule", map[string]any{
			"cron": "0 2 * * *", "timezone": "Asia/Shanghai", "catchup": false,
		}),
		realisticAspect(job, "ownership", "schema/ownership", map[string]any{"owner": "group:data-engineering"}),
		realisticMember(job, "tasks", "01-sync-orders", "schema/etl-job.tasks", map[string]any{"taskId": syncOrders, "position": 1}),
		realisticMember(job, "tasks", "02-sync-lineitem", "schema/etl-job.tasks", map[string]any{"taskId": syncLineitem, "position": 2}),
		realisticMember(job, "tasks", "03-build-trade-order", "schema/etl-job.tasks", map[string]any{"taskId": buildTrade, "position": 3}),
		realisticMember(job, "tasks", "04-aggregate-sales-daily", "schema/etl-job.tasks", map[string]any{"taskId": aggregate, "position": 4}),
	}

	for _, table := range []struct {
		id, schema, name, layer string
	}{
		{string(odsOrders), "ods", "orders", "ods"},
		{string(odsLineitem), "ods", "lineitem", "ods"},
		{string(dwdTrade), "dwd", "trade_order", "dwd"},
		{string(dwsSales), "dws", "sales_daily", "dws"},
	} {
		ops = append(ops, realisticAspect(kernel.ObjectID(table.id), "structure", "schema/table.structure", map[string]any{
			"name": table.name, "qualifiedName": table.schema + "." + table.name,
			"schemaId": "Schema:" + table.schema, "layer": table.layer, "tableType": "MANAGED_TABLE",
		}))
	}
	for _, schema := range []string{"ods", "dwd", "dws"} {
		ops = append(ops, realisticAspect(kernel.ObjectID("Schema:"+schema), "structure", "schema/schema.structure", map[string]any{
			"name": schema, "databaseId": "Database:warehouse",
		}))
	}
	ops = append(ops, realisticAspect("Database:warehouse", "structure", "schema/database.structure", map[string]any{
		"name": "warehouse", "sourceId": "DataSource:starrocks-warehouse",
	}))
	ops = append(ops, realisticAspect("DataSource:starrocks-warehouse", "definition", "schema/data-source.definition", map[string]any{
		"name": "Analytics Warehouse", "engine": "starrocks", "resourceRef": "ResourceDescriptor:starrocks-warehouse", "environment": "validation",
	}))
	ops = append(ops, realisticAspect("ResourceDescriptor:starrocks-warehouse", "definition", "schema/resource-descriptor.definition", map[string]any{
		"protocol": "starrocks-query/v1", "locator": "resource://warehouse/starrocks", "credentialRef": "vault://warehouse/starrocks-analyst",
		"capabilities": []any{"describe", "query"}, "containsLiveContent": false,
	}))

	ops = append(ops,
		derivedColumn("Column:ods.orders.order_id", odsOrders, "order_id", "bigint", 1),
		derivedColumn("Column:ods.orders.customer_id", odsOrders, "customer_id", "bigint", 2),
		derivedColumn("Column:ods.orders.order_date", odsOrders, "order_date", "date", 3),
		derivedColumn("Column:ods.lineitem.order_id", odsLineitem, "order_id", "bigint", 1),
		derivedColumn("Column:ods.lineitem.extended_price", odsLineitem, "extended_price", "decimal(15,2)", 2),
		derivedColumn("Column:ods.lineitem.discount_rate", odsLineitem, "discount_rate", "decimal(15,2)", 3),
		derivedColumn("Column:dwd.trade_order.order_id", dwdTrade, "order_id", "bigint", 1),
		derivedColumn("Column:dwd.trade_order.order_date", dwdTrade, "order_date", "date", 2),
		derivedColumn("Column:dwd.trade_order.nation_name", dwdTrade, "nation_name", "varchar(25)", 3),
		derivedColumn("Column:dwd.trade_order.gross_amount", dwdTrade, "gross_amount", "decimal(18,2)", 4),
		derivedColumn("Column:dwd.trade_order.discount_amount", dwdTrade, "discount_amount", "decimal(18,2)", 5),
		derivedColumn("Column:dwd.trade_order.net_amount", dwdTrade, "net_amount", "decimal(18,2)", 6),
		derivedColumn("Column:dws.sales_daily.order_date", dwsSales, "order_date", "date", 1),
		derivedColumn("Column:dws.sales_daily.nation_name", dwsSales, "nation_name", "varchar(25)", 2),
		derivedColumn("Column:dws.sales_daily.gmv_amount", dwsSales, "gmv_amount", "decimal(18,2)", 3),
		derivedColumn("Column:dws.sales_daily.order_count", dwsSales, "order_count", "bigint", 4),
	)

	ops = append(ops, taskDefinition(syncOrders, "sync_orders", "mysql-copy", job),
		realisticMember(syncOrders, "inputs", "source-orders", "schema/etl-task.io", map[string]any{"objectId": sourceTableID("orders"), "port": "source"}),
		realisticMember(syncOrders, "outputs", "ods-orders", "schema/etl-task.outputs", map[string]any{"objectId": odsOrders, "mode": "upsert"}),
		realisticMember(syncOrders, "columnMappings", "order-id", "schema/etl-task.column-mappings", map[string]any{
			"outputColumnId": "Column:ods.orders.order_id", "inputColumnIds": []any{sourceColumnID("orders", "o_orderkey")}, "expression": "o_orderkey",
		}),
		realisticMember(syncOrders, "columnMappings", "customer-id", "schema/etl-task.column-mappings", map[string]any{
			"outputColumnId": "Column:ods.orders.customer_id", "inputColumnIds": []any{sourceColumnID("orders", "o_custkey")}, "expression": "o_custkey",
		}),
		realisticMember(syncOrders, "columnMappings", "order-date", "schema/etl-task.column-mappings", map[string]any{
			"outputColumnId": "Column:ods.orders.order_date", "inputColumnIds": []any{sourceColumnID("orders", "o_orderdate")}, "expression": "o_orderdate",
		}),
		taskDefinition(syncLineitem, "sync_lineitem", "mysql-copy", job),
		realisticMember(syncLineitem, "inputs", "source-lineitem", "schema/etl-task.io", map[string]any{"objectId": sourceTableID("lineitem"), "port": "source"}),
		realisticMember(syncLineitem, "outputs", "ods-lineitem", "schema/etl-task.outputs", map[string]any{"objectId": odsLineitem, "mode": "upsert"}),
		realisticMember(syncLineitem, "columnMappings", "order-id", "schema/etl-task.column-mappings", map[string]any{
			"outputColumnId": "Column:ods.lineitem.order_id", "inputColumnIds": []any{sourceColumnID("lineitem", "l_orderkey")}, "expression": "l_orderkey",
		}),
		realisticMember(syncLineitem, "columnMappings", "extended-price", "schema/etl-task.column-mappings", map[string]any{
			"outputColumnId": "Column:ods.lineitem.extended_price", "inputColumnIds": []any{sourceColumnID("lineitem", "l_extendedprice")}, "expression": "l_extendedprice",
		}),
		realisticMember(syncLineitem, "columnMappings", "discount-rate", "schema/etl-task.column-mappings", map[string]any{
			"outputColumnId": "Column:ods.lineitem.discount_rate", "inputColumnIds": []any{sourceColumnID("lineitem", "l_discount")}, "expression": "l_discount",
		}),
		taskDefinition(buildTrade, "build_trade_order", "sql", job),
		realisticMember(buildTrade, "inputs", "ods-orders", "schema/etl-task.io", map[string]any{"objectId": odsOrders, "port": "orders"}),
		realisticMember(buildTrade, "inputs", "ods-lineitem", "schema/etl-task.io", map[string]any{"objectId": odsLineitem, "port": "lineitem"}),
		realisticMember(buildTrade, "inputs", "source-customer", "schema/etl-task.io", map[string]any{"objectId": sourceTableID("customer"), "port": "customer"}),
		realisticMember(buildTrade, "inputs", "source-nation", "schema/etl-task.io", map[string]any{"objectId": sourceTableID("nation"), "port": "nation"}),
		realisticMember(buildTrade, "outputs", "dwd-trade-order", "schema/etl-task.outputs", map[string]any{"objectId": dwdTrade, "mode": "overwrite-partition"}),
		realisticMember(buildTrade, "columnMappings", "discount-amount", "schema/etl-task.column-mappings", map[string]any{
			"outputColumnId": "Column:dwd.trade_order.discount_amount",
			"inputColumnIds": []any{"Column:ods.lineitem.extended_price", "Column:ods.lineitem.discount_rate"},
			"expression":     "l_extendedprice * l_discount", "sqlDigest": "sha256:build-trade-order-v1",
		}),
		realisticMember(buildTrade, "columnMappings", "net-amount", "schema/etl-task.column-mappings", map[string]any{
			"outputColumnId": "Column:dwd.trade_order.net_amount",
			"inputColumnIds": []any{"Column:ods.lineitem.extended_price", "Column:ods.lineitem.discount_rate"},
			"expression":     "l_extendedprice * (1 - l_discount)", "sqlDigest": "sha256:build-trade-order-v1",
		}),
		taskDefinition(aggregate, "aggregate_sales_daily", "sql", job),
		realisticMember(aggregate, "inputs", "dwd-trade-order", "schema/etl-task.io", map[string]any{"objectId": dwdTrade, "port": "trade_order"}),
		realisticMember(aggregate, "outputs", "dws-sales-daily", "schema/etl-task.outputs", map[string]any{"objectId": dwsSales, "mode": "overwrite-partition"}),
		realisticMember(aggregate, "columnMappings", "gmv-amount", "schema/etl-task.column-mappings", map[string]any{
			"outputColumnId": "Column:dws.sales_daily.gmv_amount",
			"inputColumnIds": []any{"Column:dwd.trade_order.net_amount"},
			"expression":     "sum(net_amount)", "sqlDigest": "sha256:aggregate-sales-daily-v1",
		}),
		realisticAspect(aggregate, "runtimeSummary", "schema/etl-task.runtime-summary", map[string]any{
			"latestRunId": "run-aggregate-sales-daily-20260824", "status": "SUCCEEDED",
			"watermark": "1998-08-02", "finishedAt": "2026-08-24T02:25:00+08:00",
		}),
		realisticAspect(dwsSales, "freshness", "schema/table.freshness", map[string]any{
			"watermark": "1998-08-02", "updatedAt": "2026-08-24T02:25:00+08:00", "status": "FRESH",
		}),
	)
	return ops
}

func governanceKnowledge() []repository.Operation {
	return []repository.Operation{
		realisticMember("Table:dws.sales_daily", "permissions", "finance-analysts-select", "schema/table.permissions", map[string]any{
			"principal": "group:finance-analysts", "privileges": []any{"SELECT"},
			"enforcedBy": "ranger", "capturedAt": "2026-08-24T03:00:00+08:00",
		}),
		realisticMember("Table:dwd.trade_order", "permissions", "data-engineering-all", "schema/table.permissions", map[string]any{
			"principal": "group:data-engineering", "privileges": []any{"SELECT", "INSERT", "ALTER"},
			"enforcedBy": "ranger", "capturedAt": "2026-08-24T03:00:00+08:00",
		}),
		realisticAspect(sourceColumnID("customer", "c_name"), "classification", "schema/column.classification", map[string]any{
			"classes": []any{"PII", "CUSTOMER_NAME"}, "policyRef": "ranger-policy://mask-customer-name",
		}),
		realisticAspect(sourceColumnID("customer", "c_phone"), "classification", "schema/column.classification", map[string]any{
			"classes": []any{"PII", "PHONE"}, "policyRef": "ranger-policy://mask-phone",
		}),
		realisticAspect("QualityRule:dwd-trade-order-id-unique", "definition", "schema/quality-rule.definition", map[string]any{
			"name": "trade_order_id_unique", "assertion": "duplicate_count(order_id) = 0", "severity": "ERROR",
		}),
		realisticMember("QualityRule:dwd-trade-order-id-unique", "qualityTargets", "dwd-trade-order", "schema/quality-rule.targets", map[string]any{
			"objectId": "Table:dwd.trade_order",
		}),
		realisticAspect("QualityRule:dws-sales-daily-fresh", "definition", "schema/quality-rule.definition", map[string]any{
			"name": "sales_daily_fresh", "assertion": "watermark_age < 36h", "severity": "ERROR",
		}),
		realisticMember("QualityRule:dws-sales-daily-fresh", "qualityTargets", "dws-sales-daily", "schema/quality-rule.targets", map[string]any{
			"objectId": "Table:dws.sales_daily",
		}),
	}
}

func semanticKnowledge() []repository.Operation {
	return []repository.Operation{
		realisticAspect("MetricView:sales", "definition", "schema/metric-view.definition", map[string]any{
			"name": "sales", "baseTableId": "Table:dws.sales_daily", "defaultTimeDimensionId": "Dimension:order-date",
		}),
		realisticAspect("MetricView:sales", "ownership", "schema/semantic.ownership", map[string]any{"owner": "group:finance-data"}),
		realisticAspect("Dimension:order-date", "definition", "schema/dimension.definition", map[string]any{
			"name": "order_date", "role": "time", "dataType": "date", "grain": "day",
		}),
		realisticMember("Dimension:order-date", "dependencies", "source-column", "schema/semantic.dependencies", map[string]any{
			"objectId": "Column:dws.sales_daily.order_date", "relation": "reads",
		}),
		realisticAspect("Dimension:nation", "definition", "schema/dimension.definition", map[string]any{
			"name": "nation", "role": "categorical", "dataType": "string",
		}),
		realisticMember("Dimension:nation", "dependencies", "source-column", "schema/semantic.dependencies", map[string]any{
			"objectId": "Column:dws.sales_daily.nation_name", "relation": "reads",
		}),
		realisticAspect("Measure:gross-sales", "definition", "schema/measure.definition", map[string]any{
			"name": "gross_sales", "expression": "gmv_amount", "aggregation": "sum", "unit": "CNY",
		}),
		realisticMember("Measure:gross-sales", "dependencies", "source-column", "schema/semantic.dependencies", map[string]any{
			"objectId": "Column:dws.sales_daily.gmv_amount", "relation": "reads",
		}),
		realisticAspect("Measure:order-count", "definition", "schema/measure.definition", map[string]any{
			"name": "order_count", "expression": "order_count", "aggregation": "sum", "unit": "count",
		}),
		realisticMember("Measure:order-count", "dependencies", "source-column", "schema/semantic.dependencies", map[string]any{
			"objectId": "Column:dws.sales_daily.order_count", "relation": "reads",
		}),
		realisticAspect("Metric:gmv", "definition", "schema/metric.definition", map[string]any{
			"name": "GMV", "description": "Paid merchandise value after line discount; excludes refunds recognized within seven days",
			"formula": "sum(gross_sales)", "grain": "day,nation", "timePolicy": "Asia/Shanghai business day",
		}),
		realisticMember("Metric:gmv", "dependencies", "measure-gross-sales", "schema/semantic.dependencies", map[string]any{
			"objectId": "Measure:gross-sales", "relation": "uses-measure",
		}),
		realisticMember("Metric:gmv", "dependencies", "metric-view-sales", "schema/semantic.dependencies", map[string]any{
			"objectId": "MetricView:sales", "relation": "served-by",
		}),
		realisticMember("Metric:gmv", "dependencies", "dimension-order-date", "schema/semantic.dependencies", map[string]any{
			"objectId": "Dimension:order-date", "relation": "grouped-by",
		}),
		realisticMember("Metric:gmv", "dependencies", "dimension-nation", "schema/semantic.dependencies", map[string]any{
			"objectId": "Dimension:nation", "relation": "grouped-by",
		}),
		realisticAspect("Metric:gmv", "ownership", "schema/semantic.ownership", map[string]any{"owner": "group:finance-data"}),
		realisticAspect("Metric:gmv", "certification", "schema/semantic.certification", map[string]any{
			"status": "CERTIFIED", "approvedBy": "role:finance-steward", "approvedAt": "2026-08-24T04:00:00+08:00",
		}),
	}
}

func assertRealisticEntityInventory(t *testing.T, listed []reader.FederatedValue) {
	t.Helper()
	if got := sourceTableID("lineitem"); got != "dw-table-55f581f489d7f19a0703c7ec" {
		t.Fatalf("lineitem identity drifted from DW-01: %s", got)
	}
	if got := sourceColumnID("lineitem", "l_discount"); got != "dw-column-b306fe3e8beeebb056a4d916" {
		t.Fatalf("l_discount identity drifted from DW-01: %s", got)
	}
	counts := map[kernel.ObjectID]int{}
	for _, value := range listed {
		counts[value.ObjectID]++
	}
	ids := []kernel.ObjectID{
		"DataSource:mysql-tpch", "ResourceDescriptor:mysql-tpch", "Database:tpch", "Schema:tpch", sourceTableID("orders"), sourceColumnID("lineitem", "l_discount"),
		"DataSource:starrocks-warehouse", "ResourceDescriptor:starrocks-warehouse", "Database:warehouse", "Schema:dwd", "Table:dwd.trade_order", "Column:dws.sales_daily.gmv_amount",
		"ETLJob:tpch-daily", "ETLTask:build-trade-order", "ETLTask:aggregate-sales-daily",
		"MetricView:sales", "Dimension:order-date", "Dimension:nation", "Measure:gross-sales", "Metric:gmv",
		"QualityRule:dwd-trade-order-id-unique",
	}
	for _, id := range ids {
		if counts[id] != 1 {
			t.Fatalf("entity %s must resolve once in Workspace list, got %d", id, counts[id])
		}
	}
}

func assertRealisticReferenceIntegrity(t *testing.T, listed []reader.FederatedValue) int {
	t.Helper()
	counts := map[kernel.ObjectID]int{}
	for _, value := range listed {
		counts[value.ObjectID]++
	}
	referenceKeys := map[string]bool{
		"resourceRef": true, "sourceId": true, "databaseId": true, "schemaId": true, "tableId": true,
		"jobId": true, "taskId": true, "objectId": true, "parentObjectId": true,
		"baseTableId": true, "defaultTimeDimensionId": true, "outputColumnId": true, "inputColumnIds": true,
	}
	targets := map[kernel.ObjectID]bool{}
	var walk func(any)
	walk = func(value any) {
		switch node := value.(type) {
		case map[string]any:
			for key, child := range node {
				if referenceKeys[key] {
					switch ref := child.(type) {
					case string:
						targets[kernel.ObjectID(ref)] = true
					case []any:
						for _, item := range ref {
							if id, ok := item.(string); ok {
								targets[kernel.ObjectID(id)] = true
							}
						}
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range node {
				walk(child)
			}
		}
	}
	for _, value := range listed {
		walk(value.Value)
	}
	for target := range targets {
		if counts[target] != 1 {
			t.Fatalf("referenced object %s must resolve once in Workspace list, got %d", target, counts[target])
		}
	}
	if len(targets) < 30 {
		t.Fatalf("fixture is too weak: only %d distinct relationship targets", len(targets))
	}
	return len(targets)
}

func assertGMVLineage(t *testing.T, serving *reader.Serving) []string {
	t.Helper()
	gmv := realisticRead(t, serving, "Metric:gmv")
	if nestedString(gmv, "certification", "status") != "CERTIFIED" {
		t.Fatalf("GMV certification: %#v", gmv)
	}
	measure := realisticMemberObjectID(t, gmv, "dependencies", "measure-gross-sales")
	if measure != "Measure:gross-sales" {
		t.Fatalf("GMV measure=%s", measure)
	}
	measureValue := realisticRead(t, serving, measure)
	warehouseColumn := realisticMemberObjectID(t, measureValue, "dependencies", "source-column")
	if warehouseColumn != "Column:dws.sales_daily.gmv_amount" {
		t.Fatalf("measure source=%s", warehouseColumn)
	}
	columnValue := realisticRead(t, serving, warehouseColumn)
	warehouseTable := kernel.ObjectID(nestedString(columnValue, "structure", "tableId"))
	if warehouseTable != "Table:dws.sales_daily" {
		t.Fatalf("warehouse table=%s", warehouseTable)
	}
	metricViewID := realisticMemberObjectID(t, gmv, "dependencies", "metric-view-sales")
	metricView := realisticRead(t, serving, metricViewID)
	if nestedString(metricView, "definition", "baseTableId") != string(warehouseTable) {
		t.Fatalf("metric view does not bind GMV's warehouse table: %#v", metricView)
	}

	aggregate := realisticRead(t, serving, "ETLTask:aggregate-sales-daily")
	if got := realisticMemberObjectID(t, aggregate, "outputs", "dws-sales-daily"); got != warehouseTable {
		t.Fatalf("aggregate output=%s", got)
	}
	dwdTable := realisticMemberObjectID(t, aggregate, "inputs", "dwd-trade-order")
	if dwdTable != "Table:dwd.trade_order" {
		t.Fatalf("aggregate input=%s", dwdTable)
	}
	aggregateMapping := realisticMemberMap(t, aggregate, "columnMappings", "gmv-amount")
	if aggregateMapping["outputColumnId"] != string(warehouseColumn) {
		t.Fatalf("aggregate column output: %#v", aggregateMapping)
	}
	if got := realisticStringSlice(t, aggregateMapping["inputColumnIds"]); len(got) != 1 || got[0] != "Column:dwd.trade_order.net_amount" {
		t.Fatalf("aggregate column inputs: %#v", aggregateMapping)
	}
	build := realisticRead(t, serving, "ETLTask:build-trade-order")
	if got := realisticMemberObjectID(t, build, "outputs", "dwd-trade-order"); got != dwdTable {
		t.Fatalf("build output=%s", got)
	}
	mapping := realisticMemberMap(t, build, "columnMappings", "net-amount")
	if mapping["outputColumnId"] != "Column:dwd.trade_order.net_amount" {
		t.Fatalf("build column output: %#v", mapping)
	}
	inputs := realisticStringSlice(t, mapping["inputColumnIds"])
	wantInputs := map[string]bool{
		"Column:ods.lineitem.extended_price": false,
		"Column:ods.lineitem.discount_rate":  false,
	}
	for _, input := range inputs {
		if _, ok := wantInputs[input]; ok {
			wantInputs[input] = true
		}
		if values, err := serving.Read(kernel.ObjectID(input), nil); err != nil || len(values) != 1 {
			t.Fatalf("lineage input %s does not resolve: %#v %v", input, values, err)
		}
	}
	for input, found := range wantInputs {
		if !found {
			t.Fatalf("column mapping missed %s: %#v", input, inputs)
		}
	}
	sync := realisticRead(t, serving, "ETLTask:sync-lineitem")
	sourceInputs := map[string]bool{
		string(sourceColumnID("lineitem", "l_extendedprice")): false,
		string(sourceColumnID("lineitem", "l_discount")):      false,
	}
	for memberKey, output := range map[string]string{
		"extended-price": "Column:ods.lineitem.extended_price",
		"discount-rate":  "Column:ods.lineitem.discount_rate",
	} {
		syncMapping := realisticMemberMap(t, sync, "columnMappings", memberKey)
		if syncMapping["outputColumnId"] != output {
			t.Fatalf("sync column output %s: %#v", memberKey, syncMapping)
		}
		inputIDs := realisticStringSlice(t, syncMapping["inputColumnIds"])
		if len(inputIDs) != 1 {
			t.Fatalf("sync column inputs %s: %#v", memberKey, syncMapping)
		}
		sourceInputs[inputIDs[0]] = true
	}
	for input, found := range sourceInputs {
		if !found {
			t.Fatalf("sync lineage missed %s", input)
		}
	}
	return []string{
		"Metric:gmv", "MetricView:sales", "Measure:gross-sales", "Column:dws.sales_daily.gmv_amount", "Table:dws.sales_daily",
		"ETLTask:aggregate-sales-daily", "Column:dwd.trade_order.net_amount", "Table:dwd.trade_order", "ETLTask:build-trade-order",
		"Column:ods.lineitem.extended_price", "Column:ods.lineitem.discount_rate", "ETLTask:sync-lineitem",
		string(sourceColumnID("lineitem", "l_extendedprice")), string(sourceColumnID("lineitem", "l_discount")),
	}
}

func assertJoinEvidenceIsNotLineage(t *testing.T, serving *reader.Serving) {
	t.Helper()
	lineitem := realisticRead(t, serving, sourceTableID("lineitem"))
	join := realisticMemberMap(t, lineitem, "joinEvidence", "lineitem-orders")
	if join["purpose"] != "joinability" || join["parentObjectId"] != string(sourceTableID("orders")) {
		t.Fatalf("join evidence: %#v", join)
	}
	if _, exists := join["producingTaskId"]; exists {
		t.Fatalf("join evidence must not pretend to be production lineage: %#v", join)
	}
	build := realisticRead(t, serving, "ETLTask:build-trade-order")
	if _, ok := realisticAspectMap(t, build, "inputs")["ods-lineitem"]; !ok {
		t.Fatalf("production lineage must remain on ETLTask inputs: %#v", build)
	}
}

func assertPermissionBoundary(t *testing.T, serving *reader.Serving) {
	t.Helper()
	table := realisticRead(t, serving, "Table:dws.sales_daily")
	grant := realisticMemberMap(t, table, "permissions", "finance-analysts-select")
	if grant["principal"] != "group:finance-analysts" || realisticStringSlice(t, grant["privileges"])[0] != "SELECT" {
		t.Fatalf("source permission snapshot: %#v", grant)
	}
	if allowOK("finance-analyst", "read-workspace", "", realisticCatalog, realisticWorkspace) {
		t.Fatal("a source-system permissions Aspect must not create kc allow")
	}
	filtered, err := serving.Read("Table:dws.sales_daily", &repository.AspectSelector{Exclude: []string{"permissions"}})
	if err != nil || len(filtered) != 1 {
		t.Fatalf("filtered read: %#v %v", filtered, err)
	}
	if _, ok := filtered[0].Value.(map[string]any)["permissions"]; ok {
		t.Fatalf("permissions selector did not remove source grants: %#v", filtered[0].Value)
	}
}

func assertRuntimeBinding(t *testing.T, r *reader.Reader, commit kernel.CommitID) {
	t.Helper()
	binding, err := r.ResolveBinding(realisticPhysical, commit, realisticRunsAddress())
	if err != nil || binding.DeclarationCommit != commit || binding.Mode != repository.BindingStream ||
		binding.Runtime != "warehouse-runtime" || binding.Protocol != "resource.v1" || binding.Operations["window"].Call != "etl.runs.window" {
		t.Fatalf("ETL run binding: %#v %v", binding, err)
	}
}

func assertRealisticProvenance(t *testing.T, r *reader.Reader, physicalCommit, semanticCommit kernel.CommitID) {
	t.Helper()
	permission, err := r.ReadAddress(realisticPhysical, kernel.Address{
		Kind: kernel.KindMember, ObjectID: "Table:dws.sales_daily", AspectName: "permissions", MemberKey: "finance-analysts-select",
	}, physicalCommit)
	if err != nil || permission.Provenance == nil || permission.Provenance.OriginKind != kernel.OriginSource ||
		permission.Provenance.SourceRefs[0] != "ranger://warehouse/policy-snapshot-2026-08-24" {
		t.Fatalf("permission provenance: %#v %v", permission, err)
	}
	metric, err := r.ReadAddress(realisticSemantic, kernel.Address{
		Kind: kernel.KindAspect, ObjectID: "Metric:gmv", AspectName: "definition",
	}, semanticCommit)
	if err != nil || metric.Provenance == nil || metric.Provenance.OriginKind != kernel.OriginDefinition || metric.Provenance.ActorRef != "finance-steward" {
		t.Fatalf("metric provenance: %#v %v", metric, err)
	}
}

func realisticSchema(objectID, entity, aspect, pattern string) repository.Operation {
	return repository.Operation{
		Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: kernel.ObjectID(objectID)},
		Value: map[string]any{
			"entity": entity, "aspect": aspect, "pattern": pattern,
			"fields": map[string]any{},
		},
	}
}

func realisticAspect(objectID kernel.ObjectID, aspect, schemaRef string, value map[string]any) repository.Operation {
	return repository.Operation{
		Op:        repository.OpPut,
		Address:   kernel.Address{Kind: kernel.KindAspect, ObjectID: objectID, AspectName: aspect},
		SchemaRef: schemaRef, Value: value,
	}
}

func realisticMember(objectID kernel.ObjectID, aspect, memberKey, schemaRef string, value map[string]any) repository.Operation {
	return repository.Operation{
		Op:        repository.OpPut,
		Address:   kernel.Address{Kind: kernel.KindMember, ObjectID: objectID, AspectName: aspect, MemberKey: memberKey},
		SchemaRef: schemaRef, Value: value,
	}
}

func sourceTable(objectID kernel.ObjectID, name string, rows int) repository.Operation {
	return realisticAspect(objectID, "structure", "schema/table.structure", map[string]any{
		"name": name, "qualifiedName": "tpch." + name, "schemaId": "Schema:tpch", "layer": "source",
		"tableType": "BASE_TABLE", "rowCount": rows, "sourceKey": realisticMySQL + "/table/tpch/" + name,
	})
}

func sourceColumn(table, name, dataType string, ordinal int) repository.Operation {
	return realisticAspect(sourceColumnID(table, name), "structure", "schema/column.structure", map[string]any{
		"name": name, "qualifiedName": "tpch." + table + "." + name, "tableId": sourceTableID(table),
		"dataType": dataType, "ordinal": ordinal, "sourceKey": realisticMySQL + "/column/tpch/" + table + "/" + name,
	})
}

func derivedColumn(objectID kernel.ObjectID, tableID kernel.ObjectID, name, dataType string, ordinal int) repository.Operation {
	return realisticAspect(objectID, "structure", "schema/column.structure", map[string]any{
		"name": name, "tableId": tableID, "dataType": dataType, "ordinal": ordinal,
	})
}

func taskDefinition(objectID kernel.ObjectID, name, taskType string, jobID kernel.ObjectID) repository.Operation {
	return realisticAspect(objectID, "definition", "schema/etl-task.definition", map[string]any{
		"name": name, "taskType": taskType, "jobId": jobID, "engine": "airflow+sql",
	})
}

func sourceTableID(table string) kernel.ObjectID {
	return realisticSourceObjectID("table", realisticMySQL+"/table/tpch/"+table)
}

func sourceColumnID(table, column string) kernel.ObjectID {
	return realisticSourceObjectID("column", realisticMySQL+"/column/tpch/"+table+"/"+column)
}

func realisticSourceObjectID(kind, sourceKey string) kernel.ObjectID {
	digest := sha256.Sum256([]byte(sourceKey))
	return kernel.ObjectID("dw-" + kind + "-" + hex.EncodeToString(digest[:12]))
}

func realisticRunsAddress() kernel.Address {
	return kernel.Address{Kind: kernel.KindAspect, ObjectID: "ETLJob:tpch-daily", AspectName: "runEvents"}
}

func realisticRunsBinding() repository.Operation {
	return repository.Operation{
		Op:      repository.OpPut,
		Address: realisticRunsAddress(),
		ValueSource: &repository.ValueSource{Kind: repository.ValueSourceBinding, Binding: &repository.BindingDeclaration{
			Mode: repository.BindingStream, Runtime: "warehouse-runtime", Protocol: "resource.v1",
			Operations: map[string]repository.BindingOperation{"window": {Call: "etl.runs.window"}},
		}},
	}
}

func realisticCommit(t *testing.T, w *writer.Writer, commandID string, repo kernel.RepositoryID, ops []repository.Operation, provenance *kernel.ProvenanceEnvelope) kernel.CommitID {
	t.Helper()
	receipt, err := w.CommitIntent(commandID, writer.CommitIntent{
		TargetRepository: repo, TargetRef: MainRef, Operations: ops, Message: commandID, Provenance: provenance,
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Disposition != writer.DispositionApplied {
		t.Fatalf("%s disposition %s", commandID, receipt.Disposition)
	}
	return receipt.Result.CommitID
}

func realisticRead(t *testing.T, serving *reader.Serving, objectID kernel.ObjectID) map[string]any {
	t.Helper()
	values, err := serving.Read(objectID, nil)
	if err != nil || len(values) != 1 {
		t.Fatalf("read %s: %#v %v", objectID, values, err)
	}
	value, ok := values[0].Value.(map[string]any)
	if !ok {
		t.Fatalf("read %s is not an assembled object: %#v", objectID, values[0].Value)
	}
	return value
}

func realisticAspectMap(t *testing.T, value map[string]any, aspect string) map[string]any {
	t.Helper()
	got, ok := value[aspect].(map[string]any)
	if !ok {
		t.Fatalf("aspect %s is not a map: %#v", aspect, value[aspect])
	}
	return got
}

func realisticMemberMap(t *testing.T, value map[string]any, aspect, memberKey string) map[string]any {
	t.Helper()
	members := realisticAspectMap(t, value, aspect)
	got, ok := members[memberKey].(map[string]any)
	if !ok {
		t.Fatalf("member %s/%s is not a map: %#v", aspect, memberKey, members[memberKey])
	}
	return got
}

func realisticMemberObjectID(t *testing.T, value map[string]any, aspect, memberKey string) kernel.ObjectID {
	t.Helper()
	member := realisticMemberMap(t, value, aspect, memberKey)
	id, ok := member["objectId"].(string)
	if !ok || id == "" {
		t.Fatalf("member %s/%s has no objectId: %#v", aspect, memberKey, member)
	}
	return kernel.ObjectID(id)
}

func realisticStringSlice(t *testing.T, value any) []string {
	t.Helper()
	raw, ok := value.([]any)
	if !ok {
		t.Fatalf("value is not an array: %#v", value)
	}
	out := make([]string, len(raw))
	for i, item := range raw {
		var ok bool
		out[i], ok = item.(string)
		if !ok {
			t.Fatalf("array item is not a string: %#v", item)
		}
	}
	return out
}

func writeRealisticEvidence(t *testing.T, evidence map[string]any) {
	t.Helper()
	path := os.Getenv("KC_REALISTIC_WAREHOUSE_REPORT")
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("validation evidence: %s", path)
}
