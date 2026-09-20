"""Product claim discovery and declaration checks, without executing evidence."""
from __future__ import annotations

import json
import os
from pathlib import Path
import re


def document_registry(repo: Path, errors: list[str]) -> dict:
    registry = {}
    for path in sorted((repo / "docs/graph/documents").glob("*.okf")):
        try:
            raw = path.read_text(encoding="utf-8")
            # OKF has a YAML envelope and a JSON document object.
            parts = re.split(r"^---\s*$", raw, maxsplit=2, flags=re.MULTILINE)
            data = json.loads(parts[2] if len(parts) == 3 else raw)
            document_id = data["id"]
            if document_id in registry:
                errors.append(f"duplicate document id {document_id} in {path}")
            else:
                registry[document_id] = data
        except (ValueError, KeyError, TypeError, OSError) as exc:
            errors.append(f"document registry {path}: {exc}")
    return registry


def section_claims(path: Path, section: str) -> list[tuple[str, str, int]]:
    claims, seen = [], set()
    active, matches, fence = False, 0, ""
    for line, text in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        marker = re.match(r"^\s{0,3}(`{3,}|~{3,})", text)
        if marker:
            mark = marker[1]
            if not fence:
                fence = mark
            elif mark[0] == fence[0] and len(mark) >= len(fence):
                fence = ""
            continue
        if fence:
            continue
        heading = re.match(r"^(#{1,6})\s+(.+?)(?:\s+#+)?\s*$", text)
        if not heading:
            continue
        level, title = len(heading[1]), heading[2]
        if level <= 2:
            active = level == 2 and title == section
            matches += int(active)
        if not active or level != 3:
            continue
        claim = re.match(r"^(U[1-9][0-9]*)[：:]\s*\S", title)
        if not claim:
            continue
        claim_id = claim[1]
        if claim_id in seen:
            raise ValueError(f"duplicate claim id {claim_id} in {path}")
        seen.add(claim_id)
        claims.append((claim_id, title, line))
    if matches != 1:
        raise ValueError(f"{path}: section {section!r} must occur exactly once, found {matches}")
    if not claims:
        raise ValueError(f"{path}: section {section!r} has no stable U-id claims")
    return claims


def load_claims(repo: Path, registrations, errors: list[str]) -> list[dict]:
    if not isinstance(registrations, dict):
        errors.append("product_documents must be a document-id mapping")
        return []
    if not registrations:
        return []
    registry = document_registry(repo, errors)
    claims = []
    for document_id, config in registrations.items():
        if not isinstance(config, dict) or set(config) - {"section", "gaps"}:
            errors.append(f"{document_id}: product document permits section and gaps only")
            continue
        section = config.get("section")
        if not isinstance(section, str) or not section.strip():
            errors.append(f"{document_id}: a nonempty section is required")
            continue
        document = registry.get(document_id)
        if document is None:
            errors.append(f"{document_id}: document is not registered in docs/graph/documents")
            continue
        relative = document.get("path")
        if not isinstance(relative, str) or not relative:
            errors.append(f"{document_id}: registered document has no path")
            continue
        path = (repo / relative).resolve()
        if not path.is_relative_to(repo) or Path(relative).is_absolute() or path.suffix.lower() != ".md":
            errors.append(f"{document_id}: source must be an in-repository Markdown owner, got {relative}")
            continue
        try:
            extracted = section_claims(path, section)
        except (OSError, ValueError) as exc:
            errors.append(str(exc))
            continue
        gaps = config.get("gaps", {})
        if not isinstance(gaps, dict):
            errors.append(f"{document_id}: gaps must map claim ids to specific remaining gaps")
            gaps = {}
        ids = {claim_id for claim_id, _, _ in extracted}
        for claim_id, reason in gaps.items():
            if claim_id not in ids:
                errors.append(f"{document_id}#{claim_id}: gap refers to an unknown claim")
            if not isinstance(reason, str) or not reason.strip():
                errors.append(f"{document_id}#{claim_id}: gap requires a nonempty reason")
        for claim_id, title, line in extracted:
            claims.append({
                "id": claim_id, "ref": f"{document_id}#{claim_id}",
                "view_id": f"product/{document_id}/{claim_id}", "title": title,
                "source": {"document": document_id, "path": relative, "section": section, "line": line},
                "gap": gaps.get(claim_id) if isinstance(gaps.get(claim_id), str) else None,
                "links": [], "execution_status": "not_evaluated",
            })
    return claims


def named_go_tests(repo: Path) -> set[str]:
    """Read source declarations only; never traverse runtime/dependency caches."""
    names = set()
    excluded = {"node_modules", "vendor", "third_party", "target", "dist", "build"}
    for directory, subdirs, files in os.walk(repo):
        subdirs[:] = [name for name in subdirs if not name.startswith(".") and name not in excluded]
        for filename in files:
            if not filename.endswith("_test.go"):
                continue
            text = (Path(directory) / filename).read_text(encoding="utf-8")
            names.update(re.findall(r"^func\s+(Test[A-Za-z0-9_]+)\s*\(\s*\w+\s+\*testing\.T\s*\)", text, re.MULTILINE))
    return names


