package dolt

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/unitcodec"
)

func sqlString(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

func textSQL(value string) string {
	return "CONVERT(FROM_BASE64(" + sqlString(base64.StdEncoding.EncodeToString([]byte(value))) + ") USING utf8mb4)"
}

func objectKey(objectID knowledge.ObjectID) string {
	return string(kernel.CanonicalDigest(map[string]any{"objectId": objectID}))
}

func unitKey(address knowledge.Address) string {
	return string(kernel.CanonicalDigest(map[string]any{"address": knowledge.AddressKey(address)}))
}

func jsonText(value any) (string, error) {
	raw, err := json.Marshal(value)
	return string(raw), err
}

func decodeJSON(value string, out any) error {
	if strings.TrimSpace(value) == "" || value == "null" {
		return nil
	}
	return json.Unmarshal([]byte(value), out)
}

func rowString(row map[string]any, key string) string {
	value := row[key]
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}

func rowText64(row map[string]any, key string) (string, error) {
	raw := rowString(row, key)
	if raw == "" {
		return "", nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	return string(decoded), err
}

func decodeUnit(row map[string]any) (unitcodec.Unit, error) {
	objectID, err := rowText64(row, "object_id64")
	if err != nil {
		return unitcodec.Unit{}, err
	}
	aspect, err := rowText64(row, "aspect_name64")
	if err != nil {
		return unitcodec.Unit{}, err
	}
	member, err := rowText64(row, "member_key64")
	if err != nil {
		return unitcodec.Unit{}, err
	}
	pathHint, err := rowText64(row, "path_hint64")
	if err != nil {
		return unitcodec.Unit{}, err
	}
	storagePath, err := rowText64(row, "storage_path64")
	if err != nil {
		return unitcodec.Unit{}, err
	}
	schemaRef, err := rowText64(row, "schema_ref64")
	if err != nil {
		return unitcodec.Unit{}, err
	}
	valueJSON, err := rowText64(row, "value_json64")
	if err != nil {
		return unitcodec.Unit{}, err
	}
	var value any
	if err := decodeJSON(valueJSON, &value); err != nil {
		return unitcodec.Unit{}, err
	}
	var source *knowledge.ValueSource
	if sourceJSON, err := rowText64(row, "value_source_json64"); err != nil {
		return unitcodec.Unit{}, err
	} else if sourceJSON != "" {
		var decoded knowledge.ValueSource
		if err := decodeJSON(sourceJSON, &decoded); err != nil {
			return unitcodec.Unit{}, err
		}
		source = decoded.Normalized()
	}
	var provenance *knowledge.ProvenanceEnvelope
	if provenanceJSON, err := rowText64(row, "provenance_json64"); err != nil {
		return unitcodec.Unit{}, err
	} else if provenanceJSON != "" {
		var decoded knowledge.ProvenanceEnvelope
		if err := decodeJSON(provenanceJSON, &decoded); err != nil {
			return unitcodec.Unit{}, err
		}
		provenance = &decoded
	}
	address := knowledge.Address{
		Kind: knowledge.AddressKind(rowString(row, "kind")), ObjectID: knowledge.ObjectID(objectID),
		AspectName: aspect, MemberKey: member,
	}
	return unitcodec.Unit{
		ObjectID: address.ObjectID, Address: address, PathHint: pathHint, Path: storagePath,
		SchemaRef: schemaRef, ValueSource: source, Provenance: provenance, Value: value,
		Digest: kernel.Digest(rowString(row, "value_digest")),
	}, nil
}

const unitSelect = `SELECT
    object_key,
    kind,
    value_digest,
    TO_BASE64(object_id) AS object_id64,
    TO_BASE64(aspect_name) AS aspect_name64,
    TO_BASE64(member_key) AS member_key64,
    TO_BASE64(path_hint) AS path_hint64,
    TO_BASE64(storage_path) AS storage_path64,
    TO_BASE64(schema_ref) AS schema_ref64,
    TO_BASE64(value_source_json) AS value_source_json64,
    TO_BASE64(provenance_json) AS provenance_json64,
    TO_BASE64(value_json) AS value_json64
FROM kc_units`
