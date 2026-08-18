"""Read hippo_sources catalog (ES metadata + file body).

ES mappings do not store `content`. Body lives at `location` (file:.spaces/...).
"""

from __future__ import annotations

import json
import os
import urllib.parse
import urllib.request
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterator

DEFAULT_ES = os.environ.get("HIPPO_OS_URL", "http://dev.omni.elastic.polaris:9200")
DEFAULT_ES_USER = os.environ.get("HIPPO_OS_USER", "elastic")
DEFAULT_HIPPO_ROOT = Path(os.environ.get("HIPPO_ROOT", "/data/home/kaiqidong/Hippo"))


@dataclass(frozen=True)
class HippoSource:
    source_id: str
    source_type: str
    space: str
    location: str
    content: str | None
    content_size: int


def _es_request(path: str, body: dict[str, Any] | None, url: str, user: str, password: str) -> dict[str, Any]:
    auth = (user + ":" + password).encode()
    import base64

    req = urllib.request.Request(
        url.rstrip("/") + path,
        data=None if body is None else json.dumps(body).encode(),
        method="GET" if body is None else "POST",
        headers={
            "Authorization": "Basic " + base64.b64encode(auth).decode(),
            "Content-Type": "application/json",
        },
    )
    with urllib.request.urlopen(req, timeout=60) as resp:
        return json.loads(resp.read().decode())


def type_counts(password: str, *, url: str = DEFAULT_ES, user: str = DEFAULT_ES_USER) -> dict[str, int]:
    data = _es_request(
        "/hippo_sources__*/_search",
        {
            "size": 0,
            "track_total_hits": True,
            "aggs": {"types": {"terms": {"field": "type", "size": 20}}},
        },
        url,
        user,
        password,
    )
    return {b["key"]: b["doc_count"] for b in data["aggregations"]["types"]["buckets"]}


def _resolve_location(location: str, hippo_root: Path) -> Path | None:
    if not location:
        return None
    raw = location.removeprefix("file:")
    path = Path(raw)
    if path.is_absolute():
        return path if path.is_file() else None
    candidate = hippo_root / raw
    return candidate if candidate.is_file() else None


def _content_from_file(path: Path) -> str | None:
    try:
        text = path.read_text(encoding="utf-8")
    except OSError:
        return None
    try:
        obj = json.loads(text)
    except json.JSONDecodeError:
        return text
    if isinstance(obj, dict):
        if isinstance(obj.get("content"), str):
            return obj["content"]
        if isinstance(obj.get("sql"), str):
            return obj["sql"]
        if isinstance(obj.get("us_sql"), str):
            return obj["us_sql"]
        if isinstance(obj.get("query_string"), str):
            return obj["query_string"]
    return text


def iter_sources(
    source_type: str,
    password: str,
    *,
    url: str = DEFAULT_ES,
    user: str = DEFAULT_ES_USER,
    hippo_root: Path = DEFAULT_HIPPO_ROOT,
    limit: int | None = None,
) -> Iterator[HippoSource]:
    fetched = 0
    search_after: list[Any] | None = None
    while True:
        body: dict[str, Any] = {
            "size": 200,
            "query": {"term": {"type": source_type}},
            "sort": [{"id": "asc"}],
            "_source": ["id", "type", "location", "space", "content_size"],
        }
        if search_after:
            body["search_after"] = search_after
        data = _es_request("/hippo_sources__*/_search", body, url, user, password)
        hits = data["hits"]["hits"]
        if not hits:
            break
        for hit in hits:
            src = hit["_source"]
            location = str(src.get("location") or "")
            path = _resolve_location(location, hippo_root)
            content = _content_from_file(path) if path else None
            yield HippoSource(
                source_id=str(src.get("id") or ""),
                source_type=str(src.get("type") or source_type),
                space=str(src.get("space") or ""),
                location=location,
                content=content,
                content_size=int(src.get("content_size") or 0),
            )
            fetched += 1
            if limit is not None and fetched >= limit:
                return
        search_after = hits[-1]["sort"]


def sql_from_hippo_content(content: str, source_type: str) -> str | None:
    """Unwrap hippo_sources body to raw SQL."""
    text = content.strip()
    if not text:
        return None
    if source_type == "query_sql":
        if text.startswith("{") or text.startswith("sql:"):
            try:
                import yaml

                parsed = yaml.safe_load(text)
            except Exception:
                parsed = None
            if isinstance(parsed, dict) and parsed.get("sql"):
                return str(parsed["sql"])
        return text
    if source_type in {"etl_sql", "etl_task"}:
        try:
            parsed = json.loads(text)
        except json.JSONDecodeError:
            parsed = None
        if isinstance(parsed, dict):
            for key in ("us_sql", "sql", "definition", "source_sql", "raw_sql"):
                if parsed.get(key):
                    return str(parsed[key])
            task = parsed.get("payload") or parsed.get("task") or {}
            if isinstance(task, dict) and task.get("text"):
                return str(task["text"])
        return text
    return text
