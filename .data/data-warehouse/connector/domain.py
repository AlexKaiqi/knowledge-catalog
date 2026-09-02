#!/usr/bin/env python3
"""Live MySQL source-key and physical-knowledge translation.

Authored Schema and semantic knowledge are already publication-shaped files
under ``knowledge/`` and never pass through this Connector module.
"""

from __future__ import annotations

import hashlib
import json
import re
from pathlib import Path
from typing import Any


def load_json(path: Path) -> dict[str, Any]:
    with path.open(encoding="utf-8") as handle:
        return json.load(handle)


def write_json(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as handle:
        json.dump(value, handle, indent=2, ensure_ascii=False, sort_keys=True)
        handle.write("\n")


def canonical_digest(value: Any) -> str:
    encoded = json.dumps(
        value, sort_keys=True, separators=(",", ":"), ensure_ascii=False
    ).encode("utf-8")
    return hashlib.sha256(encoded).hexdigest()


def declaration_digest(schema_ref: str) -> str:
    return canonical_digest({
        "schemaRef": schema_ref.strip(),
        "valueSource": None,
    })


def slug(value: str) -> str:
    value = re.sub(r"([a-z0-9])([A-Z])", r"\1-\2", value)
    return re.sub(r"[^a-zA-Z0-9]+", "-", value).strip("-").lower()


def stable_id(connector_id: str, kind: str, source_key: str) -> str:
    digest = hashlib.sha256(source_key.encode("utf-8")).hexdigest()[:24]
    return f"dw-{slug(connector_id)}-{slug(kind)}-{digest}"


def schema_id(entity: str, aspect: str) -> str:
    return f"schema/{slug(entity)}.{aspect}"


def address(kind: str, object_id: str, aspect: str = "", member: str = "") -> dict[str, str]:
    result = {"kind": kind, "objectId": object_id}
    if aspect:
        result["aspectName"] = aspect
    if member:
        result["memberKey"] = member
    return result


def unit(
    kind: str,
    object_id: str,
    value: Any,
    *,
    aspect: str = "",
    member: str = "",
    schema_ref: str = "",
    path_hint: str = "",
    source_key: str = "",
) -> dict[str, Any]:
    result: dict[str, Any] = {"address": address(kind, object_id, aspect, member), "value": value}
    if schema_ref:
        result["schemaRef"] = schema_ref
    if path_hint:
        result["pathHint"] = path_hint
    if source_key:
        result["sourceKey"] = source_key
    return result


def aspect_unit(
    entity: str,
    object_id: str,
    aspect: str,
    value: Any,
    *,
    member: str = "",
    path_hint: str = "",
    source_key: str = "",
) -> dict[str, Any]:
    return unit(
        "Member" if member else "Aspect",
        object_id,
        value,
        aspect=aspect,
        member=member,
        schema_ref=schema_id(entity, aspect),
        path_hint=path_hint,
        source_key=source_key,
    )


def relation_unit(
    publisher_id: str,
    relation_type: str,
    key: str,
    endpoints: list[dict[str, str]],
    *,
    repository: str = "kr://dw/physical",
) -> dict[str, Any]:
    object_id = stable_id(publisher_id, f"rel-{relation_type}", key)
    qualified_endpoints = [
        {"role": endpoint["role"], "objectRef": {"repository": repository, "object": endpoint["objectRef"]}}
        for endpoint in endpoints
    ]
    return unit(
        "Relation",
        object_id,
        {
            "relationId": object_id,
            "relationType": relation_type,
            "direction": "DIRECTED",
            "endpoints": qualified_endpoints,
        },
        schema_ref="schema/relation.canonical",
        path_hint=f"relations/{object_id}.json",
        source_key=key,
    )


def grouped_relation_units(
    publisher_id: str,
    relation_type: str,
    container_key: str,
    container_id: str,
    members: list[tuple[str, str]],
    *,
    max_endpoints: int = 256,
) -> list[dict[str, Any]]:
    """Build bounded, stable relation objects for one container.

    A normal table uses one relation with repeated ``member`` roles. Above the
    endpoint cap, hash-prefix buckets keep an existing member in the same
    relation when unrelated members are added.
    """
    member_limit = max_endpoints - 1
    if member_limit < 1:
        raise ValueError("max_endpoints must leave room for the container")
    ordered = sorted(members)
    if not ordered:
        return []
    groups: list[tuple[str, list[tuple[str, str]]]] = []

    def split(values: list[tuple[str, str]], prefix_length: int) -> None:
        buckets: dict[str, list[tuple[str, str]]] = {}
        for source_key, object_id in values:
            prefix = hashlib.sha256(source_key.encode("utf-8")).hexdigest()[:prefix_length]
            buckets.setdefault(prefix, []).append((source_key, object_id))
        for prefix, bucket in sorted(buckets.items()):
            if len(bucket) > member_limit:
                split(bucket, prefix_length + 1)
            else:
                groups.append((prefix, sorted(bucket)))

    if len(ordered) <= member_limit:
        groups.append(("all", ordered))
    else:
        split(ordered, 2)

    result = []
    for bucket, values in groups:
        key = f"{container_key}#members-{bucket}"
        result.append(relation_unit(
            publisher_id,
            relation_type,
            key,
            [{"role": "container", "objectRef": container_id}] + [
                {"role": "member", "objectRef": object_id}
                for _, object_id in values
            ],
        ))
    return result


def _assign(mapping: dict[str, str], publisher_id: str, kind: str, source_key: str) -> str:
    object_id = stable_id(publisher_id, kind, source_key)
    prior = mapping.setdefault(source_key, object_id)
    if prior != object_id:
        raise ValueError(f"source key {source_key} changed object_id")
    return object_id


def physical_desired(snapshot: dict[str, Any]) -> tuple[list[dict[str, Any]], dict[str, str]]:
    connector_id = snapshot["connectorId"]
    mapping: dict[str, str] = {}
    platform = snapshot["platform"]
    platform_id = _assign(mapping, connector_id, "platform", platform["sourceKey"])
    for database in snapshot["databases"]:
        _assign(mapping, connector_id, "database", database["sourceKey"])
        for schema in database["schemas"]:
            _assign(mapping, connector_id, "database-schema", schema["sourceKey"])
            for job in schema.get("jobs", []):
                _assign(mapping, connector_id, "data-job", job["sourceKey"])
            for table in schema["tables"]:
                _assign(mapping, connector_id, "table", table["sourceKey"])
                for column in table["columns"]:
                    _assign(mapping, connector_id, "column", column["sourceKey"])

    desired = [
        aspect_unit("DataPlatformInstance", platform_id, "properties", {
            "entityType": "DataPlatformInstance", "name": platform["name"],
            "qualifiedName": f"mysql:{platform['nativeId']}", "nativeId": platform["nativeId"],
            "nativeKind": "PLATFORM_INSTANCE", "nativeType": "mysql", "environment": platform["environment"],
        }, path_hint=f"data-platform-instances/{platform_id}/properties.json", source_key=platform["sourceKey"]),
    ]

    for database in snapshot["databases"]:
        database_id = mapping[database["sourceKey"]]
        desired.extend([
            aspect_unit("Database", database_id, "properties", {
                "entityType": "Database", "name": database["name"],
                "qualifiedName": f"{platform['nativeId']}.{database['nativeId']}", "nativeId": database["nativeId"],
                "nativeKind": "DATABASE", "nativeType": "DATABASE", "platformRef": platform_id,
            }, path_hint=f"databases/{database_id}/properties.json", source_key=database["sourceKey"]),
            relation_unit(connector_id, "contains", f"{platform['sourceKey']}->{database['sourceKey']}", [
                {"role": "container", "objectRef": platform_id}, {"role": "member", "objectRef": database_id},
            ]),
        ])
        for schema in database["schemas"]:
            schema_object_id = mapping[schema["sourceKey"]]
            desired.extend([
                aspect_unit("DatabaseSchema", schema_object_id, "properties", {
                    "entityType": "DatabaseSchema", "name": schema["name"],
                    "qualifiedName": f"{platform['nativeId']}.{database['nativeId']}.{schema['nativeId']}",
                    "nativeId": schema["nativeId"], "nativeKind": "SCHEMA", "nativeType": "SCHEMA", "platformRef": platform_id,
                }, path_hint=f"database-schemas/{schema_object_id}/properties.json", source_key=schema["sourceKey"]),
                relation_unit(connector_id, "contains", f"{database['sourceKey']}->{schema['sourceKey']}", [
                    {"role": "container", "objectRef": database_id}, {"role": "member", "objectRef": schema_object_id},
                ]),
            ])
            for job in schema.get("jobs", []):
                job_id = mapping[job["sourceKey"]]
                desired.extend([
                    aspect_unit("DataJob", job_id, "properties", {
                        "entityType": "DataJob", "name": job["name"],
                        "qualifiedName": job["nativeId"], "nativeId": job["nativeId"],
                        "nativeKind": "EVENT", "nativeType": job["nativeType"],
                        "platformRef": platform_id,
                    }, path_hint=f"data-jobs/{job_id}/properties.json", source_key=job["sourceKey"]),
                    aspect_unit("DataJob", job_id, "definition", {
                        "language": job["language"], "sourceCode": job["sourceCode"],
                        "schedule": job["schedule"], "enabled": job["enabled"],
                        "description": job["description"],
                    }, path_hint=f"data-jobs/{job_id}/definition.json"),
                    relation_unit(connector_id, "contains", f"{schema['sourceKey']}->{job['sourceKey']}", [
                        {"role": "container", "objectRef": schema_object_id}, {"role": "member", "objectRef": job_id},
                    ]),
                ])
            for table in schema["tables"]:
                table_id = mapping[table["sourceKey"]]
                column_ids = {column["name"]: mapping[column["sourceKey"]] for column in table["columns"]}
                desired.extend([
                    aspect_unit("Table", table_id, "properties", {
                        "entityType": "Table", "name": table["name"], "qualifiedName": table["nativeId"],
                        "nativeId": table["nativeId"], "nativeKind": "TABLE", "nativeType": table["nativeType"], "platformRef": platform_id,
                    }, path_hint=f"tables/{table_id}/properties.json", source_key=table["sourceKey"]),
                    aspect_unit("Table", table_id, "schema", {
                        "columnCount": len(table["columns"]),
                        "primaryKeyColumnRefs": [column_ids[name] for name in table.get("primaryKey", [])],
                    }, path_hint=f"tables/{table_id}/schema.json", source_key=table["sourceKey"]),
                    relation_unit(connector_id, "contains", f"{schema['sourceKey']}->{table['sourceKey']}", [
                        {"role": "container", "objectRef": schema_object_id}, {"role": "member", "objectRef": table_id},
                    ]),
                ])
                column_members = []
                for column in table["columns"]:
                    column_id = mapping[column["sourceKey"]]
                    column_members.append((column["sourceKey"], column_id))
                    desired.append(
                        aspect_unit("Column", column_id, "properties", {
                            "entityType": "Column", "name": column["name"], "qualifiedName": column["nativeId"],
                            "nativeId": column["nativeId"], "nativeKind": "COLUMN", "nativeType": column["dataType"],
                            "platformRef": platform_id, "tableRef": table_id, "dataType": column["dataType"],
                            "nullable": column["nullable"], "ordinal": column["ordinal"],
                        }, path_hint=f"columns/{column_id}/properties.json", source_key=column["sourceKey"]),
                    )
                desired.extend(grouped_relation_units(
                    connector_id, "contains", table["sourceKey"], table_id, column_members,
                ))
    return desired, mapping
