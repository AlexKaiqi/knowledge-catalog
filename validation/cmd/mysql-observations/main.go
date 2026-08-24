package main

import (
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
	connectorID = "mysql-tpch-observations"
	targetRepo  = "kr://tpch/public/physical"
)

type observationSnapshot struct {
	CapturedAt     string         `json:"capturedAt"`
	SourceRef      string         `json:"sourceRef"`
	ProvenanceRefs []string       `json:"provenanceRefs,omitempty"`
	Profile        columnProfile  `json:"profile"`
	Joins          []joinEvidence `json:"joins"`
	Annotation     annotation     `json:"annotation"`
}

type columnProfile struct {
	Schema       string              `json:"schema"`
	Table        string              `json:"table"`
	Column       string              `json:"column"`
	RowCount     int64               `json:"rowCount"`
	NDV          int64               `json:"ndv"`
	MinValue     string              `json:"minValue"`
	MaxValue     string              `json:"maxValue"`
	AvgValue     string              `json:"avgValue"`
	Distribution []distributionEntry `json:"distribution"`
}

type distributionEntry struct {
	Value    string `json:"value"`
	RowCount int64  `json:"rowCount"`
}

type joinEvidence struct {
	Relation      string   `json:"relation"`
	ChildSchema   string   `json:"childSchema"`
	ChildTable    string   `json:"childTable"`
	ChildColumns  []string `json:"childColumns"`
	ParentSchema  string   `json:"parentSchema"`
	ParentTable   string   `json:"parentTable"`
	ParentColumns []string `json:"parentColumns"`
	ChildRowCount int64    `json:"childRowCount"`
	OrphanCount   int64    `json:"orphanCount"`
	EvidenceSQL   string   `json:"evidenceSql"`
}

type annotation struct {
	Schema         string `json:"schema"`
	Table          string `json:"table"`
	Column         string `json:"column"`
	Comment        string `json:"comment"`
	SourceFragment string `json:"sourceFragment"`
}

type identityMap struct {
	Version int               `json:"version"`
	Objects map[string]string `json:"objects"`
}

func main() {
	var snapshotPath, mappingPath, observedPath, outputPath, baseCommit, producedAt string
	flag.StringVar(&snapshotPath, "snapshot", "", "observation snapshot JSON")
	flag.StringVar(&mappingPath, "mapping", "", "DW-01 source-key mapping JSON")
	flag.StringVar(&observedPath, "observed", "", "observed Address digests JSON")
	flag.StringVar(&outputPath, "out", "", "preview output JSON")
	flag.StringVar(&baseCommit, "base", "", "target repository base commit")
	flag.StringVar(&producedAt, "produced-at", "", "SOURCE envelope timestamp")
	flag.Parse()
	if snapshotPath == "" || mappingPath == "" || outputPath == "" {
		fatalf("--snapshot, --mapping, and --out are required")
	}
	var snapshot observationSnapshot
	var mapping identityMap
	readJSON(snapshotPath, &snapshot)
	readJSON(mappingPath, &mapping)
	units, err := translate(snapshot, mapping)
	if err != nil {
		fatalf("translate: %v", err)
	}
	var observed []connector.Observed
	if observedPath != "" {
		readJSON(observedPath, &observed)
	}
	if producedAt == "" {
		producedAt = snapshot.CapturedAt
	}
	sourceRefs := snapshot.ProvenanceRefs
	if len(sourceRefs) == 0 {
		sourceRefs = []string{snapshot.SourceRef + "/information_schema", snapshot.SourceRef + "/query/dw02"}
	}
	preview, err := connector.Preview(connector.Plan{
		ConnectorID: connectorID, Mode: connector.ModeReconcile,
		Scope:            connector.Scope{Aspects: []string{"profile", "joinEvidence", "annotation"}},
		TargetRepository: targetRepo, BaseCommit: kernel.CommitID(baseCommit),
		Desired: units, Observed: observed,
		SourceRefs: sourceRefs,
		ProducedAt: producedAt, Message: "mirror MySQL tpch observations",
	})
	if err != nil {
		fatalf("preview: %v", err)
	}
	writeJSON(outputPath, preview)
}

