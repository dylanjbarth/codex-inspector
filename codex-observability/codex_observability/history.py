from __future__ import annotations

from collections import Counter
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import sqlite3
from typing import Any, Callable

from .coaching import build_global_coaching, classify_task
from .compat import CompatibilityError, CompatibilityResult
from .data import build_session_report
from .redact import redact_text


CACHE_FILENAME = "history-0.2.sqlite"


def _read_connection(path: Path) -> sqlite3.Connection:
    connection = sqlite3.connect(f"file:{path.as_posix()}?mode=ro", uri=True)
    connection.row_factory = sqlite3.Row
    connection.execute("PRAGMA query_only = ON")
    return connection


def _state_inventory(state_db: Path) -> tuple[list[dict[str, Any]], list[dict[str, str]]]:
    connection = _read_connection(state_db)
    try:
        columns = {row[1] for row in connection.execute("PRAGMA table_info(threads)")}
        created_ms = "created_at_ms" if "created_at_ms" in columns else "created_at * 1000"
        updated_ms = "updated_at_ms" if "updated_at_ms" in columns else "updated_at * 1000"
        rows = connection.execute(
            f"""
            SELECT id, rollout_path, created_at, updated_at, {created_ms} AS created_at_ms,
                   {updated_ms} AS updated_at_ms, source, model_provider, cwd, tokens_used,
                   archived, model, reasoning_effort, cli_version
            FROM threads
            ORDER BY updated_at_ms DESC, id DESC
            """
        ).fetchall()
        edges = connection.execute(
            "SELECT parent_thread_id, child_thread_id, status FROM thread_spawn_edges"
        ).fetchall()
    finally:
        connection.close()
    return ([dict(row) for row in rows], [dict(row) for row in edges])


def _source_class(source: str, *, is_spawned_child: bool) -> tuple[str, str | None]:
    if is_spawned_child:
        return "spawned", None
    try:
        parsed = json.loads(source)
    except (TypeError, json.JSONDecodeError):
        return "top-level", None
    if not isinstance(parsed, dict) or "subagent" not in parsed:
        return "top-level", None
    subagent = parsed["subagent"]
    if isinstance(subagent, dict) and isinstance(subagent.get("thread_spawn"), dict):
        parent = subagent["thread_spawn"].get("parent_thread_id")
        return "spawned", str(parent) if parent else None
    return "internal", None


def _fingerprint(
    row: dict[str, Any], *, adapter_version: str, policy_version: str
) -> str:
    rollout = Path(str(row["rollout_path"])).expanduser()
    try:
        stat = rollout.stat()
        file_fingerprint: tuple[int | None, int | None] = (stat.st_size, stat.st_mtime_ns)
    except OSError:
        file_fingerprint = (None, None)
    allowed = {
        "id": row["id"],
        "rollout_size": file_fingerprint[0],
        "rollout_mtime_ns": file_fingerprint[1],
        "updated_at_ms": row["updated_at_ms"],
        "tokens_used": row["tokens_used"],
        "archived": row["archived"],
        "model": row["model"],
        "reasoning_effort": row["reasoning_effort"],
        "source": row["source"],
        "parent_id": row.get("parent_id"),
        "edge_status": row.get("edge_status"),
        "adapter_version": adapter_version,
        "policy_version": policy_version,
    }
    return hashlib.sha256(
        json.dumps(allowed, sort_keys=True, separators=(",", ":")).encode("utf-8")
    ).hexdigest()


