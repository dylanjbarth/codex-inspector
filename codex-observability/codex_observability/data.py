from __future__ import annotations

from collections import Counter
from dataclasses import dataclass, field
from datetime import datetime
import json
from pathlib import Path
import re
import sqlite3
from typing import Any, Iterable

from .compat import CompatibilityError, CompatibilityResult
from .redact import redact_text


KNOWN_ROOT_TYPES = {
    "session_meta",
    "response_item",
    "inter_agent_communication",
    "inter_agent_communication_metadata",
    "compacted",
    "turn_context",
    "world_state",
    "event_msg",
}

KNOWN_EVENT_TYPES = {
    "error", "warning", "guardian_warning", "model_reroute", "model_verification",
    "turn_moderation_metadata", "safety_buffering", "context_compacted", "thread_rolled_back",
    "task_started", "turn_started", "thread_settings_applied", "task_complete", "turn_complete",
    "token_count", "agent_message", "user_message", "agent_reasoning", "agent_reasoning_raw_content",
    "agent_reasoning_section_break", "session_configured", "thread_goal_updated", "mcp_startup_update",
    "mcp_startup_complete", "mcp_tool_call_begin", "mcp_tool_call_end", "web_search_begin",
    "web_search_end", "image_generation_begin", "image_generation_end", "exec_command_begin",
    "exec_command_output_delta", "terminal_interaction", "exec_command_end", "view_image_tool_call",
    "exec_approval_request", "request_permissions", "request_user_input", "dynamic_tool_call_request",
    "dynamic_tool_call_response", "elicitation_request", "apply_patch_approval_request",
    "guardian_assessment", "deprecation_notice", "stream_error", "patch_apply_begin",
    "patch_apply_updated", "patch_apply_end", "turn_diff", "plan_update", "turn_aborted",
    "shutdown_complete", "entered_review_mode", "exited_review_mode", "raw_response_item",
    "item_started", "item_completed", "hook_started", "hook_completed", "agent_message_content_delta",
    "plan_delta", "reasoning_content_delta", "reasoning_raw_content_delta", "collab_agent_spawn_begin",
    "collab_agent_spawn_end", "collab_agent_interaction_begin", "collab_agent_interaction_end",
    "collab_waiting_begin", "collab_waiting_end", "collab_close_begin", "collab_close_end",
    "collab_resume_begin", "collab_resume_end", "sub_agent_activity", "realtime_conversation_started",
    "realtime_conversation_realtime", "realtime_conversation_closed", "realtime_conversation_sdp",
    "realtime_conversation_list_voices_response",
}


CALL_TYPES = {"custom_tool_call", "function_call", "local_shell_call"}
OUTPUT_TYPES = {"custom_tool_call_output", "function_call_output", "local_shell_call_output"}


@dataclass
class ToolCall:
    call_id: str
    name: str
    category: str
    started_at: str | None = None
    completed_at: str | None = None
    duration_ms: int | None = None
    status: str = "incomplete"
    input_preview: str = ""
    output_preview: str = ""
    evidence: list[str] = field(default_factory=list)

    def as_dict(self, include_details: bool) -> dict[str, Any]:
        result: dict[str, Any] = {
            "id": self.call_id,
            "name": self.name,
            "category": self.category,
            "status": self.status,
            "startedAt": self.started_at,
            "completedAt": self.completed_at,
            "durationMs": self.duration_ms,
            "evidence": self.evidence,
        }
        if include_details:
            result["inputPreview"] = self.input_preview
            result["outputPreview"] = self.output_preview
        return result


def _parse_time(value: str | None) -> datetime | None:
    if not value:
        return None
    try:
        return datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None


def _duration_ms(start: str | None, end: str | None) -> int | None:
    start_dt = _parse_time(start)
    end_dt = _parse_time(end)
    if start_dt is None or end_dt is None:
        return None
    return max(0, round((end_dt - start_dt).total_seconds() * 1000))


