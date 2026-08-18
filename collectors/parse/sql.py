"""sqlglot table I/O parser for collector eval.

Envelope I/O stays SOURCE. This module is DERIVATION: SQL text → reads/writes.
Does not write Ingress.
"""

from __future__ import annotations

import re
import signal
from dataclasses import dataclass, field
from typing import Iterable

import sqlglot
from sqlglot import exp
from sqlglot.errors import ParseError, TokenError

DIALECTS = ("hive", "spark")
MAX_SQL_CHARS = 100_000
DEFAULT_TIMEOUT_SEC = 8

_EMPTY = frozenset({"", "null", "none", "\\n"})


class _Timeout(Exception):
    pass


@dataclass(frozen=True)
class TableRef:
    cluster: str | None
    db: str | None
    table: str

    def fqn(self) -> str:
        parts = [p for p in (self.cluster, self.db, self.table) if p]
        return ".".join(parts)


@dataclass
class ParseResult:
    ok: bool
    dialect: str | None = None
    preprocessed: bool = False
    reads: list[TableRef] = field(default_factory=list)
    writes: list[TableRef] = field(default_factory=list)
    statements: int = 0
    error_type: str | None = None
    error: str | None = None
    skipped: str | None = None


def is_blank(value: str | None) -> bool:
    return value is None or value.strip().lower() in _EMPTY


def preprocess(raw: str) -> str:
    """Hippo-aligned cleanup. Does not strip INSERT (that drops the write target)."""
    sql = raw.strip().rstrip(";")
    sql = re.sub(r"^\s*SET\b.*$", "", sql, flags=re.IGNORECASE | re.MULTILINE)
    sql = re.sub(r"\bINSERT\s+TABLE\b", "INSERT INTO TABLE", sql, flags=re.IGNORECASE)
    sql = re.sub(r"\|{4,}", ";\n", sql)
    sql = re.sub(r"\s*::\s*", ".", sql)
    sql = re.sub(r"`\s*\.\s*`", "`.`", sql)
    sql = re.sub(r"^\s*EXPLAIN\s+(EXTENDED\s+)?", "", sql, flags=re.IGNORECASE)
    sql = re.sub(r"\bPARTITION\s*\([^)]*\)", "", sql, flags=re.IGNORECASE)
    sql = re.sub(r"\bstring1?\s*\(([^)]*)\)", r"\1", sql, flags=re.IGNORECASE)
    sql = re.sub(r"---+\+?", "--", sql)
    sql = sql.replace("$", "__DOLLAR__")
    sql = re.sub(r"\bgrouop\s+by\b", "GROUP BY", sql, flags=re.IGNORECASE)
    sql = re.sub(r"\bgruop\s+by\b", "GROUP BY", sql, flags=re.IGNORECASE)
    sql = re.sub(r"\bgropu\s+by\b", "GROUP BY", sql, flags=re.IGNORECASE)
    sql = re.sub(r"\bselct\b", "SELECT", sql, flags=re.IGNORECASE)
    sql = re.sub(r"\bwehre\b", "WHERE", sql, flags=re.IGNORECASE)
    sql = re.sub(r"\bfomr\b", "FROM", sql, flags=re.IGNORECASE)
    sql = re.sub(r"\*from\b", "* from", sql, flags=re.IGNORECASE)
    sql = re.sub(
        r"\bDECODE\s*\(\s*GROUPING\s*\(([^)]+)\)\s*,\s*1\s*,\s*([^,]+)\s*,\s*([^)]+)\)",
        r"IF(GROUPING(\1)=1,\2,\3)",
        sql,
        flags=re.IGNORECASE,
    )
    sql = re.sub(r",\s*CUBE\s*\([^)]*\)", "", sql, flags=re.IGNORECASE)
    sql = re.sub(r"\bCUBE\s*\([^)]*\)\s*,?", "", sql, flags=re.IGNORECASE)
    sql = re.sub(r"/\*\+[^*]*\*/", "", sql)
    return sql.strip()