func translate(snapshot observationSnapshot, mapping identityMap) ([]connector.Unit, error) {
	if snapshot.SourceRef == "" || mapping.Version != 1 || mapping.Objects == nil {
		return nil, fmt.Errorf("sourceRef and version 1 identity mapping are required")
	}
	profileID, err := mappedColumn(mapping, snapshot.SourceRef, snapshot.Profile.Schema, snapshot.Profile.Table, snapshot.Profile.Column)
	if err != nil {
		return nil, err
	}
	units := []connector.Unit{{
		Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: kernel.ObjectID(profileID), AspectName: "profile"},
		Value: map[string]any{
			"entityType": "columnProfile", "schema": snapshot.Profile.Schema,
			"table": snapshot.Profile.Table, "column": snapshot.Profile.Column,
			"rowCount": snapshot.Profile.RowCount, "ndv": snapshot.Profile.NDV,
			"minValue": snapshot.Profile.MinValue, "maxValue": snapshot.Profile.MaxValue,
			"avgValue": snapshot.Profile.AvgValue, "distribution": snapshot.Profile.Distribution,
		},
		PathHint: "observations/" + profileID + ".profile.json",
	}}

	sort.Slice(snapshot.Joins, func(i, j int) bool { return snapshot.Joins[i].Relation < snapshot.Joins[j].Relation })
	seenRelations := map[string]struct{}{}
	for _, join := range snapshot.Joins {
		if join.Relation == "" || len(join.ChildColumns) == 0 || len(join.ChildColumns) != len(join.ParentColumns) {
			return nil, fmt.Errorf("invalid join evidence %q", join.Relation)
		}
		if _, exists := seenRelations[join.Relation]; exists {
			return nil, fmt.Errorf("duplicate join relation %s", join.Relation)
		}
		seenRelations[join.Relation] = struct{}{}
		childID, err := mappedTable(mapping, snapshot.SourceRef, join.ChildSchema, join.ChildTable)
		if err != nil {
			return nil, err
		}
		parentID, err := mappedTable(mapping, snapshot.SourceRef, join.ParentSchema, join.ParentTable)
		if err != nil {
			return nil, err
		}
		units = append(units, connector.Unit{
			Address: kernel.Address{Kind: kernel.KindMember, ObjectID: kernel.ObjectID(childID), AspectName: "joinEvidence", MemberKey: join.Relation},
			Value: map[string]any{
				"entityType": "joinEvidence", "relation": join.Relation,
				"childObjectId": childID, "childColumns": join.ChildColumns,
				"parentObjectId": parentID, "parentColumns": join.ParentColumns,
				"childRowCount": join.ChildRowCount, "orphanCount": join.OrphanCount,
				"evidenceSql": join.EvidenceSQL,
			},
			PathHint: "observations/" + childID + ".join." + safeName(join.Relation) + ".json",
		})
	}

	annotationID, err := mappedColumn(mapping, snapshot.SourceRef, snapshot.Annotation.Schema, snapshot.Annotation.Table, snapshot.Annotation.Column)
	if err != nil {
		return nil, err
	}
	units = append(units, connector.Unit{
		Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: kernel.ObjectID(annotationID), AspectName: "annotation"},
		Value: map[string]any{
			"entityType": "sourceAnnotation", "schema": snapshot.Annotation.Schema,
			"table": snapshot.Annotation.Table, "column": snapshot.Annotation.Column,
			"comment": snapshot.Annotation.Comment, "sourceFragment": snapshot.Annotation.SourceFragment,
		},
		PathHint: "observations/" + annotationID + ".annotation.json",
	})
	return units, nil
}

func mappedTable(mapping identityMap, sourceRef, schema, table string) (string, error) {
	return mapped(mapping, strings.TrimSuffix(sourceRef, "/")+"/table/"+schema+"/"+table)
}

func mappedColumn(mapping identityMap, sourceRef, schema, table, column string) (string, error) {
	return mapped(mapping, strings.TrimSuffix(sourceRef, "/")+"/column/"+schema+"/"+table+"/"+column)
}

func mapped(mapping identityMap, sourceKey string) (string, error) {
	id := mapping.Objects[sourceKey]
	if id == "" {
		return "", fmt.Errorf("source key %s has no DW-01 identity", sourceKey)
	}
	return id, nil
}

func safeName(value string) string {
	return strings.NewReplacer("/", "_", ":", "_", " ", "_").Replace(value)
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
		fatalf("encode: %v", err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		fatalf("write %s: %v", path, err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "mysql-observations: "+format+"\n", args...)
	os.Exit(1)
}