def categorize_tool(name: str) -> str:
    lowered = name.lower()
    if any(part in lowered for part in ("exec", "shell", "terminal", "bash", "command")):
        return "Shell"
    if any(part in lowered for part in ("patch", "file", "resource", "image", "read_mcp")):
        return "Files & patches"
    if any(part in lowered for part in ("search", "web", "weather", "finance", "sports")):
        return "Search & web"
    if "mcp" in lowered or "app" in lowered or "plugin" in lowered:
        return "MCP & apps"
    if "browser" in lowered or "computer" in lowered or "chrome" in lowered:
        return "Browser & computer use"
    if any(part in lowered for part in ("plan", "user_input", "request_user")):
        return "Planning & interaction"
    if any(part in lowered for part in ("agent", "collab", "delegate", "message")):
        return "Multi-agent"
    return "Other"


def _call_is_write(call: ToolCall) -> bool:
    name = call.name.lower()
    if any(part in name for part in ("apply_patch", "patch_apply", "write_file", "edit_file", "create_file", "delete_file", "move_file")):
        return True
    if call.category != "Shell":
        return False
    return bool(
        re.search(
            r"(?i)(?:\b(?:rm|mv|cp|mkdir|touch|chmod|chown|git\s+(?:add|commit)|npm\s+install|pip\s+install)\b|(?:^|[^<])>>?)",
            call.input_preview,
        )
    )


def _call_read_kind(call: ToolCall) -> str | None:
    name = call.name.lower()
    if call.category == "Search & web":
        return "search"
    if any(part in name for part in ("read", "view", "open", "list", "find", "fetch", "get_resource")):
        return "file-read"
    if call.category == "Shell" and re.search(
        r"(?i)\b(?:rg|sed|cat|ls|find|head|tail|wc|stat|git\s+(?:status|diff|log|show)|sqlite3)\b",
        call.input_preview,
    ):
        return "shell-read"
    return None


def _call_is_verification(call: ToolCall) -> bool:
    if call.category != "Shell":
        return False
    return any(
        word in call.input_preview.lower()
        for word in ("test", "pytest", "unittest", "lint", "build", "check", "typecheck")
    )


def classify_tool_output(value: Any) -> str:
    if isinstance(value, dict):
        if value.get("success") is False:
            return "failure"
        exit_code = value.get("exit_code")
        if isinstance(exit_code, int) and exit_code != 0:
            return "failure"
        status = str(value.get("status", "")).lower()
        if status in {"error", "failed", "failure", "cancelled", "canceled"}:
            return "failure"
        if value.get("error") not in (None, "", False):
            return "failure"
        return "success"
    if isinstance(value, str):
        stripped = value.strip()
        if stripped.startswith(("{", "[")):
            try:
                return classify_tool_output(json.loads(stripped))
            except json.JSONDecodeError:
                pass
        if re.match(r"(?is)^(error|failed|failure|exception|traceback)\b", stripped):
            return "failure"
    return "success"


def _speed_for_service_tier(value: Any) -> str | None:
    if value is None:
        return "Standard"
    normalized = str(value).strip().lower()
    if normalized in {"fast", "priority"}:
        return "Fast"
    if normalized == "default":
        return "Standard"
    return None


