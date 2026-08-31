from __future__ import annotations

import json
import os
import re
import subprocess
from pathlib import Path
from string import Template

from behave import given, then, when


def _environment(context) -> dict[str, str]:
    env = os.environ.copy()
    env.update({
        "FIXTURE": str(context.fixture),
        "REPO": str(context.repo),
        "RUN": str(context.run),
        "KC_HOME": str(context.home),
        "KC_BIN": str(context.kc),
        "CONNECTOR_PREVIEW": str(context.preview),
        "PYTHON": str(context.python),
        "KC_MYSQL_CONTAINER": context.mysql_container,
        "KC_MYSQL_PASSWORD": "dw-test-root",
        # DSH intentionally scrubs credential-shaped ambient names from model
        # shell calls. This non-secret fixture alias is mapped back only by the
        # declared Connector command; production credentials stay in runtime.
        "KC_MYSQL_AUTH": "dw-test-root",
        "PATH": os.pathsep.join([
            str(context.kc.parent), str(context.preview.parent), str(context.bin_dir), env.get("PATH", ""),
        ]),
        "LC_ALL": "C",
        "PYTHONDONTWRITEBYTECODE": "1",
    })
    if getattr(context, "kc_serve", ""):
        env["KC_SERVE"] = context.kc_serve
        env["KC_SERVER_URL"] = context.kc_serve
        env["KC_AS"] = "service:e2e"
        env["KC_WORKSPACE"] = "warehouse-agent"
    if getattr(context, "resource_access", ""):
        env["KC_RESOURCE_ACCESS_URL"] = context.resource_access
    return env


def _execute(context, command: str) -> None:
    context.command_index += 1
    expanded = os.path.expandvars(command)
    result = subprocess.run(
        ["/bin/sh", "-c", expanded],
        cwd=context.repo,
        env=_environment(context),
        capture_output=True,
        text=True,
    )
    evidence = context.run / "commands"
    evidence.mkdir(exist_ok=True)
    stem = f"{context.command_index:02d}"
    (evidence / f"{stem}.stdout.txt").write_text(result.stdout, encoding="utf-8")
    (evidence / f"{stem}.stderr.txt").write_text(result.stderr, encoding="utf-8")
    context.command = {
        "command": command,
        "expandedCommand": expanded,
        "exitCode": result.returncode,
        "stdout": result.stdout,
        "stderr": result.stderr,
    }
    context.commands.append(context.command)


def _expand(context, value: str) -> str:
    return Template(value).safe_substitute(_environment(context))


@when('I run `{command}`')
def run_one_line(context, command: str) -> None:
    _execute(context, command)


@when("I run the command")
def run_doc_string(context) -> None:
    _execute(context, context.text)


@then("the command succeeds")
def command_succeeds(context) -> None:
    assert context.command is not None, "no command has run"
    assert context.command["exitCode"] == 0, (
        f"command exited {context.command['exitCode']}: {context.command['command']}\n"
        f"{context.command['stderr']}"
    )


@then("the command fails")
def command_fails(context) -> None:
    assert context.command is not None, "no command has run"
    assert context.command["exitCode"] != 0, (
        f"command unexpectedly succeeded: {context.command['command']}\n"
        f"{context.command['stdout']}"
    )


@then('the command fails with stdout error code "{code}"')
def command_fails_with_code(context, code: str) -> None:
    command_fails(context)
    value = _parse_json(context.command["stdout"], "stdout")
    actual = _at(value, "error.code")
    assert actual == code, f"stdout error.code: expected {code!r}, got {actual!r}"


@then('stderr contains "{text}"')
def stderr_contains(context, text: str) -> None:
    assert context.command is not None, "no command has run"
    assert text in context.command["stderr"], (
        f"stderr does not contain {text!r}:\n{context.command['stderr']}"
    )


@then("stdout is empty")
def stdout_is_empty(context) -> None:
    assert context.command is not None, "no command has run"
    assert not context.command["stdout"].strip(), (
        f"stdout must be empty, got:\n{context.command['stdout']}"
    )


def _parse_json(body: str, label: str):
    try:
        return json.loads(body)
    except json.JSONDecodeError as error:
        raise AssertionError(f"{label} is not JSON:\n{body}") from error


_PATH_TOKEN = re.compile(r"(?:^|\.)([^.\[\]]+)|\[(\d+)\]")


def _at(value, path: str):
    if path in ("", "$"):
        return value
    current = value
    normalized = path[2:] if path.startswith("$.") else path
    for match in _PATH_TOKEN.finditer(normalized):
        key, index = match.groups()
        current = current[int(index)] if index is not None else current[key]
    return current


def _expected(raw: str):
    text = raw.strip()
    if text == "":
        return None
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        return text


def _satisfy(value, table, label: str) -> None:
    for row in table:
        path = row["path"]
        matcher = row["matcher"]
        wanted = _expected(row["expected"])
        actual = _at(value, path)
        if matcher == "equals":
            assert actual == wanted, f"{label} {path}: expected {wanted!r}, got {actual!r}"
        elif matcher == "has length":
            assert len(actual) == int(wanted), f"{label} {path}: expected length {wanted}, got {len(actual)}"
        elif matcher == "is non-empty":
            assert actual not in (None, "", [], {}), f"{label} {path}: expected a non-empty value"
        elif matcher == "contains":
            assert wanted in actual, f"{label} {path}: expected to contain {wanted!r}, got {actual!r}"
        else:
            raise AssertionError(f"unsupported matcher {matcher!r}")


@then("stdout JSON satisfies:")
def stdout_json_satisfies(context) -> None:
    command_succeeds(context)
    _satisfy(_parse_json(context.command["stdout"], "stdout"), context.table, "stdout")


@then('JSON file "{path}" satisfies:')
def file_json_satisfies(context, path: str) -> None:
    expanded = Path(_expand(context, path))
    assert expanded.is_file(), f"JSON file does not exist: {expanded}"
    _satisfy(_parse_json(expanded.read_text(encoding="utf-8"), str(expanded)), context.table, str(expanded))


@then('file "{path}" exists and is non-empty')
def file_exists(context, path: str) -> None:
    expanded = Path(_expand(context, path))
    assert expanded.is_file() and expanded.stat().st_size > 0, f"missing or empty file: {expanded}"
