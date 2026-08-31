package dolt

import (
	"sort"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/unitcodec"
	snapshotdolt "kc/snapshot/dolt"
)

func (r *Repository) ApplyKnowledgeChange(commandID string, changeSet knowledge.ChangeSet) (kernel.CommitID, error) {
	if changeSet.TargetRepository != r.ID() {
		return "", kernel.Fail(kernel.ErrTargetRepositoryDenied, "target %s does not match %s", changeSet.TargetRepository, r.ID())
	}
	if changeSet.BaseCommit != changeSet.ExpectedTargetCommit {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "baseCommit must equal expectedTargetCommit")
	}
	if !r.HasCommit(changeSet.BaseCommit) {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", changeSet.BaseCommit)
	}
	byObject := map[knowledge.ObjectID][]knowledge.Operation{}
	for _, operation := range changeSet.Operations {
		byObject[operation.Address.ObjectID] = append(byObject[operation.Address.ObjectID], operation)
	}
	objectIDs := make([]knowledge.ObjectID, 0, len(byObject))
	for objectID := range byObject {
		objectIDs = append(objectIDs, objectID)
	}
	sort.Slice(objectIDs, func(i, j int) bool { return objectIDs[i] < objectIDs[j] })
	if err := r.validateObjectKeys(objectIDs, changeSet.BaseCommit); err != nil {
		return "", err
	}
	existing, err := r.loadUnits(objectIDs, changeSet.BaseCommit)
	if err != nil {
		return "", err
	}
	statements := []string{}
	for _, objectID := range objectIDs {
		final, _, err := unitcodec.Apply(existing[objectID], byObject[objectID], changeSet.Provenance)
		if err != nil {
			return "", err
		}
		objectStatements, err := statementsForObject(objectID, final)
		if err != nil {
			return "", err
		}
		statements = append(statements, objectStatements...)
	}
	return r.base.ApplyNativeCommit(snapshotdolt.NativeCommit{
		TargetRef: changeSet.TargetRef, BaseCommit: changeSet.BaseCommit,
		ExpectedTargetCommit: changeSet.ExpectedTargetCommit,
		Statements:           statements, Tables: nativeTables, Message: changeSet.Message,
		Author: changeSet.Author, RequestID: changeSet.RequestID, RuleID: changeSet.RuleID, CommandID: commandID,
	})
}

func (r *Repository) validateObjectKeys(objectIDs []knowledge.ObjectID, commit kernel.CommitID) error {
	keys := make([]string, 0, len(objectIDs))
	wanted := map[string]knowledge.ObjectID{}
	for _, objectID := range objectIDs {
		key := objectKey(objectID)
		keys = append(keys, sqlString(key))
		wanted[key] = objectID
	}
	rows, err := r.base.NativeQuery(`SELECT object_key, TO_BASE64(CAST(object_id AS BINARY)) AS object_id64
        FROM kc_objects AS OF ` + sqlString(string(commit)) + ` WHERE object_key IN (` + joinComma(keys) + `)`)
	if err != nil {
		return err
	}
	for _, row := range rows {
		expected := wanted[rowString(row, "object_key")]
		actual, err := rowText64(row, "object_id64")
		if err != nil {
			return err
		}
		if actual != string(expected) {
			return kernel.Fail(kernel.ErrPreconditionFailed, "native object key collision for %s", expected)
		}
	}
	return nil
}

func statementsForObject(objectID knowledge.ObjectID, units []unitcodec.Unit) ([]string, error) {
	key := objectKey(objectID)
	statements := []string{
		"DELETE FROM kc_units WHERE object_key=" + sqlString(key),
	}
	if len(units) == 0 {
		statements = append(statements, `REPLACE INTO kc_objects(
			object_key,object_id,kind,is_schema,status,unit_count,object_digest,declaration_digest
		) VALUES (`+sqlString(key)+","+textSQL(string(objectID))+","+sqlString(string(knowledge.KindEntity))+","+
			boolSQL(knowledge.IsSchemaObject(objectID))+","+
			sqlString(string(knowledge.StatusRemoved))+`,0,'','')`)
		return statements, nil
	}
	for _, unit := range units {
		valueJSON, err := jsonText(unit.Value)
		if err != nil {
			return nil, err
		}
		sourceSQL := "NULL"
		if normalized := unit.ValueSource.Normalized(); normalized != nil {
			raw, err := jsonText(normalized)
			if err != nil {
				return nil, err
			}
			sourceSQL = textSQL(raw)
		}
		provenanceSQL := "NULL"
		if unit.Provenance != nil {
			raw, err := jsonText(unit.Provenance)
			if err != nil {
				return nil, err
			}
			provenanceSQL = textSQL(raw)
		}
		statements = append(statements, `REPLACE INTO kc_units(
            unit_key,object_key,object_id,kind,aspect_name,member_key,path_hint,storage_path,
            schema_ref,value_source_json,provenance_json,value_json,value_digest
        ) VALUES (`+
			sqlString(unitKey(unit.Address))+","+sqlString(key)+","+textSQL(string(objectID))+","+
			sqlString(string(unit.Address.Kind))+","+textSQL(unit.Address.AspectName)+","+textSQL(unit.Address.MemberKey)+","+
			textSQL(unit.PathHint)+","+textSQL(unit.Path)+","+textSQL(unit.SchemaRef)+","+sourceSQL+","+
			provenanceSQL+","+textSQL(valueJSON)+","+sqlString(string(kernel.CanonicalDigest(unit.Value)))+")")
	}
	assembled, err := unitcodec.Assemble(units)
	if err != nil {
		return nil, err
	}
	kind := knowledge.KindEntity
	for _, unit := range units {
		if unit.Address.Kind != knowledge.KindRelation {
			continue
		}
		kind = knowledge.KindRelation
		if _, err := knowledge.DecodeRelation(unit.Address, unit.Value); err != nil {
			return nil, err
		}
		break
	}
	statements = append(statements, `REPLACE INTO kc_objects(
		object_key,object_id,kind,is_schema,status,unit_count,object_digest,declaration_digest
	) VALUES (`+sqlString(key)+","+textSQL(string(objectID))+","+sqlString(string(kind))+","+
		boolSQL(knowledge.IsSchemaObject(objectID))+","+
		sqlString(string(knowledge.StatusResolved))+","+intSQL(len(units))+","+
		sqlString(string(kernel.CanonicalDigest(assembled)))+","+
		sqlString(string(unitcodec.DeclarationDigest(units)))+")")
	return statements, nil
}

func intSQL(value int) string {
	if value == 0 {
		return "0"
	}
	digits := []byte{}
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

func boolSQL(value bool) string {
	if value {
		return "TRUE"
	}
	return "FALSE"
}
