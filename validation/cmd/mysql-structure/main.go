package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kc/connector"
	"kc/kernel"
)

const (
	connectorID = "mysql-tpch-structure"
	targetRepo  = "kr://tpch/public/physical"
)

type sourceSnapshot struct {
	CapturedAt string         `json:"capturedAt"`
	SourceRef  string         `json:"sourceRef"`
	Tables     []sourceTable  `json:"tables"`
	Columns    []sourceColumn `json:"columns"`
}

type sourceTable struct {
	Schema    string `json:"tableSchema"`
	Name      string `json:"tableName"`
	Type      string `json:"tableType"`
	Engine    string `json:"engine"`
	Comment   string `json:"tableComment"`
	Collation string `json:"tableCollation"`
}

type sourceColumn struct {
	Schema                 string `json:"tableSchema"`
	Table                  string `json:"tableName"`
	Name                   string `json:"columnName"`
	Ordinal                int64  `json:"ordinalPosition"`
	Default                any    `json:"columnDefault"`
	Nullable               string `json:"isNullable"`
	DataType               string `json:"dataType"`
	ColumnType             string `json:"columnType"`
	CharacterMaximumLength *int64 `json:"characterMaximumLength"`
	NumericPrecision       *int64 `json:"numericPrecision"`
	NumericScale           *int64 `json:"numericScale"`
	ColumnKey              string `json:"columnKey"`
	Extra                  string `json:"extra"`
	Comment                string `json:"columnComment"`
}

type identityMap struct {
	Version int               `json:"version"`
	Objects map[string]string `json:"objects"`
}

func main() {
	var snapshotPath, mappingPath, observedPath, outputPath, baseCommit, producedAt string
	flag.StringVar(&snapshotPath, "snapshot", "", "MySQL metadata snapshot JSON")
	flag.StringVar(&mappingPath, "mapping", "", "connector-private source-key mapping JSON")
	flag.StringVar(&observedPath, "observed", "", "observed Address digests JSON")
	flag.StringVar(&outputPath, "out", "", "preview output JSON")
	flag.StringVar(&baseCommit, "base", "", "target repository base commit")
	flag.StringVar(&producedAt, "produced-at", "", "SOURCE envelope timestamp")
	flag.Parse()
	if snapshotPath == "" || mappingPath == "" || outputPath == "" {
		fatalf("--snapshot, --mapping, and --out are required")
	}

	var snapshot sourceSnapshot
	readJSON(snapshotPath, &snapshot)
	mapping := loadMapping(mappingPath)
	desired, err := translate(snapshot, &mapping)
	if err != nil {
		fatalf("translate source snapshot: %v", err)
	}
	var observed []connector.Observed
	if observedPath != "" {
		readJSON(observedPath, &observed)
	}
	if producedAt == "" {
		producedAt = snapshot.CapturedAt
	}
	preview, err := connector.Preview(connector.Plan{
		ConnectorID:      connectorID,
		Mode:             connector.ModeReconcile,
		Scope:            connector.Scope{Aspects: []string{"structure"}},
		TargetRepository: targetRepo,
		BaseCommit:       kernel.CommitID(baseCommit),
		Desired:          desired,
		Observed:         observed,
		SourceRefs:       []string{snapshot.SourceRef},
		ProducedAt:       producedAt,
		Message:          "mirror MySQL tpch structure",
	})
	if err != nil {
		fatalf("preview: %v", err)
	}
	writeJSON(mappingPath, mapping)
	writeJSON(outputPath, preview)
}