def _summarize_session_speed(observations: list[dict[str, Any]]) -> dict[str, Any]:
    known = {item["speed"] for item in observations if item.get("speed")}
    unsupported = any(item.get("speed") is None for item in observations)
    if not observations or unsupported:
        classification = "Unavailable"
        provenance = "unavailable" if not observations else "observed-partial"
    elif known == {"Fast"}:
        classification = "Fast"
        provenance = "observed"
    elif known == {"Standard"}:
        classification = "Standard"
        provenance = "observed"
    elif known == {"Fast", "Standard"}:
        classification = "Mixed"
        provenance = "observed"
    else:
        classification = "Unavailable"
        provenance = "unavailable"

    speed_sequence = [item.get("speed") for item in observations]
    changes = sum(
        previous != current
        for previous, current in zip(speed_sequence, speed_sequence[1:])
        if previous is not None and current is not None
    )
    observed_tiers: list[str] = []
    for item in observations:
        raw_tier = item.get("serviceTier")
        tier_label = "default" if raw_tier is None else str(raw_tier)
        if tier_label not in observed_tiers:
            observed_tiers.append(tier_label)
    return {
        "classification": classification,
        "changes": changes,
        "observedTiers": observed_tiers,
        "observations": observations,
        "provenance": provenance,
    }


def _read_state_row(state_db: Path, session: str) -> sqlite3.Row:
    connection = sqlite3.connect(f"file:{state_db.as_posix()}?mode=ro", uri=True)
    connection.row_factory = sqlite3.Row
    connection.execute("PRAGMA query_only = ON")
    try:
        if session == "latest":
            columns = {
                row[1] for row in connection.execute("PRAGMA table_info(threads)").fetchall()
            }
            order = "updated_at_ms DESC, updated_at DESC, id DESC" if "updated_at_ms" in columns else "updated_at DESC, id DESC"
            row = connection.execute(
                f"SELECT * FROM threads ORDER BY {order} LIMIT 1"
            ).fetchone()
        else:
            row = connection.execute("SELECT * FROM threads WHERE id = ?", (session,)).fetchone()
    finally:
        connection.close()
    if row is None:
        raise CompatibilityError(f"session not found: {session}")
    return row


def list_sessions(state_db: Path, limit: int) -> list[dict[str, Any]]:
    connection = sqlite3.connect(f"file:{state_db.as_posix()}?mode=ro", uri=True)
    connection.row_factory = sqlite3.Row
    connection.execute("PRAGMA query_only = ON")
    try:
        columns = {
            row[1] for row in connection.execute("PRAGMA table_info(threads)").fetchall()
        }
        order = "updated_at_ms DESC, updated_at DESC, id DESC" if "updated_at_ms" in columns else "updated_at DESC, id DESC"
        rows = connection.execute(
            f"SELECT id, created_at, updated_at, source, model_provider, cwd, tokens_used, archived, model, reasoning_effort, cli_version FROM threads ORDER BY {order} LIMIT ?",
            (max(1, min(limit, 500)),),
        ).fetchall()
    finally:
        connection.close()
    return [
        {
            "id": row["id"],
            "createdAt": row["created_at"],
            "updatedAt": row["updated_at"],
            "source": row["source"],
            "modelProvider": row["model_provider"],
            "model": row["model"],
            "reasoningEffort": row["reasoning_effort"],
            "workspace": Path(row["cwd"]).name or "/",
            "totalTokens": int(row["tokens_used"] or 0),
            "archived": bool(row["archived"]),
            "cliVersion": row["cli_version"],
        }
        for row in rows
    ]


def _safe_rollout_path(codex_home: Path, raw_path: str) -> Path:
    path = Path(raw_path).expanduser().resolve()
    allowed = [
        (codex_home / "sessions").resolve(),
        (codex_home / "archived_sessions").resolve(),
    ]
    if not any(path.is_relative_to(root) for root in allowed):
        raise CompatibilityError(f"rollout path is outside Codex session roots: {path}")
    if not path.is_file():
        raise CompatibilityError(f"rollout file is missing: {path}")
    return path


def _iter_jsonl(path: Path) -> Iterable[tuple[int, dict[str, Any] | None]]:
    with path.open("r", encoding="utf-8", errors="replace") as handle:
        for ordinal, line in enumerate(handle, start=1):
            try:
                value = json.loads(line)
            except json.JSONDecodeError:
                yield ordinal, None
                continue
            yield ordinal, value if isinstance(value, dict) else None


