from __future__ import annotations

import json
from pathlib import Path
from typing import Any, Callable

from .compat import CompatibilityError


def load_policy(path: Path, *, source_kind: str) -> dict[str, Any]:
    try:
        policy = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise CompatibilityError(f"unable to load coaching policy {path}: {exc}") from exc
    if not isinstance(policy, dict) or not isinstance(policy.get("policy_version"), str):
        raise CompatibilityError(f"coaching policy is missing policy_version: {path}")
    for key in ("global_thresholds", "task_signatures", "model_roles", "subagents"):
        if not isinstance(policy.get(key), dict):
            raise CompatibilityError(f"coaching policy is missing {key}: {path}")
    policy["_source_kind"] = source_kind
    return policy


def model_role(model: Any) -> str | None:
    lowered = str(model or "").lower()
    if lowered in {"gpt-5.6", "gpt-5.6-sol"} or lowered.endswith(".gpt-5.6-sol"):
        return "sol"
    if lowered.endswith("gpt-5.6-terra"):
        return "terra"
    if lowered.endswith("gpt-5.6-luna"):
        return "luna"
    return None


def classify_task(summary: dict[str, Any], policy: dict[str, Any]) -> str:
    if not summary.get("coachingEligible"):
        return "unavailable"
    signatures = policy["task_signatures"]
    complex_rule = signatures["complex_open_ended"]
    if (
        summary["turnCount"] >= complex_rule["minimum_turns"]
        or summary["toolCalls"] >= complex_rule["minimum_tool_calls"]
        or summary["toolCategoryCount"] >= complex_rule["minimum_tool_categories"]
        or summary["compactions"] >= complex_rule["minimum_compactions"]
        or (
            summary["fileChangesObserved"]
            and summary["verificationObserved"]
            and summary["toolCalls"] >= complex_rule["substantial_write_minimum_tools"]
        )
    ):
        return "complex-open-ended"

    read_rule = signatures["read_heavy"]
    if (
        not summary["fileChangesObserved"]
        and summary["readCalls"] >= read_rule["minimum_read_calls"]
        and summary["readCategoryCount"] >= read_rule["minimum_read_categories"]
        and summary["failures"] <= read_rule["maximum_failures"]
        and summary["compactions"] <= read_rule["maximum_compactions"]
    ):
        return "read-heavy-exploration"

    clear_rule = signatures["clear_repeatable"]
    if (
        summary["turnCount"] <= clear_rule["maximum_turns"]
        and summary["toolCalls"] <= clear_rule["maximum_tool_calls"]
        and summary["toolCategoryCount"] <= clear_rule["maximum_tool_categories"]
        and summary["failures"] <= clear_rule["maximum_failures"]
        and summary["compactions"] <= clear_rule["maximum_compactions"]
        and not summary.get("modelChanged")
        and not summary.get("effortChanged")
    ):
        return "clear-repeatable"
    return "everyday"


def _delegation_matches(summary: dict[str, Any], policy: dict[str, Any]) -> bool:
    signature = summary["taskSignature"]
    count = summary["spawnedChildren"]
    rules = policy["subagents"]
    if signature == "clear-repeatable":
        return count == 0
    if signature in {"everyday", "read-heavy-exploration"}:
        return count <= rules["everyday_maximum"]
    if signature == "complex-open-ended":
        return (
            rules["complex_minimum_positive"] <= count <= rules["complex_maximum_positive"]
            and summary["childCompletionRatio"] >= rules["minimum_completion_ratio"]
            and summary["childrenOverlapped"]
        )
    return False


def _delegation_excessive(summary: dict[str, Any], policy: dict[str, Any]) -> bool:
    signature = summary["taskSignature"]
    count = summary["spawnedChildren"]
    rules = policy["subagents"]
    if signature == "clear-repeatable" and count >= 2:
        return True
    if signature in {"everyday", "read-heavy-exploration"} and count >= rules["everyday_excessive"]:
        return True
    if count >= 2 and summary["childCompletionRatio"] < rules["majority_incomplete_ratio"]:
        return True
    return count >= rules["large_fanout"] and not summary["childrenOverlapped"]


