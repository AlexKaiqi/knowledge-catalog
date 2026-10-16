#!/usr/bin/env python3
"""Assemble a postgres:// DSN for lakeFS Graveler."""

from __future__ import annotations

import os
import sys
from urllib.parse import quote


def dsn_from_env(environ: dict[str, str]) -> str:
    """Return a libpq URL from operator environment.

    Prefers LAKEFS_DATABASE_POSTGRES_CONNECTION_STRING. Otherwise builds from
    PG_HOST, PG_PORT, PG_USER, PG_PASSWORD, PG_DATABASE, and PG_SSLMODE.

    Args:
        environ: Environment mapping, typically os.environ.

    Returns:
        A postgres:// connection string.

    Raises:
        ValueError: Required pieces are missing.
    """
    ready = environ.get("LAKEFS_DATABASE_POSTGRES_CONNECTION_STRING", "").strip()
    if ready:
        return ready
    host = environ.get("PG_HOST", "").strip()
    user = environ.get("PG_USER", "").strip()
    password = environ.get("PG_PASSWORD", "").strip()
    database = environ.get("PG_DATABASE", "").strip()
    port = environ.get("PG_PORT", "5432").strip() or "5432"
    sslmode = environ.get("PG_SSLMODE", "require").strip() or "require"
    missing = [
        name
        for name, value in (
            ("PG_HOST", host),
            ("PG_USER", user),
            ("PG_PASSWORD", password),
            ("PG_DATABASE", database),
        )
        if not value
    ]
    if missing:
        raise ValueError(
            "fill LAKEFS_DATABASE_POSTGRES_CONNECTION_STRING or " + ", ".join(missing)
        )
    return (
        f"postgres://{quote(user, safe='')}:{quote(password, safe='')}"
        f"@{host}:{port}/{database}?sslmode={quote(sslmode, safe='')}"
    )


def main(argv: list[str]) -> int:
    """Print the DSN to stdout.

    Args:
        argv: Command-line arguments; unused besides program name.

    Returns:
        0 on success, 1 on missing configuration.
    """
    del argv
    try:
        print(dsn_from_env(dict(os.environ)), end="")
    except ValueError as err:
        print(f"FAIL: {err}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
