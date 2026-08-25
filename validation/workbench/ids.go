package scenario

import "kc/kernel"

// Company workbench identities. Repos split by who can be responsible, not by folder.
const (
	CatalogID = "kr://acme/catalog"
	MainRef   = "refs/heads/main"

	ViewBoard = "analyst-board"
	ViewDesk  = "kai-desk"
)

const (
	TableTrade   kernel.ObjectID = "Table:dwd.trade_order"
	MetricGMV    kernel.ObjectID = "Metric:gmv"
	ExampleGMV   kernel.ObjectID = "Example:gmv-refund"
	HabitMorning kernel.ObjectID = "Habit:morning-review"
	DistErrors   kernel.ObjectID = "Dist:error-by-topic"
	DraftScratch kernel.ObjectID = "Draft:scratch"
)

const (
	SchemaTableStruct = "schema/table.structure"
	SchemaTableOwner  = "schema/table.ownership"
	SchemaMetricDef   = "schema/metric.definition"
	SchemaExampleBody = "schema/example.body"
	SchemaHabitNote   = "schema/habit.note"
	SchemaDistStats   = "schema/dist.stats"
)

const (
	GMVCompany  = "GMV 不含 7 日内退货"
	GMVPersonal = "GMV 含税，个人草稿"
)

var (
	Metadata  kernel.RepositoryID = "kr://acme/public/metadata"
	Semantics kernel.RepositoryID = "kr://acme/org/semantics"
	Personal  kernel.RepositoryID = "kr://acme/personals/kai"
	Unknown   kernel.RepositoryID = "kr://acme/unknown/ghost"
)

func companySources() []struct {
	Repository kernel.RepositoryID
	Selector   string
} {
	return []struct {
		Repository kernel.RepositoryID
		Selector   string
	}{
		{Repository: Metadata, Selector: MainRef},
		{Repository: Semantics, Selector: MainRef},
	}
}
