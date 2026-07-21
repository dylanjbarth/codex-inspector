from __future__ import annotations

from copy import deepcopy
from pathlib import Path
import unittest

from codex_observability.coaching import (
    build_global_coaching,
    build_session_coaching,
    classify_task,
    load_policy,
)


POLICY_PATH = Path(__file__).resolve().parents[1] / "contracts" / "coaching-policy-0.2.0.json"


def summary(session_id: str, **overrides: object) -> dict[str, object]:
    value: dict[str, object] = {
        "id": session_id,
        "updatedAt": 100,
        "status": "complete",
        "model": "gpt-5.6-luna",
        "reasoningEffort": "low",
        "turnCount": 2,
        "toolCalls": 4,
        "toolCategoryCount": 1,
        "toolCategories": {"Shell": 4},
        "failures": 0,
        "compactions": 0,
        "modelChanged": False,
        "effortChanged": False,
        "fileChangesObserved": False,
        "verificationObserved": False,
        "readCalls": 2,
        "readCategoryCount": 1,
        "coachingEligible": True,
        "spawnedChildren": 0,
        "completedChildren": 0,
        "childCompletionRatio": 1.0,
        "childrenOverlapped": False,
    }
    value.update(overrides)
    return value


class CoachingTests(unittest.TestCase):
    def setUp(self) -> None:
        self.policy = load_policy(POLICY_PATH, source_kind="bundled")

    def classify(self, value: dict[str, object]) -> str:
        classification = classify_task(value, self.policy)
        value["taskSignature"] = classification
        return classification

    def test_prompt_free_task_signatures_cover_all_policy_buckets(self) -> None:
        clear = summary("clear")
        read_heavy = summary(
            "read", toolCalls=10, toolCategoryCount=2, readCalls=9, readCategoryCount=2
        )
        everyday = summary("everyday", toolCalls=10, toolCategoryCount=2, readCalls=2)
        complex_work = summary("complex", toolCalls=20, toolCategoryCount=4)
        unavailable = summary("unavailable", coachingEligible=False)

        self.assertEqual(self.classify(clear), "clear-repeatable")
        self.assertEqual(self.classify(read_heavy), "read-heavy-exploration")
        self.assertEqual(self.classify(everyday), "everyday")
        self.assertEqual(self.classify(complex_work), "complex-open-ended")
        self.assertEqual(self.classify(unavailable), "unavailable")

    def test_global_positive_cards_require_three_sessions_and_sixty_percent(self) -> None:
        sessions = [summary(f"session-{index}") for index in range(3)]
        for item in sessions:
            self.classify(item)
        coaching = build_global_coaching(sessions, self.policy)
        rules = {item["ruleId"] for item in coaching["positive"]}

        self.assertIn("model-fit", rules)
        self.assertIn("delegation-right-sized", rules)
        self.assertTrue(all(item["affectedSessions"] == 3 for item in coaching["positive"]))
        self.assertTrue(all(item["evidenceIds"] for item in coaching["positive"]))

        coaching = build_global_coaching(sessions[:2], self.policy)
        self.assertEqual(coaching["positive"], [])

    def test_global_improvements_flag_sol_fanout_and_cap_at_three(self) -> None:
        sessions = [
            summary(
                f"session-{index}",
                model="gpt-5.6-sol",
                spawnedChildren=2,
                completedChildren=0,
                childCompletionRatio=0.0,
                fileChangesObserved=True,
                verificationObserved=False,
                failures=0,
            )
            for index in range(3)
        ]
        for item in sessions:
            self.classify(item)
        coaching = build_global_coaching(sessions, self.policy)
        rules = {item["ruleId"] for item in coaching["improvements"]}

        self.assertLessEqual(len(coaching["improvements"]), 3)
        self.assertIn("sol-on-lighter-work", rules)
        self.assertIn("delegation-excessive", rules)
        self.assertTrue(all(item.get("nextAction") for item in coaching["improvements"]))

    def test_session_cards_match_sol_luna_terra_and_unknown_models(self) -> None:
        clear = summary("clear")
        self.classify(clear)
        coaching = build_session_coaching(clear, self.policy)
        self.assertIn("model-fit", {item["ruleId"] for item in coaching["positive"]})

        complex_luna = summary("complex", toolCalls=20, toolCategoryCount=4)
        self.classify(complex_luna)
        coaching = build_session_coaching(complex_luna, self.policy)
        self.assertIn("luna-on-complex-work", {item["ruleId"] for item in coaching["improvements"]})

        read_terra = summary(
            "read",
            model="gpt-5.6-terra",
            toolCalls=10,
            toolCategoryCount=2,
            readCalls=9,
            readCategoryCount=2,
        )
        self.classify(read_terra)
        coaching = build_session_coaching(read_terra, self.policy)
        self.assertIn("model-fit", {item["ruleId"] for item in coaching["positive"]})

        unknown = deepcopy(clear)
        unknown["model"] = "gpt-unknown"
        coaching = build_session_coaching(unknown, self.policy)
        self.assertNotIn("model-fit", {item["ruleId"] for item in coaching["positive"]})


if __name__ == "__main__":
    unittest.main()