def _entry(
    *,
    rule_id: str,
    title: str,
    observed: str,
    impact: str,
    confidence: str,
    summaries: list[dict[str, Any]],
    policy: dict[str, Any],
    next_action: str | None = None,
) -> dict[str, Any]:
    representative_limit = int(policy["global_thresholds"]["representative_sessions"])
    ordered = sorted(summaries, key=lambda item: (item.get("updatedAt") or 0, item["id"]), reverse=True)
    session_ids = [item["id"] for item in ordered[:representative_limit]]
    value: dict[str, Any] = {
        "ruleId": rule_id,
        "title": title,
        "observed": observed,
        "impact": impact,
        "confidence": confidence,
        "affectedSessions": len(summaries),
        "sessionIds": session_ids,
        "evidenceIds": [f"session:{session_id}" for session_id in session_ids],
        "provenance": "inferred",
        "policyVersion": policy["policy_version"],
        "policySource": policy.get("_source_kind", "bundled"),
    }
    if next_action:
        value["nextAction"] = next_action
    return value


def _global_candidate(
    *,
    results: list[dict[str, Any]],
    eligible: list[dict[str, Any]],
    matched: list[dict[str, Any]],
    positive: bool,
    policy: dict[str, Any],
    rule_id: str,
    title: str,
    observed: str,
    impact: str,
    next_action: str | None = None,
) -> None:
    thresholds = policy["global_thresholds"]
    minimum = int(thresholds["minimum_occurrences"])
    ratio_target = float(thresholds["positive_ratio"] if positive else thresholds["improvement_ratio"])
    ratio = len(matched) / len(eligible) if eligible else 0
    if len(matched) < minimum or ratio < ratio_target:
        return
    results.append(
        _entry(
            rule_id=rule_id,
            title=title,
            observed=observed.format(matches=len(matched), eligible=len(eligible), ratio=round(ratio * 100)),
            impact=impact,
            confidence="high" if ratio >= 0.75 else "medium",
            summaries=matched,
            policy=policy,
            next_action=next_action,
        )
    )