def build_session_report(
    *,
    compatibility: CompatibilityResult,
    codex_home: Path,
    session: str,
    content: str,
) -> dict[str, Any]:
    row = _read_state_row(compatibility.state_database, session)
    rollout_path = _safe_rollout_path(codex_home, row["rollout_path"])
    tool_calls: dict[str, ToolCall] = {}
    tool_outputs: dict[str, tuple[str | None, Any, str]] = {}
    event_begins: dict[str, tuple[str, str | None, str]] = {}
    malformed = 0
    unknown_roots: Counter[str] = Counter()
    unknown_events: Counter[str] = Counter()
    final_usage: dict[str, int] | None = None
    turns: set[str] = set()
    turn_contexts: list[dict[str, Any]] = []
    skills_seen: dict[str, str] = {}
    subagents: dict[str, dict[str, Any]] = {}
    compacted_items = 0
    compacted_events = 0
    last_turn_state = "unknown"
    speed_observations: list[dict[str, Any]] = []
    timeline: list[dict[str, Any]] = []
    first_time: str | None = None
    last_time: str | None = None

    for ordinal, record in _iter_jsonl(rollout_path):
        if record is None:
            malformed += 1
            continue
        timestamp = record.get("timestamp")
        if isinstance(timestamp, str):
            first_time = first_time or timestamp
            last_time = timestamp
        root_type = str(record.get("type", "unknown"))
        payload = record.get("payload")
        if not isinstance(payload, dict):
            payload = {}
        evidence = f"rollout:{rollout_path.name}:{ordinal}"

        if root_type not in KNOWN_ROOT_TYPES:
            unknown_roots[root_type] += 1
        if root_type == "turn_context":
            turn_id = payload.get("turn_id")
            if isinstance(turn_id, str):
                turns.add(turn_id)
            turn_contexts.append(
                {
                    "turnId": turn_id,
                    "model": payload.get("model"),
                    "reasoningEffort": payload.get("effort"),
                    "contextWindow": payload.get("context_window") or payload.get("model_context_window"),
                    "evidence": evidence,
                }
            )
        elif root_type == "compacted":
            compacted_items += 1
            timeline.append({"timestamp": timestamp, "type": "compaction", "evidence": evidence})
        elif root_type == "event_msg":
            event_type = str(payload.get("type", "unknown"))
            if event_type not in KNOWN_EVENT_TYPES:
                unknown_events[event_type] += 1
            if event_type in {"task_started", "turn_started", "task_complete", "turn_complete", "turn_aborted"}:
                turn_id = payload.get("turn_id")
                if isinstance(turn_id, str):
                    turns.add(turn_id)
            if event_type in {"task_started", "turn_started"}:
                last_turn_state = "in-progress-or-incomplete"
            elif event_type in {"task_complete", "turn_complete"}:
                last_turn_state = "complete"
            elif event_type == "turn_aborted":
                last_turn_state = "aborted"
            if event_type in {"context_compacted"}:
                compacted_events += 1
            if event_type == "token_count":
                info = payload.get("info")
                usage = info.get("total_token_usage") if isinstance(info, dict) else None
                if isinstance(usage, dict):
                    final_usage = {
                        key: int(usage.get(key, 0) or 0)
                        for key in (
                            "input_tokens",
                            "cached_input_tokens",
                            "output_tokens",
                            "reasoning_output_tokens",
                            "total_tokens",
                        )
                    }
            if event_type in {"thread_settings_applied", "session_configured"}:
                settings = payload.get("thread_settings")
                source = settings if isinstance(settings, dict) else payload
                raw_tier = source.get("service_tier")
                observation = {
                    "speed": _speed_for_service_tier(raw_tier),
                    "serviceTier": raw_tier,
                    "timestamp": timestamp,
                    "evidence": evidence,
                }
                if (
                    not speed_observations
                    or speed_observations[-1]["serviceTier"] != observation["serviceTier"]
                ):
                    speed_observations.append(observation)
            if event_type == "collab_agent_spawn_end":
                child_id = payload.get("new_thread_id")
                if isinstance(child_id, str):
                    subagents[child_id] = {
                        "threadId": child_id,
                        "nickname": payload.get("new_agent_nickname"),
                        "role": payload.get("new_agent_role"),
                        "model": payload.get("model"),
                        "reasoningEffort": payload.get("reasoning_effort"),
                        "status": payload.get("status"),
                        "evidence": [evidence],
                    }
            elif event_type == "sub_agent_activity":
                child_id = payload.get("agent_thread_id")
                if isinstance(child_id, str):
                    subagent = subagents.setdefault(
                        child_id,
                        {
                            "threadId": child_id,
                            "nickname": None,
                            "role": None,
                            "model": None,
                            "reasoningEffort": None,
                            "status": None,
                            "evidence": [],
                        },
                    )
                    subagent["activity"] = payload.get("kind")
                    subagent["path"] = payload.get("agent_path")
                    subagent["evidence"].append(evidence)
            _ingest_event_tool(payload, timestamp, evidence, tool_calls, event_begins)
            if event_type not in {"agent_message", "user_message", "agent_reasoning", "agent_reasoning_raw_content", "token_count"}:
                timeline.append({"timestamp": timestamp, "type": event_type, "evidence": evidence})
        elif root_type == "response_item":
            item_type = str(payload.get("type", ""))
            if item_type in CALL_TYPES:
                call_id = str(payload.get("call_id") or payload.get("id") or f"ordinal-{ordinal}")
                name = str(payload.get("name") or payload.get("tool_name") or item_type)
                raw_input = payload.get("input", payload.get("arguments", ""))
                raw_input_text = str(raw_input)
                for match in re.finditer(
                    r"(?:^|[/\\])skills[/\\]([^/\\]+)[/\\]SKILL\.md\b",
                    raw_input_text,
                    flags=re.IGNORECASE,
                ):
                    skills_seen.setdefault(match.group(1), evidence)
                tool_calls.setdefault(
                    call_id,
                    ToolCall(
                        call_id=call_id,
                        name=name,
                        category=categorize_tool(name),
                        started_at=timestamp,
                        input_preview=redact_text(raw_input),
                        evidence=[evidence],
                    ),
                )
            elif item_type in OUTPUT_TYPES:
                call_id = str(payload.get("call_id") or payload.get("id") or f"ordinal-{ordinal}")
                raw_output = payload.get("output", payload.get("result", ""))
                tool_outputs[call_id] = (timestamp, raw_output, evidence)

    for call_id, (timestamp, raw_output, evidence) in tool_outputs.items():
        call = tool_calls.get(call_id)
        if call is None:
            call = ToolCall(call_id, "unknown", "Other", completed_at=timestamp)
            tool_calls[call_id] = call
        call.completed_at = call.completed_at or timestamp
        call.duration_ms = call.duration_ms or _duration_ms(call.started_at, call.completed_at)
        call.output_preview = redact_text(raw_output)
        call.status = classify_tool_output(raw_output)
        call.evidence.append(evidence)

    for call in tool_calls.values():
        if call.status == "incomplete" and call.completed_at:
            call.status = "success"

    usage = final_usage or {key: 0 for key in (
        "input_tokens", "cached_input_tokens", "output_tokens", "reasoning_output_tokens", "total_tokens"
    )}
    state_total = int(row["tokens_used"] or 0)
    input_tokens = usage["input_tokens"]
    cached_tokens = usage["cached_input_tokens"]
    cache_ratio = round(cached_tokens / input_tokens * 100, 2) if input_tokens else None
    calls = sorted(tool_calls.values(), key=lambda call: (call.started_at or "", call.call_id))
    counts_by_category = Counter(call.category for call in calls)
    counts_by_name = Counter(call.name for call in calls)
    failures = sum(call.status == "failure" for call in calls)
    incomplete = sum(call.status == "incomplete" for call in calls)
    duration = _duration_ms(first_time, last_time)
    rollout_total = usage["total_tokens"] if final_usage else None
    mismatch = rollout_total is not None and rollout_total != state_total
    compactions = max(compacted_items, compacted_events)
    coverage_issues = malformed + sum(unknown_roots.values()) + sum(unknown_events.values()) + incomplete
    status = last_turn_state if last_turn_state != "unknown" else "in-progress-or-incomplete"
    model_values = {str(item["model"]) for item in turn_contexts if item.get("model")}
    effort_values = {str(item["reasoningEffort"]) for item in turn_contexts if item.get("reasoningEffort")}
    read_kinds = [kind for call in calls if (kind := _call_read_kind(call))]
    write_calls = [call for call in calls if _call_is_write(call)]
    verification_calls = [call for call in calls if _call_is_verification(call)]
    coaching_eligible = status == "complete" and malformed == 0 and incomplete == 0
    session_speed = _summarize_session_speed(speed_observations)

    report: dict[str, Any] = {
        "reportVersion": "0.2.4",
        "provenance": {
            "codexCliVersion": compatibility.codex_cli_version,
            "sourceCommit": compatibility.source_commit,
            "sourceTag": compatibility.contract.get("source_tag"),
            "adapterVersion": compatibility.contract.get("adapter_version"),
            "stateDatabase": redact_text(compatibility.state_database),
            "rollout": redact_text(rollout_path),
        },
        "session": {
            "id": row["id"],
            "status": status,
            "createdAt": row["created_at"],
            "updatedAt": row["updated_at"],
            "observedDurationMs": duration,
            "observedStart": first_time,
            "observedEnd": last_time,
            "source": row["source"],
            "modelProvider": row["model_provider"],
            "model": row["model"],
            "reasoningEffort": row["reasoning_effort"],
            "workspace": Path(row["cwd"]).name or "/",
            "turnCount": len(turns),
            "archived": bool(row["archived"]),
        },
        "tokenEfficiency": {
            "totalTokens": state_total,
            "inputTokens": input_tokens if final_usage else None,
            "cachedInputTokens": cached_tokens if final_usage else None,
            "uncachedInputTokens": max(0, input_tokens - cached_tokens) if final_usage else None,
            "outputTokens": usage["output_tokens"] if final_usage else None,
            "reasoningOutputTokens": usage["reasoning_output_tokens"] if final_usage else None,
            "cacheRatio": cache_ratio,
            "tokensPerTurn": round(state_total / len(turns), 2) if turns else None,
            "tokensPerToolCall": round(state_total / len(calls), 2) if calls else None,
            "rolloutTotalTokens": rollout_total,
            "reconciliation": "mismatch" if mismatch else "matched" if rollout_total is not None else "partial",
        },
        "sessionSpeed": session_speed,
        "toolActivity": {
            "totalCalls": len(calls),
            "successes": sum(call.status == "success" for call in calls),
            "failures": failures,
            "incomplete": incomplete,
            "byCategory": dict(sorted(counts_by_category.items())),
            "byTool": dict(sorted(counts_by_name.items())),
            "calls": [call.as_dict(content == "redacted") for call in calls],
        },
        "contextAndReasoning": {
            "compactions": compactions,
            "turnContexts": turn_contexts,
            "modelChanged": len(model_values) > 1,
            "effortChanged": len(effort_values) > 1,
        },
        "skillsAndAgents": {
            "skillObservation": "unavailable unless persisted as a supported local event",
            "skills": [
                {
                    "name": name,
                    "state": "instructions-loaded-inferred",
                    "confidence": "inferred",
                    "evidence": evidence,
                }
                for name, evidence in sorted(skills_seen.items())
            ],
            "subagents": list(subagents.values()),
        },
        "filesAndVerification": {
            "fileChangesObserved": bool(write_calls),
            "verificationObserved": bool(verification_calls),
            "writeCalls": len(write_calls),
            "readCalls": len(read_kinds),
            "readCategories": sorted(set(read_kinds)),
        },
        "reliability": {
            "coverage": "partial" if coverage_issues or mismatch or final_usage is None else "complete",
            "malformedLines": malformed,
            "unknownRootTypes": dict(sorted(unknown_roots.items())),
            "unknownEventTypes": sorted(unknown_events),
            "orphanedToolCalls": incomplete,
            "tokenMismatch": mismatch,
            "coachingEligible": coaching_eligible,
        },
        "coaching": {
            "positive": [],
            "improvements": [],
            "policyVersion": None,
            "policySource": None,
        },
        "recommendations": [],
        "timeline": timeline,
    }
    report["recommendations"] = _recommend(report)
    return report