func translate(snapshot sourceSnapshot, mapping *identityMap) ([]connector.Unit, error) {
	if strings.TrimSpace(snapshot.SourceRef) == "" {
		return nil, fmt.Errorf("sourceRef is required")
	}
	if len(snapshot.Tables) == 0 || len(snapshot.Columns) == 0 {
		return nil, fmt.Errorf("source snapshot must contain tables and columns")
	}
	sort.Slice(snapshot.Tables, func(i, j int) bool {
		return snapshot.Tables[i].Schema+"."+snapshot.Tables[i].Name < snapshot.Tables[j].Schema+"."+snapshot.Tables[j].Name
	})
	sort.Slice(snapshot.Columns, func(i, j int) bool {
		a, b := snapshot.Columns[i], snapshot.Columns[j]
		if a.Schema != b.Schema {
			return a.Schema < b.Schema
		}
		if a.Table != b.Table {
			return a.Table < b.Table
		}
		return a.Ordinal < b.Ordinal
	})

	columnCounts := map[string]int{}
	for _, column := range snapshot.Columns {
		columnCounts[tableKey(snapshot.SourceRef, column.Schema, column.Table)]++
	}
	units := make([]connector.Unit, 0, len(snapshot.Tables)+len(snapshot.Columns))
	tableIDs := map[string]string{}
	seenAddress := map[string]struct{}{}
	for _, table := range snapshot.Tables {
		key := tableKey(snapshot.SourceRef, table.Schema, table.Name)
		id := mapping.objectID("table", key)
		tableIDs[key] = id
		unit := connector.Unit{
			Address:   structureAddress(id),
			SourceKey: connector.SourceKey(key),
			PathHint:  "physical/tables/" + id + ".json",
			Value: map[string]any{
				"entityType": "table", "sourceKey": key,
				"schema": table.Schema, "name": table.Name,
				"qualifiedName": table.Schema + "." + table.Name,
				"tableType":     table.Type, "engine": table.Engine,
				"comment": table.Comment, "collation": table.Collation,
				"columnCount": columnCounts[key],
			},
		}
		if err := appendUnique(&units, seenAddress, unit); err != nil {
			return nil, err
		}
	}
	for _, column := range snapshot.Columns {
		parentKey := tableKey(snapshot.SourceRef, column.Schema, column.Table)
		parentID, ok := tableIDs[parentKey]
		if !ok {
			return nil, fmt.Errorf("column %s.%s.%s has no table record", column.Schema, column.Table, column.Name)
		}
		key := columnKey(snapshot.SourceRef, column.Schema, column.Table, column.Name)
		id := mapping.objectID("column", key)
		unit := connector.Unit{
			Address:   structureAddress(id),
			SourceKey: connector.SourceKey(key),
			PathHint:  "physical/columns/" + id + ".json",
			Value: map[string]any{
				"entityType": "column", "sourceKey": key,
				"parentObjectId": parentID, "schema": column.Schema,
				"tableName": column.Table, "name": column.Name,
				"qualifiedName":   column.Schema + "." + column.Table + "." + column.Name,
				"ordinalPosition": column.Ordinal, "nullable": column.Nullable == "YES",
				"dataType": column.DataType, "columnType": column.ColumnType,
				"columnDefault":          column.Default,
				"characterMaximumLength": column.CharacterMaximumLength,
				"numericPrecision":       column.NumericPrecision, "numericScale": column.NumericScale,
				"columnKey": column.ColumnKey, "extra": column.Extra, "comment": column.Comment,
			},
		}
		if err := appendUnique(&units, seenAddress, unit); err != nil {
			return nil, err
		}
	}
	return units, nil
}

func appendUnique(units *[]connector.Unit, seen map[string]struct{}, unit connector.Unit) error {
	key := kernel.AddressKey(unit.Address)
	if _, ok := seen[key]; ok {
		return fmt.Errorf("duplicate translated Address %s", key)
	}
	seen[key] = struct{}{}
	*units = append(*units, unit)
	return nil
}

func structureAddress(id string) kernel.Address {
	return kernel.Address{Kind: kernel.KindAspect, ObjectID: kernel.ObjectID(id), AspectName: "structure"}
}

func tableKey(sourceRef, schema, table string) string {
	return strings.TrimSuffix(sourceRef, "/") + "/table/" + schema + "/" + table
}

func columnKey(sourceRef, schema, table, column string) string {
	return strings.TrimSuffix(sourceRef, "/") + "/column/" + schema + "/" + table + "/" + column
}

func (m *identityMap) objectID(kind, sourceKey string) string {
	if id := m.Objects[sourceKey]; id != "" {
		return id
	}
	digest := sha256.Sum256([]byte(sourceKey))
	id := "dw-" + kind + "-" + hex.EncodeToString(digest[:12])
	m.Objects[sourceKey] = id
	return id
}

func loadMapping(path string) identityMap {
	mapping := identityMap{Version: 1, Objects: map[string]string{}}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return mapping
	}
	readJSON(path, &mapping)
	if mapping.Version != 1 || mapping.Objects == nil {
		fatalf("unsupported or corrupt identity mapping %s", path)
	}
	return mapping
}

func readJSON(path string, target any) {
	file, err := os.Open(path)
	if err != nil {
		fatalf("open %s: %v", path, err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		fatalf("decode %s: %v", path, err)
	}
}

func writeJSON(path string, value any) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fatalf("create parent for %s: %v", path, err)
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fatalf("encode %s: %v", path, err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o644); err != nil {
		fatalf("write %s: %v", path, err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "mysql-structure: "+format+"\n", args...)
	os.Exit(1)
}
