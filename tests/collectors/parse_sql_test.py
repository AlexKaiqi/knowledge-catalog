from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "collectors"))

from parse.sql import parse_sql, preprocess


def test_select_reads():
    result = parse_sql("SELECT a FROM db.t1 JOIN db.t2 ON t1.id = t2.id")
    assert result.ok
    assert {r.fqn() for r in result.reads} == {"db.t1", "db.t2"}
    assert result.writes == []


def test_insert_io():
    result = parse_sql("INSERT OVERWRITE TABLE tl.app.out SELECT * FROM tl.app.events")
    assert result.ok
    assert {w.fqn() for w in result.writes} == {"tl.app.out"}
    assert {r.fqn() for r in result.reads} == {"tl.app.events"}


def test_cte_not_a_read():
    result = parse_sql("WITH x AS (SELECT * FROM db.src) SELECT * FROM x")
    assert result.ok
    assert {r.fqn() for r in result.reads} == {"db.src"}


def test_empty_skipped():
    assert parse_sql("NULL").skipped == "empty"
    assert parse_sql("").skipped == "empty"


def test_preprocess_dollar_and_typo():
    sql = preprocess("selct *from db.t where id = $bizid")
    assert "SELECT" in sql.upper() or "selct" not in sql
    assert "__DOLLAR__" in sql
    assert "* from" in sql.lower()