def _ingest_event_tool(
    payload: dict[str, Any],
    timestamp: str | None,
    evidence: str,
    tool_calls: dict[str, ToolCall],
    event_begins: dict[str, tuple[str, str | None, str]],
) -> None:
    event_type = str(payload.get("type", ""))
    begin_names = {
        "exec_command_begin": "exec_command",
        "mcp_tool_call_begin": str(payload.get("tool") or payload.get("name") or "mcp_tool"),
        "web_search_begin": "web_search",
        "patch_apply_begin": "apply_patch",
        "image_generation_begin": "image_generation",
    }
    end_names = {
        "exec_command_end": "exec_command",
        "mcp_tool_call_end": str(payload.get("tool") or payload.get("name") or "mcp_tool"),
        "web_search_end": "web_search",
        "patch_apply_end": "apply_patch",
        "image_generation_end": "image_generation",
    }
    call_id = str(payload.get("call_id") or payload.get("id") or payload.get("tool_use_id") or "")
    if event_type in begin_names and call_id:
        event_begins[call_id] = (begin_names[event_type], timestamp, evidence)
    elif event_type in end_names and call_id:
        name, started_at, begin_evidence = event_begins.get(call_id, (end_names[event_type], None, ""))
        call = tool_calls.setdefault(call_id, ToolCall(call_id, name, categorize_tool(name)))
        call.started_at = call.started_at or started_at
        call.completed_at = call.completed_at or timestamp
        duration = payload.get("duration_ms")
        call.duration_ms = int(duration) if isinstance(duration, (int, float)) else _duration_ms(call.started_at, call.completed_at)
        success = payload.get("success")
        if success is False or payload.get("error"):
            call.status = "failure"
        else:
            call.status = "success"
        call.evidence.extend(item for item in (begin_evidence, evidence) if item)


def _recommend(report: dict[str, Any]) -> list[dict[str, Any]]:
    recommendations: list[dict[str, Any]] = []
    tools = report["toolActivity"]
    files = report["filesAndVerification"]
    context = report["contextAndReasoning"]
    if tools["failures"] >= 2:
        recommendations.append({
            "ruleId": "repeated-tool-failures",
            "confidence": "high",
            "observed": f"{tools['failures']} tool calls failed.",
            "action": "Include the failing command, environment, and exact error in the next request.",
        })
    if files["fileChangesObserved"] and not files["verificationObserved"]:
        recommendations.append({
            "ruleId": "changes-without-verification",
            "confidence": "medium",
            "observed": "File-changing activity was observed without a recognizable verification command.",
            "action": "Ask Codex to run the relevant tests, build, or linter before declaring completion.",
        })
    if context["compactions"] >= 2 or tools["incomplete"] >= 3:
        recommendations.append({
            "ruleId": "context-or-retry-churn",
            "confidence": "medium",
            "observed": "The session had repeated compaction or incomplete tool activity.",
            "action": "Split the next request into a narrower goal with explicit completion checks.",
        })
    return recommendations[:3]