def validate_links(raw, label: str, claims: dict, errors: list[str]) -> list[dict]:
    if not isinstance(raw, list):
        errors.append(f"{label}: verifies must be a list of claim/detail mappings")
        return []
    links, seen = [], set()
    for item in raw:
        if not isinstance(item, dict) or set(item) != {"claim", "detail"}:
            errors.append(f"{label}: verifies entries require exactly claim and detail")
            continue
        ref, detail = item["claim"], item["detail"]
        if not isinstance(ref, str) or ref not in claims:
            errors.append(f"{label}: unknown product claim {ref!r}")
            continue
        if not isinstance(detail, str) or not detail.strip():
            errors.append(f"{label}: {ref} requires a nonempty assertion detail")
            continue
        if ref in seen:
            errors.append(f"{label}: duplicate claim link {ref}")
            continue
        seen.add(ref)
        links.append({"claim": ref, "detail": detail, "source": claims[ref]["source"]})
    return links


def build_product(root: Path, registrations, states: list[dict]) -> dict:
    errors = []
    repo = root.resolve().parent.parent
    claims = load_claims(repo, registrations, errors)
    by_ref = {claim["ref"]: claim for claim in claims}
    tests = None
    for state in states:
        if "verifies" in state:
            errors.append(f"{state['id']}: node-level verifies is forbidden; attach it to a verification case")
        for item in [state] + state.get("processes", []):
            for tag in item["views"]:
                if tag.startswith("product/"):
                    errors.append(f"{item['id']}: handwritten product membership is forbidden: {tag}")
        raw_construct = state.get("construct_verifies", [])
        state["construct_verifies"] = validate_links(raw_construct, state["id"] + " construct", by_ref, errors)
        if raw_construct and not state["executable"]:
            errors.append(f"{state['id']}: construct_verifies requires a real executable scene construct and buildable ancestors")
            state["construct_verifies"] = []
        for link in state["construct_verifies"]:
            by_ref[link["claim"]]["links"].append({
                "case_id": f"{state['id']}::construct", "state": state["id"], "kind": "construct",
                "file": state["construct"], "detail": link["detail"],
            })
        for case in state.get("processes", []):
            case["verifies"] = validate_links(case.get("verifies", []), case["id"], by_ref, errors)
            if case["verifies"] and case["surface"] == "feature" and not state["executable"]:
                errors.append(f"{case['id']}: probe verifies requires an executable scene fixture; reference-only features are not evidence")
                case["verifies"] = []
            if case["verifies"] and case["surface"] == "go-test":
                if tests is None:
                    tests = named_go_tests(repo)
                refs = case["evidence"]
                bad = [ref for ref in refs if not isinstance(ref, str) or not re.fullmatch(r"Test[A-Za-z0-9_]+", ref)]
                absent = [ref for ref in refs if isinstance(ref, str) and ref.startswith("Test") and ref not in tests]
                if not refs or bad:
                    errors.append(f"{case['id']}: product evidence requires named Go tests, not file placeholders: {bad}")
                if absent:
                    errors.append(f"{case['id']}: named Go test declarations not found: {', '.join(absent)}")
                if not refs or bad or absent:
                    case["verifies"] = []
            for link in case["verifies"]:
                evidence = {"case_id": case["id"], "state": state["id"], "kind": case["surface"], "detail": link["detail"]}
                evidence["file" if case["surface"] == "feature" else "tests"] = case.get("file", case.get("evidence"))
                by_ref[link["claim"]]["links"].append(evidence)
    for claim in claims:
        linked, gap = bool(claim["links"]), bool(claim["gap"])
        claim["status"] = "linked_with_gap" if linked and gap else "linked" if linked else "gap" if gap else "unlinked"
        if not linked and not gap:
            errors.append(f"{claim['ref']}: product claim has neither an actual case link nor an explicit gap")
    return {"family": "product", "claims": claims, "errors": errors,
            "execution_status": "not_evaluated", "registered_documents": list(registrations) if isinstance(registrations, dict) else []}


def product_definitions(product: dict) -> dict:
    return {claim["view_id"]: {"family": "product", "title": claim["title"],
                              "description": claim["ref"], "claim": claim["ref"], "source": claim["source"]}
            for claim in product["claims"]}


def inventory_text(product: dict) -> str:
    lines = ["Product claims — declaration links only; execution not evaluated"]
    for claim in product["claims"]:
        lines.extend(claim_text(claim))
    lines.extend(f"ERROR: {error}" for error in product["errors"])
    return "\n".join(lines) + "\n"


def claim_text(claim: dict) -> list[str]:
    source = claim["source"]
    lines = [f"{claim['view_id']}: {claim['title']} [{claim['status']}]",
             f"  source: {source['path']}:{source['line']} ({claim['ref']})"]
    if "selected_link_count" in claim:
        lines.append(f"  display selection: {claim['selected_link_count']} of {claim['declared_link_count']} declared links")
    for link in claim["links"]:
        lines.append(f"  {link['case_id']}: {link['detail']}")
    if claim["gap"]:
        lines.append(f"  gap: {claim['gap']}")
    return lines
