package projection

import (
	"context"
	"fmt"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"strconv"
	"strings"
)

// compileProjectionDocument turns the shared logical field meaning into the
// physical document shape. It contains no unit applicability or residual rules.
func compileProjectionDocument(repo objectmodel.Repository, value objectmodel.ObjectValue, spec retrieval.AccessSpec) (ports.CompiledDoc, error) {
	return compileProjectionDocumentObserved(repo, value, nil, spec)
}
func compileProjectionDocumentObserved(repo objectmodel.Repository, value objectmodel.ObjectValue, observations []objectmodel.UnitObservation, spec retrieval.AccessSpec) (ports.CompiledDoc, error) {
	return compileProjectionDocumentComputed(context.Background(), repo, value, observations, spec, nil)
}

func compileProjectionDocumentComputed(ctx context.Context, repo objectmodel.Repository, value objectmodel.ObjectValue, observations []objectmodel.UnitObservation, spec retrieval.AccessSpec, computations retrieval.MemberComputations) (ports.CompiledDoc, error) {

	logical, err := retrieval.ExtractFieldsWithComputations(ctx, repo, value, observations, spec, computations)
	if err != nil {
		return ports.CompiledDoc{}, err
	}
	// Repository projections take their read basis from the published control
	// record, so unchanged documents remain reusable when that basis advances.
	doc := ports.CompiledDoc{ObjectID: value.Address.ObjectID, Kind: logical.Kind, Text: logical.Text, EligibleFields: logical.EligibleFields}
	doc.Semantic = append([]retrieval.SemanticField(nil), logical.Semantic...)
	for _, scalar := range logical.Values {
		cell, err := projectionCell(scalar)
		if err != nil {
			return ports.CompiledDoc{}, err
		}
		doc.Cells = append(doc.Cells, cell)
		doc.Fields = append(doc.Fields, [2]string{cell.Field, cell.Value})
	}
	if doc.Kind == objectmodel.KindRelation {
		address := objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: value.Address.ObjectID}
		relation, err := objectmodel.DecodeRelation(address, value.Value)
		if err != nil {
			return ports.CompiledDoc{}, err
		}
		doc.Relation = &ports.ProjectionRelation{Type: relation.RelationType, Direction: relation.Direction}
		for _, endpoint := range relation.Endpoints {
			doc.Relation.Endpoints = append(doc.Relation.Endpoints, ports.ProjectionRelationEndpoint{Role: endpoint.Role, Object: endpoint.Object})
		}
	}
	doc.ObjectDigest = kernel.CanonicalDigest(map[string]any{
		"objectId": doc.ObjectID, "kind": doc.Kind, "eligibleFields": doc.EligibleFields,
		"cells": doc.Cells, "relation": doc.Relation, "text": doc.Text, "semantic": doc.Semantic,
	})
	return doc, nil
}

func projectionCell(value retrieval.FieldValue) (ports.ProjectionCell, error) {
	field, normalized := value.Field, value.Value
	cell := ports.ProjectionCell{Field: value.Key(), Member: value.Member, Value: normalized, Access: append([]reader.AccessHint(nil), field.Access...)}
	if field.Has(reader.HintText) {
		cell.TextValue = normalized
	}
	// Text discovery does not request a whole-value scalar index. In
	// particular, long text must never become one oversized keyword term.
	if !field.Has(reader.HintFilter) && !field.Has(reader.HintSort) {
		return cell, nil
	}
	typeName := strings.ToLower(strings.TrimSpace(field.Type))
	switch typeName {
	case "", "string", "object_ref", "object_ref_list":
		cell.StringValue = stringPointer(normalized)
	case "bool", "boolean":
		parsed, _ := strconv.ParseBool(normalized)
		cell.BooleanValue = &parsed
	case "int", "integer", "long":
		parsed, _ := strconv.ParseInt(normalized, 10, 64)
		cell.LongValue = &parsed
	case "number", "float", "double":
		parsed, _ := strconv.ParseFloat(normalized, 64)
		cell.DoubleValue = &parsed
	case "date", "datetime", "timestamp":
		cell.DateValue = normalized
	default:
		return ports.ProjectionCell{}, fmt.Errorf("unsupported scalar type %q", field.Type)
	}
	return cell, nil
}

func stringPointer(value string) *string { return &value }