def _summary_from_report(
    report: dict[str, Any], row: dict[str, Any], source_class: str, parent_id: str | None
) -> dict[str, Any]:
    session = report["session"]
    tools = report["toolActivity"]
    context = report["contextAndReasoning"]
    files = report["filesAndVerification"]
    reliability = report["reliability"]
    return {
        "id": session["id"],
        "createdAt": row["created_at_ms"],
        "updatedAt": row["updated_at_ms"],
        "observedStart": session.get("observedStart"),
        "observedEnd": session.get("observedEnd"),
        "status": session["status"],
        "sourceClass": source_class,
        "parentId": parent_id,
        "archived": bool(row["archived"]),
        "model": row["model"],
        "reasoningEffort": row["reasoning_effort"],
        "speed": report["sessionSpeed"]["classification"],
        "speedChanges": report["sessionSpeed"]["changes"],
        "workspace": Path(str(row["cwd"])).name or "/",
        "totalTokens": int(row["tokens_used"] or 0),
        "inputTokens": report["tokenEfficiency"]["inputTokens"],
        "cachedInputTokens": report["tokenEfficiency"]["cachedInputTokens"],
        "outputTokens": report["tokenEfficiency"]["outputTokens"],
        "reasoningOutputTokens": report["tokenEfficiency"]["reasoningOutputTokens"],
        "turnCount": session["turnCount"],
        "toolCalls": tools["totalCalls"],
        "failures": tools["failures"],
        "incompleteTools": tools["incomplete"],
        "toolCategories": tools["byCategory"],
        "toolCategoryCount": len(tools["byCategory"]),
        "compactions": context["compactions"],
        "modelChanged": context.get("modelChanged", False),
        "effortChanged": context.get("effortChanged", False),
        "fileChangesObserved": files["fileChangesObserved"],
        "verificationObserved": files["verificationObserved"],
        "readCalls": files.get("readCalls", 0),
        "readCategoryCount": len(files.get("readCategories", [])),
        "coverage": reliability["coverage"],
        "malformedLines": reliability["malformedLines"],
        "unknownEventTypes": len(reliability["unknownEventTypes"]),
        "tokenMismatch": reliability["tokenMismatch"],
        "coachingEligible": reliability.get("coachingEligible", False),
        "analysisError": None,
        "evidence": [f"state:threads:{session['id']}", f"rollout:session:{session['id']}"],
    }


def _unavailable_summary(
    row: dict[str, Any], source_class: str, parent_id: str | None, error: str
) -> dict[str, Any]:
    return {
        "id": row["id"],
        "createdAt": row["created_at_ms"],
        "updatedAt": row["updated_at_ms"],
        "observedStart": None,
        "observedEnd": None,
        "status": "unavailable",
        "sourceClass": source_class,
        "parentId": parent_id,
        "archived": bool(row["archived"]),
        "model": row["model"],
        "reasoningEffort": row["reasoning_effort"],
        "speed": "Unavailable",
        "speedChanges": 0,
        "workspace": Path(str(row["cwd"])).name or "/",
        "totalTokens": int(row["tokens_used"] or 0),
        "inputTokens": None,
        "cachedInputTokens": None,
        "outputTokens": None,
        "reasoningOutputTokens": None,
        "turnCount": 0,
        "toolCalls": 0,
        "failures": 0,
        "incompleteTools": 0,
        "toolCategories": {},
        "toolCategoryCount": 0,
        "compactions": 0,
        "modelChanged": False,
        "effortChanged": False,
        "fileChangesObserved": False,
        "verificationObserved": False,
        "readCalls": 0,
        "readCategoryCount": 0,
        "coverage": "unavailable",
        "malformedLines": 0,
        "unknownEventTypes": 0,
        "tokenMismatch": False,
        "coachingEligible": False,
        "analysisError": "rollout-unavailable",
        "evidence": [f"state:threads:{row['id']}"],
    }


def _cache_connection(path: Path) -> sqlite3.Connection:
    connection = sqlite3.connect(path)
    connection.execute("PRAGMA foreign_keys = ON")
    connection.execute(
        """
        CREATE TABLE IF NOT EXISTS session_summaries (
            session_id TEXT PRIMARY KEY,
            fingerprint TEXT NOT NULL,
            summary_json TEXT NOT NULL
        )
        """
    )
    connection.execute(
        "CREATE TABLE IF NOT EXISTS cache_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)"
    )
    return connection


def _parse_iso(value: str | None) -> datetime | None:
    if not value:
        return None
    try:
        return datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None


def _children_overlap(children: list[dict[str, Any]]) -> bool:
    intervals: list[tuple[datetime, datetime]] = []
    for child in children:
        start = _parse_iso(child.get("observedStart"))
        end = _parse_iso(child.get("observedEnd"))
        if start and end:
            intervals.append((start, end))
    intervals.sort()
    return any(next_start < current_end for (_, current_end), (next_start, _) in zip(intervals, intervals[1:]))


