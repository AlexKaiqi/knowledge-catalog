"""Shared Bound State source for the scene accessor containers."""

from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Any

DEFAULT_PATH = os.environ.get("KC_SOURCE_PATH", str(Path(__file__).with_name("source.json")))


def path() -> Path:
    return Path(os.environ.get("KC_SOURCE_PATH", DEFAULT_PATH))


def load() -> dict[str, Any]:
    raw = json.loads(path().read_text())
    if not isinstance(raw, dict):
        raise ValueError("source must be an object")
    raw.setdefault("revision", "0")
    raw.setdefault("generation", "scene-accessor-v1")
    raw.setdefault("entities", {})
    return raw


def save(data: dict[str, Any]) -> None:
    target = path()
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(data, indent=2, ensure_ascii=False) + "\n")


def entity(object_id: str) -> dict[str, Any]:
    entities = load().get("entities") or {}
    value = entities.get(object_id)
    if not isinstance(value, dict):
        return {"status": "healthy", "runtime": "docker"}
    return value