def _table_ref(node: exp.Table) -> TableRef | None:
    name = node.name
    if not name:
        return None
    catalog = node.catalog or None
    db = node.db or None
    if catalog and db:
        return TableRef(cluster=catalog, db=db, table=name)
    if db:
        return TableRef(cluster=None, db=db, table=name)
    return TableRef(cluster=None, db=None, table=name)


def _write_tables(tree: exp.Expression) -> list[TableRef]:
    out: list[TableRef] = []

    def add(table: exp.Expression | None) -> None:
        if isinstance(table, exp.Table):
            ref = _table_ref(table)
            if ref:
                out.append(ref)
        elif isinstance(table, exp.Schema) and isinstance(table.this, exp.Table):
            ref = _table_ref(table.this)
            if ref:
                out.append(ref)

    if isinstance(tree, exp.Insert):
        add(tree.this)
    elif isinstance(tree, exp.Create):
        add(tree.this)
    elif isinstance(tree, (exp.Delete, exp.Update)):
        add(tree.this)
    elif isinstance(tree, exp.Merge):
        add(tree.this)
    return out


def _read_tables(tree: exp.Expression, writes: Iterable[TableRef]) -> list[TableRef]:
    write_fqns = {w.fqn().lower() for w in writes}
    cte_names = {c.alias_or_name.lower() for c in tree.find_all(exp.CTE) if c.alias_or_name}
    reads: list[TableRef] = []
    seen: set[str] = set()
    for node in tree.find_all(exp.Table):
        ref = _table_ref(node)
        if ref is None:
            continue
        key = ref.fqn().lower()
        if key in write_fqns or key in seen:
            continue
        if ref.db is None and ref.table.lower() in cte_names:
            continue
        seen.add(key)
        reads.append(ref)
    return reads


def _io(tree: exp.Expression) -> tuple[list[TableRef], list[TableRef]]:
    writes = _write_tables(tree)
    reads = _read_tables(tree, writes)
    return reads, writes


def _parse_once(sql: str, dialect: str) -> list[exp.Expression]:
    trees = sqlglot.parse(sql, read=dialect)
    return [t for t in trees if t is not None]


def parse_sql(sql: str, *, dialects: Iterable[str] = DIALECTS, timeout_sec: int = DEFAULT_TIMEOUT_SEC) -> ParseResult:
    if is_blank(sql):
        return ParseResult(ok=False, skipped="empty")
    if len(sql) > MAX_SQL_CHARS:
        return ParseResult(ok=False, skipped="too_long", error=f"{len(sql)} chars")

    def _run() -> ParseResult:
        cleaned = preprocess(sql)
        if not cleaned:
            return ParseResult(ok=False, skipped="empty_after_preprocess")
        last_err: Exception | None = None
        for dialect in dialects:
            try:
                trees = _parse_once(cleaned, dialect)
            except (ParseError, TokenError, ValueError) as err:
                last_err = err
                continue
            if not trees:
                last_err = ParseError("empty parse")
                continue
            reads: list[TableRef] = []
            writes: list[TableRef] = []
            seen_r: set[str] = set()
            seen_w: set[str] = set()
            for tree in trees:
                r, w = _io(tree)
                for item in r:
                    if item.fqn() not in seen_r:
                        seen_r.add(item.fqn())
                        reads.append(item)
                for item in w:
                    if item.fqn() not in seen_w:
                        seen_w.add(item.fqn())
                        writes.append(item)
            return ParseResult(
                ok=True,
                dialect=dialect,
                preprocessed=cleaned != sql.strip().rstrip(";"),
                reads=reads,
                writes=writes,
                statements=len(trees),
            )
        err = last_err or ParseError("unparsed")
        return ParseResult(ok=False, error_type=type(err).__name__, error=str(err)[:300])

    if timeout_sec <= 0:
        return _run()

    def _handler(_signum, _frame):
        raise _Timeout()

    previous = signal.signal(signal.SIGALRM, _handler)
    signal.setitimer(signal.ITIMER_REAL, timeout_sec)
    try:
        return _run()
    except _Timeout:
        return ParseResult(ok=False, error_type="Timeout", error=f">{timeout_sec}s")
    finally:
        signal.setitimer(signal.ITIMER_REAL, 0)
        signal.signal(signal.SIGALRM, previous)