def _attach_graph_and_signatures(
    summaries: list[dict[str, Any]], edges: list[dict[str, str]], policy: dict[str, Any]
) -> None:
    by_id = {item["id"]: item for item in summaries}
    edge_status = {item["child_thread_id"]: item["status"] for item in edges}
    children_by_parent: dict[str, list[dict[str, Any]]] = {}
    for edge in edges:
        child = by_id.get(edge["child_thread_id"])
        if child:
            child["parentId"] = edge["parent_thread_id"]
            children_by_parent.setdefault(edge["parent_thread_id"], []).append(child)
    for summary in summaries:
        children = children_by_parent.get(summary["id"], [])
        completed = sum(
            child["status"] == "complete" or edge_status.get(child["id"]) == "closed"
            for child in children
        )
        summary["spawnedChildren"] = len(children)
        summary["completedChildren"] = completed
        summary["childCompletionRatio"] = completed / len(children) if children else 1.0
        summary["childrenOverlapped"] = _children_overlap(children)
        summary["taskSignature"] = classify_task(summary, policy)


def refresh_history(
    *,
    compatibility: CompatibilityResult,
    codex_home: Path,
    cache_dir: Path,
    policy: dict[str, Any],
    limit: int,
    rebuild: bool = False,
    progress: Callable[[int, int], None] | None = None,
) -> dict[str, Any]:
    cache_dir = cache_dir.expanduser().resolve()
    cache_dir.mkdir(parents=True, exist_ok=True)
    cache_path = cache_dir / CACHE_FILENAME
    rows, edges = _state_inventory(compatibility.state_database)
    parent_by_child = {edge["child_thread_id"]: edge["parent_thread_id"] for edge in edges}
    edge_status_by_child = {edge["child_thread_id"]: edge["status"] for edge in edges}
    child_ids = set(parent_by_child)
    connection = _cache_connection(cache_path)
    try:
        if rebuild:
            connection.execute("DELETE FROM session_summaries")
        existing = {
            row[0]: row[1]
            for row in connection.execute("SELECT session_id, fingerprint FROM session_summaries")
        }
        hits = 0
        misses = 0
        total = len(rows)
        adapter_version = str(compatibility.contract.get("adapter_version"))
        for row in rows:
            row["parent_id"] = parent_by_child.get(row["id"])
            row["edge_status"] = edge_status_by_child.get(row["id"])
            source_class, source_parent = _source_class(
                str(row["source"]), is_spawned_child=row["id"] in child_ids
            )
            parent_id = parent_by_child.get(row["id"], source_parent)
            fingerprint = _fingerprint(
                row,
                adapter_version=adapter_version,
                policy_version=policy["policy_version"],
            )
            if existing.get(row["id"]) == fingerprint:
                hits += 1
            else:
                try:
                    report = build_session_report(
                        compatibility=compatibility,
                        codex_home=codex_home,
                        session=row["id"],
                        content="metadata",
                    )
                    summary = _summary_from_report(report, row, source_class, parent_id)
                except CompatibilityError as exc:
                    summary = _unavailable_summary(row, source_class, parent_id, str(exc))
                connection.execute(
                    """
                    INSERT INTO session_summaries(session_id, fingerprint, summary_json)
                    VALUES (?, ?, ?)
                    ON CONFLICT(session_id) DO UPDATE SET
                        fingerprint = excluded.fingerprint,
                        summary_json = excluded.summary_json
                    """,
                    (row["id"], fingerprint, json.dumps(summary, sort_keys=True, separators=(",", ":"))),
                )
                misses += 1
                if progress and misses % 25 == 0:
                    progress(misses, total)
        current_ids = {row["id"] for row in rows}
        stale_ids = set(existing) - current_ids
        if stale_ids:
            connection.executemany(
                "DELETE FROM session_summaries WHERE session_id = ?",
                [(session_id,) for session_id in sorted(stale_ids)],
            )
        if progress and misses and misses % 25:
            progress(misses, total)
        refreshed_at = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        connection.execute(
            "INSERT OR REPLACE INTO cache_meta(key, value) VALUES ('refreshed_at', ?)",
            (refreshed_at,),
        )
        connection.commit()
        summaries = [
            json.loads(row[0])
            for row in connection.execute("SELECT summary_json FROM session_summaries")
        ]
    finally:
        connection.close()

    _attach_graph_and_signatures(summaries, edges, policy)
    top_level = [item for item in summaries if item["sourceClass"] == "top-level"]
    coaching = build_global_coaching(top_level, policy)
    recent = sorted(top_level, key=lambda item: (item["updatedAt"], item["id"]), reverse=True)[:max(1, min(limit, 500))]
    model_counts = Counter(str(item["model"] or "Unavailable") for item in top_level)
    effort_counts = Counter(str(item["reasoningEffort"] or "Unavailable") for item in top_level)
    tool_counts: Counter[str] = Counter()
    for item in top_level:
        tool_counts.update(item["toolCategories"])
    population = {
        "total": len(summaries),
        "topLevel": len(top_level),
        "spawned": sum(item["sourceClass"] == "spawned" for item in summaries),
        "internal": sum(item["sourceClass"] == "internal" for item in summaries),
        "archived": sum(item["archived"] for item in summaries),
        "analyzed": sum(item["analysisError"] is None for item in summaries),
    }
    aggregates = {
        "tokens": {
            "allThreads": sum(item["totalTokens"] for item in summaries),
            "topLevel": sum(item["totalTokens"] for item in top_level),
            "cachedInput": sum(item["cachedInputTokens"] or 0 for item in top_level),
            "input": sum(item["inputTokens"] or 0 for item in top_level),
            "output": sum(item["outputTokens"] or 0 for item in top_level),
            "reasoningOutput": sum(item["reasoningOutputTokens"] or 0 for item in top_level),
        },
        "models": dict(model_counts.most_common()),
        "reasoningEffort": dict(effort_counts.most_common()),
        "tools": dict(tool_counts.most_common()),
        "toolCalls": sum(item["toolCalls"] for item in top_level),
        "toolFailures": sum(item["failures"] for item in top_level),
        "spawnedChildren": sum(item["spawnedChildren"] for item in top_level),
        "changedSessions": sum(item["fileChangesObserved"] for item in top_level),
        "verifiedChangeSessions": sum(
            item["fileChangesObserved"] and item["verificationObserved"] for item in top_level
        ),
        "workspaces": sorted({item["workspace"] for item in top_level}),
    }
    return {
        "dashboardVersion": "0.2.0",
        "population": population,
        "aggregates": aggregates,
        "coaching": coaching,
        "sessions": recent,
        "sessionSummaries": summaries,
        "reliability": {
            "coachingEligible": sum(item["coachingEligible"] for item in top_level),
            "partial": sum(item["coverage"] == "partial" for item in summaries),
            "unavailable": sum(item["analysisError"] is not None for item in summaries),
        },
        "cache": {
            "path": redact_text(cache_path),
            "hits": hits,
            "misses": misses,
            "purged": len(stale_ids),
            "rebuild": rebuild,
            "refreshedAt": refreshed_at,
            "stateThreadsAtRefresh": len(rows),
            "stateTotalTokensAtRefresh": sum(int(row["tokens_used"] or 0) for row in rows),
        },
        "provenance": {
            **compatibility.as_dict(),
            "stateDatabase": redact_text(compatibility.state_database),
        },
    }


def session_summary(dashboard: dict[str, Any], session_id: str) -> dict[str, Any] | None:
    for summary in dashboard["sessionSummaries"]:
        if summary["id"] == session_id:
            return summary
    return None


def load_cached_summary(cache_dir: Path, session_id: str) -> dict[str, Any] | None:
    cache_path = cache_dir.expanduser().resolve() / CACHE_FILENAME
    if not cache_path.is_file():
        return None
    connection = sqlite3.connect(f"file:{cache_path.as_posix()}?mode=ro", uri=True)
    connection.execute("PRAGMA query_only = ON")
    try:
        row = connection.execute(
            "SELECT summary_json FROM session_summaries WHERE session_id = ?", (session_id,)
        ).fetchone()
    finally:
        connection.close()
    return json.loads(row[0]) if row else None