def build_global_coaching(
    summaries: list[dict[str, Any]], policy: dict[str, Any]
) -> dict[str, Any]:
    usable = [item for item in summaries if item["taskSignature"] != "unavailable"]
    positive: list[dict[str, Any]] = []
    improvements: list[dict[str, Any]] = []
    model_eligible = [item for item in usable if model_role(item["model"])]
    model_matches = [
        item
        for item in model_eligible
        if model_role(item["model"]) == policy["model_roles"].get(item["taskSignature"])
    ]
    _global_candidate(
        results=positive,
        eligible=model_eligible,
        matched=model_matches,
        positive=True,
        policy=policy,
        rule_id="model-fit",
        title="Models matched the observed work",
        observed="{matches} of {eligible} classifiable GPT-5.6 sessions ({ratio}%) used the documented model fit.",
        impact="Matching capability to the observed workload balances depth, speed, and cost.",
    )

    delegation_eligible = usable
    delegation_matches = [item for item in delegation_eligible if _delegation_matches(item, policy)]
    _global_candidate(
        results=positive,
        eligible=delegation_eligible,
        matched=delegation_matches,
        positive=True,
        policy=policy,
        rule_id="delegation-right-sized",
        title="Delegation stayed proportional",
        observed="{matches} of {eligible} classifiable tasks ({ratio}%) used a proportional number of subagents.",
        impact="Right-sized delegation preserves parallelism without unnecessary coordination and token overhead.",
    )

    changed = [item for item in usable if item["fileChangesObserved"]]
    verified = [item for item in changed if item["verificationObserved"]]
    _global_candidate(
        results=positive,
        eligible=changed,
        matched=verified,
        positive=True,
        policy=policy,
        rule_id="changes-verified",
        title="Changes were followed by verification",
        observed="{matches} of {eligible} change sessions ({ratio}%) included an observed test, build, lint, or check.",
        impact="Verification after changes reduces regressions and makes completion claims easier to trust.",
    )

    efficient_model_eligible = [
        item for item in model_eligible if item["taskSignature"] in {"clear-repeatable", "read-heavy-exploration"}
    ]
    overpowered = [item for item in efficient_model_eligible if model_role(item["model"]) == "sol"]
    _global_candidate(
        results=improvements,
        eligible=efficient_model_eligible,
        matched=overpowered,
        positive=False,
        policy=policy,
        rule_id="sol-on-lighter-work",
        title="Sol was repeatedly used for lighter work",
        observed="{matches} of {eligible} lighter classifiable sessions ({ratio}%) used Sol.",
        impact="Sol adds depth that clear repeatable or read-heavy exploratory work may not require.",
        next_action="Try Luna for clear extraction or transformation and Terra for exploratory scans.",
    )

    complex_model_eligible = [item for item in model_eligible if item["taskSignature"] == "complex-open-ended"]
    underpowered = [item for item in complex_model_eligible if model_role(item["model"]) == "luna"]
    _global_candidate(
        results=improvements,
        eligible=complex_model_eligible,
        matched=underpowered,
        positive=False,
        policy=policy,
        rule_id="luna-on-complex-work",
        title="Luna was used for complex open-ended work",
        observed="{matches} of {eligible} complex classifiable sessions ({ratio}%) used Luna.",
        impact="Complex multi-step work benefits from the deeper judgment and follow-through of Sol.",
        next_action="Use Sol for the next similarly complex task, or narrow the task before choosing Luna.",
    )

    delegated = [item for item in usable if item["spawnedChildren"] > 0]
    excessive = [item for item in delegated if _delegation_excessive(item, policy)]
    _global_candidate(
        results=improvements,
        eligible=delegated,
        matched=excessive,
        positive=False,
        policy=policy,
        rule_id="delegation-excessive",
        title="Some delegation created avoidable fan-out",
        observed="{matches} of {eligible} delegated tasks ({ratio}%) showed excessive or poorly completed fan-out.",
        impact="Extra agents add coordination, latency, and token usage when the work is not independently parallel.",
        next_action="Keep simple work local and delegate only bounded sidecar tasks that can run independently.",
    )

    unverified = [item for item in changed if not item["verificationObserved"]]
    _global_candidate(
        results=improvements,
        eligible=changed,
        matched=unverified,
        positive=False,
        policy=policy,
        rule_id="changes-without-verification",
        title="Changes sometimes ended without verification",
        observed="{matches} of {eligible} change sessions ({ratio}%) lacked an observed verification command.",
        impact="Unverified changes leave correctness and completion less certain.",
        next_action="Ask Codex to run the relevant tests, build, linter, or targeted check before finishing.",
    )

    failures = [item for item in usable if item["failures"] >= 2]
    _global_candidate(
        results=improvements,
        eligible=usable,
        matched=failures,
        positive=False,
        policy=policy,
        rule_id="repeated-tool-failures",
        title="Repeated tool failures created churn",
        observed="{matches} of {eligible} classifiable tasks ({ratio}%) had at least two failed tool calls.",
        impact="Repeated failures consume context and delay useful work.",
        next_action="Include the exact failing command, environment, and expected result when retrying.",
    )

    maximum = int(policy["global_thresholds"]["maximum_items_per_card"])
    rank: Callable[[dict[str, Any]], tuple[int, int, str]] = lambda item: (
        1 if item["confidence"] == "high" else 0,
        item["affectedSessions"],
        item["ruleId"],
    )
    return {
        "positive": sorted(positive, key=rank, reverse=True)[:maximum],
        "improvements": sorted(improvements, key=rank, reverse=True)[:maximum],
        "policyVersion": policy["policy_version"],
        "policySource": policy.get("_source_kind", "bundled"),
    }


def build_session_coaching(summary: dict[str, Any], policy: dict[str, Any]) -> dict[str, Any]:
    positive: list[dict[str, Any]] = []
    improvements: list[dict[str, Any]] = []
    signature = summary["taskSignature"]
    role = model_role(summary["model"])
    expected = policy["model_roles"].get(signature)
    if role and expected and role == expected:
        positive.append(_entry(
            rule_id="model-fit",
            title="The model matched the observed workload",
            observed=f"{summary['model']} matched the {signature} operational signature.",
            impact="The selected model balanced the documented capability profile with the observed work.",
            confidence="medium",
            summaries=[summary],
            policy=policy,
        ))
    elif role == "sol" and signature in {"clear-repeatable", "read-heavy-exploration"}:
        improvements.append(_entry(
            rule_id="sol-on-lighter-work",
            title="A lighter model may fit this workflow",
            observed=f"Sol handled a {signature} operational signature.",
            impact="The observed work did not require Sol's full complex-task profile.",
            confidence="medium",
            summaries=[summary],
            policy=policy,
            next_action="Try Luna for clear repeatable work or Terra for exploratory scans.",
        ))
    elif role == "luna" and signature == "complex-open-ended":
        improvements.append(_entry(
            rule_id="luna-on-complex-work",
            title="Use a deeper model for similar work",
            observed="Luna handled a complex open-ended operational signature.",
            impact="Complex multi-step tasks benefit from stronger planning and follow-through.",
            confidence="medium",
            summaries=[summary],
            policy=policy,
            next_action="Use Sol next time, or split the work into clearer repeatable subtasks.",
        ))

    if signature != "unavailable" and _delegation_matches(summary, policy):
        positive.append(_entry(
            rule_id="delegation-right-sized",
            title="Subagent use was proportional",
            observed=f"The {signature} task used {summary['spawnedChildren']} spawned subagent(s).",
            impact="The observed delegation level avoided unnecessary fan-out.",
            confidence="medium",
            summaries=[summary],
            policy=policy,
        ))
    if _delegation_excessive(summary, policy):
        improvements.append(_entry(
            rule_id="delegation-excessive",
            title="Reduce subagent fan-out",
            observed=f"The {signature} task used {summary['spawnedChildren']} spawned subagent(s).",
            impact="The observed fan-out added coordination without enough completed parallel work.",
            confidence="medium",
            summaries=[summary],
            policy=policy,
            next_action="Keep the critical path local and delegate only independent bounded sidecar work.",
        ))

    if summary["fileChangesObserved"] and summary["verificationObserved"]:
        positive.append(_entry(
            rule_id="changes-verified",
            title="Changes were verified",
            observed="File-changing activity was followed by an observed verification command.",
            impact="Verification provides stronger evidence that the change works as intended.",
            confidence="high",
            summaries=[summary],
            policy=policy,
        ))
    elif summary["fileChangesObserved"]:
        improvements.append(_entry(
            rule_id="changes-without-verification",
            title="Verify after making changes",
            observed="File-changing activity was not followed by a recognized verification command.",
            impact="The session ended with weaker evidence of correctness.",
            confidence="medium",
            summaries=[summary],
            policy=policy,
            next_action="Run the relevant tests, build, linter, or targeted check before completion.",
        ))
    if summary["failures"] >= 2:
        improvements.append(_entry(
            rule_id="repeated-tool-failures",
            title="Reduce repeated tool failures",
            observed=f"{summary['failures']} tool calls failed in this session.",
            impact="Repeated failures consume time and context before recovery.",
            confidence="high",
            summaries=[summary],
            policy=policy,
            next_action="Provide the exact command, environment, and error when asking Codex to recover.",
        ))
    maximum = int(policy["global_thresholds"]["maximum_items_per_card"])
    return {
        "positive": positive[:maximum],
        "improvements": improvements[:maximum],
        "policyVersion": policy["policy_version"],
        "policySource": policy.get("_source_kind", "bundled"),
    }
