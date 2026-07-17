from __future__ import annotations

import copy
import json
import os
from pathlib import Path
import re
import sqlite3
import subprocess
import sys
import tempfile
import time
import unittest
from urllib.request import urlopen

from codex_observability.data import _speed_for_service_tier, _summarize_session_speed
from codex_observability.render import _format_preview, render_session_html


PLUGIN_ROOT = Path(__file__).resolve().parents[1]
CLI = PLUGIN_ROOT / "scripts" / "codex_observability.py"


class CodexObservabilityCliTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        self.codex_home = self.root / "codex-home"
        self.source_root = self.root / "codex-source"
        self.output_dir = self.root / "reports"
        self.cache_dir = self.root / "cache"
        self.bin_dir = self.root / "bin"
        self.codex_home.mkdir()
        self.source_root.mkdir()
        self.bin_dir.mkdir()

        subprocess.run(["git", "init", "-q"], cwd=self.source_root, check=True)
        subprocess.run(
            ["git", "config", "user.email", "tests@example.com"],
            cwd=self.source_root,
            check=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Codex Observability Tests"],
            cwd=self.source_root,
            check=True,
        )
        source_file = self.source_root / "protocol.rs"
        source_file.write_text("pub enum EventMsg {}\n", encoding="utf-8")
        subprocess.run(["git", "add", "protocol.rs"], cwd=self.source_root, check=True)
        subprocess.run(["git", "commit", "-qm", "fixture"], cwd=self.source_root, check=True)
        self.commit = subprocess.run(
            ["git", "rev-parse", "HEAD"],
            cwd=self.source_root,
            check=True,
            capture_output=True,
            text=True,
        ).stdout.strip()

        codex = self.bin_dir / "codex"
        codex.write_text("#!/bin/sh\necho 'codex-cli 0.143.0'\n", encoding="utf-8")
        codex.chmod(0o755)

        self.contract = self.root / "contract.json"
        self.contract.write_text(
            json.dumps(
                {
                    "adapter_version": "0.2.0",
                    "codex_cli_version": "0.143.0",
                    "source_commit": self.commit,
                    "source_tag": "rust-v0.143.0",
                    "required_sources": ["protocol.rs"],
                    "required_thread_columns": [
                        "id",
                        "rollout_path",
                        "tokens_used",
                        "model",
                        "reasoning_effort",
                    ],
                    "required_tables": {
                        "thread_spawn_edges": ["parent_thread_id", "child_thread_id", "status"]
                    },
                }
            ),
            encoding="utf-8",
        )
        self._create_state_db()

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def _create_state_db(self) -> None:
        db = sqlite3.connect(self.codex_home / "state_1.sqlite")
        db.execute(
            """
            CREATE TABLE threads (
                id TEXT PRIMARY KEY,
                rollout_path TEXT NOT NULL,
                created_at INTEGER NOT NULL,
                updated_at INTEGER NOT NULL,
                source TEXT NOT NULL,
                model_provider TEXT NOT NULL,
                cwd TEXT NOT NULL,
                title TEXT NOT NULL,
                sandbox_policy TEXT NOT NULL,
                approval_mode TEXT NOT NULL,
                tokens_used INTEGER NOT NULL,
                archived INTEGER NOT NULL,
                model TEXT,
                reasoning_effort TEXT,
                cli_version TEXT
            )
            """
        )
        db.execute(
            """
            CREATE TABLE thread_spawn_edges (
                parent_thread_id TEXT NOT NULL,
                child_thread_id TEXT PRIMARY KEY,
                status TEXT NOT NULL
            )
            """
        )
        db.commit()
        db.close()

    def add_session(self, service_tiers: tuple[str | None, ...] = ("priority",)) -> str:
        session_id = "019f0000-0000-7000-8000-000000000001"
        sessions = self.codex_home / "sessions" / "2026" / "07" / "16"
        sessions.mkdir(parents=True)
        rollout = sessions / f"rollout-2026-07-16T12-00-00-{session_id}.jsonl"
        speed_records: list[dict[str, object]] = []
        for index, service_tier in enumerate(service_tiers, start=1):
            settings: dict[str, object] = {"model": "gpt-test"}
            if service_tier is not None:
                settings["service_tier"] = service_tier
            speed_records.append(
                {
                    "timestamp": f"2026-07-16T12:00:01.{index}00Z",
                    "type": "event_msg",
                    "payload": {
                        "type": "thread_settings_applied",
                        "thread_settings": settings,
                    },
                }
            )
        records = [
            {
                "timestamp": "2026-07-16T12:00:00Z",
                "type": "session_meta",
                "payload": {
                    "id": session_id,
                    "session_id": session_id,
                    "cli_version": "0.143.0",
                    "model_provider": "openai",
                    "cwd": "/Users/example/project",
                },
            },
            {
                "timestamp": "2026-07-16T12:00:01Z",
                "type": "turn_context",
                "payload": {
                    "turn_id": "turn-1",
                    "model": "gpt-test",
                    "effort": "high",
                    "context_window": 1000,
                },
            },
            *speed_records,
            {
                "timestamp": "2026-07-16T12:00:02Z",
                "type": "event_msg",
                "payload": {"type": "task_started", "turn_id": "turn-1"},
            },
            {
                "timestamp": "2026-07-16T12:00:03Z",
                "type": "event_msg",
                "payload": {
                    "type": "token_count",
                    "info": {
                        "total_token_usage": {
                            "input_tokens": 100,
                            "cached_input_tokens": 10,
                            "output_tokens": 10,
                            "reasoning_output_tokens": 2,
                            "total_tokens": 110,
                        }
                    },
                },
            },
            {
                "timestamp": "2026-07-16T12:00:04Z",
                "type": "response_item",
                "payload": {
                    "type": "custom_tool_call",
                    "name": "exec_command",
                    "call_id": "call-1",
                    "input": '{"cmd":"sed -n 1p /Users/example/.codex/skills/test-skill/SKILL.md; echo sk-test-secret"}',
                },
            },
            {
                "timestamp": "2026-07-16T12:00:05Z",
                "type": "response_item",
                "payload": {
                    "type": "custom_tool_call_output",
                    "call_id": "call-1",
                    "output": "finished with sk-test-secret",
                },
            },
            {
                "timestamp": "2026-07-16T12:00:06Z",
                "type": "event_msg",
                "payload": {"type": "future_event", "value": 1},
            },
            {
                "timestamp": "2026-07-16T12:00:06.100Z",
                "type": "compacted",
                "payload": {"message": "omitted"},
            },
            {
                "timestamp": "2026-07-16T12:00:06.101Z",
                "type": "event_msg",
                "payload": {"type": "context_compacted"},
            },
            {
                "timestamp": "2026-07-16T12:00:06.200Z",
                "type": "event_msg",
                "payload": {
                    "type": "collab_agent_spawn_end",
                    "call_id": "spawn-1",
                    "sender_thread_id": session_id,
                    "new_thread_id": "019f0000-0000-7000-8000-000000000002",
                    "new_agent_nickname": "reviewer",
                    "new_agent_role": "review",
                    "prompt": "must never be rendered",
                    "model": "gpt-test-mini",
                    "reasoning_effort": "medium",
                    "status": "completed"
                },
            },
            {
                "timestamp": "2026-07-16T12:00:07Z",
                "type": "event_msg",
                "payload": {
                    "type": "token_count",
                    "info": {
                        "total_token_usage": {
                            "input_tokens": 200,
                            "cached_input_tokens": 50,
                            "output_tokens": 20,
                            "reasoning_output_tokens": 5,
                            "total_tokens": 220,
                        }
                    },
                },
            },
            {
                "timestamp": "2026-07-16T12:00:08Z",
                "type": "event_msg",
                "payload": {"type": "task_complete", "turn_id": "turn-1"},
            },
        ]
        with rollout.open("w", encoding="utf-8") as handle:
            for record in records:
                handle.write(json.dumps(record) + "\n")
            handle.write("{malformed\n")

        db = sqlite3.connect(self.codex_home / "state_1.sqlite")
        db.execute(
            """
            INSERT INTO threads (
              id, rollout_path, created_at, updated_at, source, model_provider,
              cwd, title, sandbox_policy, approval_mode, tokens_used, archived,
              model, reasoning_effort, cli_version
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (
                session_id,
                str(rollout),
                1784203200,
                1784203208,
                "cli",
                "openai",
                "/Users/example/project",
                "Sensitive title is not rendered",
                "workspace-write",
                "on-request",
                220,
                0,
                "gpt-test",
                "high",
                "0.143.0",
            ),
        )
        db.commit()
        db.close()
        return session_id

    def run_cli(self, *args: str) -> subprocess.CompletedProcess[str]:
        env = os.environ.copy()
        env["PATH"] = f"{self.bin_dir}{os.pathsep}{env.get('PATH', '')}"
        return subprocess.run(
            [
                sys.executable,
                str(CLI),
                *args,
                "--source-root",
                str(self.source_root),
                "--codex-home",
                str(self.codex_home),
                "--output-dir",
                str(self.output_dir),
                "--cache-dir",
                str(self.cache_dir),
                "--contract",
                str(self.contract),
            ],
            cwd=PLUGIN_ROOT,
            env=env,
            capture_output=True,
            text=True,
        )

    def test_doctor_verifies_source_runtime_and_state_schema(self) -> None:
        result = self.run_cli("doctor", "--format", "json")

        self.assertEqual(result.returncode, 0, result.stderr)
        payload = json.loads(result.stdout)
        self.assertEqual(payload["status"], "ok")
        self.assertEqual(payload["codexCliVersion"], "0.143.0")
        self.assertEqual(payload["sourceCommit"], self.commit)
        self.assertTrue(payload["stateDatabase"].endswith("state_1.sqlite"))

    def test_report_uses_final_cumulative_tokens_and_redacts_tool_details(self) -> None:
        session_id = self.add_session()

        result = self.run_cli("report", "--session", "latest", "--format", "json")

        self.assertEqual(result.returncode, 0, result.stderr)
        payload = json.loads(result.stdout)
        self.assertEqual(payload["session"]["id"], session_id)
        self.assertEqual(payload["tokenEfficiency"]["totalTokens"], 220)
        self.assertEqual(payload["tokenEfficiency"]["cachedInputTokens"], 50)
        self.assertEqual(payload["tokenEfficiency"]["cacheRatio"], 25.0)
        self.assertEqual(payload["toolActivity"]["totalCalls"], 1)
        self.assertEqual(payload["toolActivity"]["calls"][0]["status"], "success")
        serialized = json.dumps(payload)
        self.assertNotIn("sk-test-secret", serialized)
        self.assertRegex(serialized, re.compile(r"REDACTED"))
        self.assertEqual(payload["reliability"]["malformedLines"], 1)
        self.assertIn("future_event", payload["reliability"]["unknownEventTypes"])
        self.assertEqual(payload["contextAndReasoning"]["compactions"], 1)
        self.assertEqual(payload["skillsAndAgents"]["skills"][0]["name"], "test-skill")
        self.assertEqual(payload["skillsAndAgents"]["skills"][0]["state"], "instructions-loaded-inferred")
        self.assertEqual(len(payload["skillsAndAgents"]["subagents"]), 1)
        self.assertEqual(payload["sessionSpeed"]["classification"], "Fast")
        self.assertEqual(payload["sessionSpeed"]["observedTiers"], ["priority"])
        self.assertEqual(payload["sessionSpeed"]["changes"], 0)
        self.assertNotIn("must never be rendered", json.dumps(payload))

    def test_session_speed_classifies_standard_and_mixed(self) -> None:
        self.add_session(service_tiers=(None,))
        standard = self.run_cli("report", "--session", "latest", "--format", "json")

        self.assertEqual(standard.returncode, 0, standard.stderr)
        standard_speed = json.loads(standard.stdout)["sessionSpeed"]
        self.assertEqual(standard_speed["classification"], "Standard")
        self.assertEqual(standard_speed["observedTiers"], ["default"])

        rollout = next((self.codex_home / "sessions").rglob("*.jsonl"))
        with rollout.open("a", encoding="utf-8") as handle:
            handle.write(
                json.dumps(
                    {
                        "timestamp": "2026-07-16T12:00:09Z",
                        "type": "event_msg",
                        "payload": {
                            "type": "thread_settings_applied",
                            "thread_settings": {"model": "gpt-test", "service_tier": "priority"},
                        },
                    }
                )
                + "\n"
            )
        mixed = self.run_cli("report", "--session", "latest", "--format", "json")

        self.assertEqual(mixed.returncode, 0, mixed.stderr)
        mixed_speed = json.loads(mixed.stdout)["sessionSpeed"]
        self.assertEqual(mixed_speed["classification"], "Mixed")
        self.assertEqual(mixed_speed["changes"], 1)
        self.assertEqual(mixed_speed["observedTiers"], ["default", "priority"])

    def test_session_speed_does_not_guess_unsupported_or_missing_tiers(self) -> None:
        self.assertEqual(_speed_for_service_tier("fast"), "Fast")
        self.assertEqual(_speed_for_service_tier("priority"), "Fast")
        self.assertEqual(_speed_for_service_tier("default"), "Standard")
        self.assertEqual(_speed_for_service_tier(None), "Standard")
        self.assertIsNone(_speed_for_service_tier("flex"))
        self.assertEqual(_summarize_session_speed([])["classification"], "Unavailable")
        self.assertEqual(
            _summarize_session_speed(
                [{"speed": None, "serviceTier": "flex", "evidence": "fixture:1"}]
            )["classification"],
            "Unavailable",
        )

    def test_state_total_wins_and_token_mismatch_is_visible(self) -> None:
        self.add_session()
        db = sqlite3.connect(self.codex_home / "state_1.sqlite")
        db.execute("UPDATE threads SET tokens_used = 221")
        db.commit()
        db.close()

        result = self.run_cli("report", "--session", "latest", "--format", "json")

        self.assertEqual(result.returncode, 0, result.stderr)
        payload = json.loads(result.stdout)
        self.assertEqual(payload["tokenEfficiency"]["totalTokens"], 221)
        self.assertEqual(payload["tokenEfficiency"]["rolloutTotalTokens"], 220)
        self.assertEqual(payload["tokenEfficiency"]["reconciliation"], "mismatch")
        self.assertTrue(payload["reliability"]["tokenMismatch"])

    def test_session_status_uses_latest_turn_lifecycle(self) -> None:
        self.add_session()
        rollout = next((self.codex_home / "sessions").rglob("*.jsonl"))
        with rollout.open("a", encoding="utf-8") as handle:
            handle.write(
                json.dumps(
                    {
                        "timestamp": "2026-07-16T12:00:09Z",
                        "type": "event_msg",
                        "payload": {"type": "task_started", "turn_id": "turn-2"},
                    }
                )
                + "\n"
            )

        result = self.run_cli("report", "--session", "latest", "--format", "json")

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["session"]["status"], "in-progress-or-incomplete")

    def test_metadata_mode_omits_tool_argument_and_output_previews(self) -> None:
        self.add_session()

        result = self.run_cli(
            "report", "--session", "latest", "--format", "json", "--content", "metadata"
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        call = json.loads(result.stdout)["toolActivity"]["calls"][0]
        self.assertNotIn("inputPreview", call)
        self.assertNotIn("outputPreview", call)

    def test_structured_tool_failure_is_classified_without_text_guessing(self) -> None:
        self.add_session()
        rollout = next((self.codex_home / "sessions").rglob("*.jsonl"))
        with rollout.open("a", encoding="utf-8") as handle:
            handle.write(json.dumps({"timestamp": "2026-07-16T12:00:10Z", "type": "response_item", "payload": {"type": "custom_tool_call", "name": "exec_command", "call_id": "call-2", "input": "{}"}}) + "\n")
            handle.write(json.dumps({"timestamp": "2026-07-16T12:00:11Z", "type": "response_item", "payload": {"type": "custom_tool_call_output", "call_id": "call-2", "output": {"exit_code": 2, "output": "ordinary text"}}}) + "\n")

        result = self.run_cli("report", "--session", "latest", "--format", "json")

        self.assertEqual(result.returncode, 0, result.stderr)
        calls = {call["id"]: call for call in json.loads(result.stdout)["toolActivity"]["calls"]}
        self.assertEqual(calls["call-2"]["status"], "failure")

    def test_doctor_fails_closed_for_source_commit_mismatch(self) -> None:
        contract = json.loads(self.contract.read_text(encoding="utf-8"))
        contract["source_commit"] = "0" * 40
        self.contract.write_text(json.dumps(contract), encoding="utf-8")

        result = self.run_cli("doctor", "--format", "json")

        self.assertEqual(result.returncode, 2)
        payload = json.loads(result.stdout)
        self.assertEqual(payload["status"], "error")
        self.assertIn("unsupported /codex commit", payload["error"])

    def test_html_report_is_self_contained_grouped_and_safe(self) -> None:
        session_id = self.add_session()

        result = self.run_cli("report", "--session", session_id, "--format", "html")

        self.assertEqual(result.returncode, 0, result.stderr)
        report_path = Path(result.stdout.strip())
        self.assertTrue(report_path.is_file())
        contents = report_path.read_text(encoding="utf-8")
        for section_id in (
            "session-overview",
            "token-efficiency",
            "tool-activity",
            "context-reasoning",
            "skills-agents",
            "files-verification",
            "reliability",
            "timeline",
            "provenance",
        ):
            self.assertIn(f'id="{section_id}"', contents)
        self.assertNotIn("sk-test-secret", contents)
        self.assertNotIn("Sensitive title", contents)
        self.assertNotRegex(contents, re.compile(r'<(?:script|link)[^>]+src=["\']https?://'))
        self.assertIn("Content-Security-Policy", contents)
        self.assertNotIn("Local · source-verified · session report", contents)
        self.assertIn("8 seconds", contents)
        self.assertIn("Fast", contents)
        self.assertIn("priority service tier observed", contents)
        self.assertNotIn("What you did well", contents)
        self.assertNotIn("What to improve", contents)
        self.assertNotIn('id="coaching"', contents)
        self.assertNotIn('id="recommendations"', contents)
        for icon in ("sessions", "coverage", "tokens", "tools", "agents", "verification", "context", "timeline", "provenance"):
            self.assertIn(f'data-icon="{icon}"', contents)
        self.assertGreaterEqual(contents.count('class="category-icon"'), 9)
        self.assertIn("<svg", contents)
        self.assertIn('id="advanced-explorer"', contents)
        self.assertIn('role="tablist"', contents)
        self.assertIn('id="explorer-tab-tools"', contents)
        self.assertIn('id="explorer-tab-timeline"', contents)
        self.assertIn('height:clamp(420px,60vh,680px)', contents)
        self.assertIn('width:clamp(440px,44vw,720px)', contents)
        self.assertIn('height:min(88vh,760px)', contents)
        self.assertIn('data-page-action="first"', contents)
        self.assertIn('data-page-action="last"', contents)
        self.assertIn('role="dialog"', contents)
        self.assertIn('aria-modal="true"', contents)
        self.assertIn('const PAGE_SIZE = 25', contents)
        self.assertIn('window.__codexExplorer', contents)
        self.assertNotIn('<tr class="tool-row"', contents)
        self.assertNotIn('<li class="timeline-item', contents)
        self.assertNotIn("innerHTML", contents)

    def test_compact_explorer_embeds_large_datasets_once_without_prerendering_rows(self) -> None:
        self.add_session()
        result = self.run_cli("report", "--session", "latest", "--format", "json")
        self.assertEqual(result.returncode, 0, result.stderr)
        base = json.loads(result.stdout)

        for size in (0, 1, 25, 26, 1000):
            report = copy.deepcopy(base)
            calls = []
            timeline = []
            for index in range(size):
                calls.append(
                    {
                        "id": f"call-{index}",
                        "name": "exec_command",
                        "category": "Shell",
                        "status": "success",
                        "startedAt": f"2026-07-16T12:{index % 60:02d}:00Z",
                        "completedAt": f"2026-07-16T12:{index % 60:02d}:01Z",
                        "durationMs": 1000,
                        "evidence": [f"fixture:tool:{index}"],
                        "inputPreview": f"unique-preview-{size}-{index}",
                        "outputPreview": "ok",
                    }
                )
                timeline.append(
                    {
                        "timestamp": f"2026-07-16T12:{index % 60:02d}:02Z",
                        "type": "fixture_event",
                        "evidence": f"fixture:timeline:{index}",
                    }
                )
            report["toolActivity"].update(
                {
                    "totalCalls": size,
                    "successes": size,
                    "failures": 0,
                    "incomplete": 0,
                    "byCategory": {"Shell": size} if size else {},
                    "byTool": {"exec_command": size} if size else {},
                    "calls": calls,
                }
            )
            report["timeline"] = timeline
            output = render_session_html(report, self.output_dir / f"size-{size}")
            contents = output.read_text(encoding="utf-8")

            self.assertEqual(contents.count('id="report-data"'), 1)
            self.assertEqual(contents.count('<tbody id="tool-rows"></tbody>'), 1)
            self.assertEqual(contents.count('<tbody id="timeline-rows"></tbody>'), 1)
            self.assertNotIn('<tr class="tool-row"', contents)
            self.assertNotIn('<li class="timeline-item', contents)
            self.assertIn('const PAGE_SIZE = 25', contents)
            if size:
                self.assertEqual(contents.count(f"unique-preview-{size}-{size - 1}"), 1)

    def test_redacted_detail_formatter_pretty_prints_structured_content(self) -> None:
        json_format, pretty_json = _format_preview('{"z":1,"nested":{"value":2}}')
        block_format, content_blocks = _format_preview(
            '[{"type":"input_text","text":"first line\\nsecond line"}]'
        )
        code_format, pretty_code = _format_preview(
            'const result=await tools.exec_command({cmd:"pwd",workdir:"/tmp"});text(result);'
        )

        self.assertEqual(json_format, "JSON")
        self.assertIn('\n  "nested": {', pretty_json)
        self.assertEqual(block_format, "Content blocks")
        self.assertIn("first line\nsecond line", content_blocks)
        self.assertNotIn("first line\\nsecond line", content_blocks)
        self.assertEqual(code_format, "JavaScript")
        self.assertIn('\n  cmd:"pwd",', pretty_code)
        self.assertIn("\ntext(result);", pretty_code)

    def test_index_lists_sessions_without_titles_or_prompt_content(self) -> None:
        session_id = self.add_session()

        result = self.run_cli("index", "--limit", "50", "--format", "html")

        self.assertEqual(result.returncode, 0, result.stderr)
        index_path = Path(result.stdout.strip())
        contents = index_path.read_text(encoding="utf-8")
        self.assertIn(session_id, contents)
        self.assertIn("gpt-test", contents)
        self.assertIn("220", contents)
        self.assertIn("Local · all-history · source-verified", contents)
        self.assertNotIn("Sensitive title", contents)
        self.assertNotRegex(contents, re.compile(r'<(?:script|link)[^>]+src=["\']https?://'))
        self.assertNotIn("What you did well", contents)
        self.assertNotIn("What to improve", contents)
        self.assertNotIn('id="coaching"', contents)
        for icon in ("sessions", "coverage", "tokens", "tools", "delegation", "models", "reasoning", "recent", "provenance"):
            self.assertIn(f'data-icon="{icon}"', contents)
        self.assertGreaterEqual(contents.count('class="category-icon"'), 9)

    def test_coaching_is_opt_in_and_written_separately(self) -> None:
        session_id = self.add_session()

        default_report = self.run_cli("report", "--session", session_id, "--format", "json")
        coached_report = self.run_cli(
            "report", "--session", session_id, "--coaching", "--format", "html"
        )
        default_index = self.run_cli("index", "--format", "json")
        coached_index = self.run_cli("index", "--coaching", "--format", "html")

        self.assertEqual(default_report.returncode, 0, default_report.stderr)
        default_report_payload = json.loads(default_report.stdout)
        self.assertFalse(default_report_payload["coaching"]["enabled"])
        self.assertEqual(default_report_payload["coaching"]["positive"], [])
        self.assertEqual(default_report_payload["coaching"]["improvements"], [])
        self.assertEqual(default_report_payload["recommendations"], [])

        self.assertEqual(coached_report.returncode, 0, coached_report.stderr)
        coached_report_path = Path(coached_report.stdout.strip())
        self.assertEqual(coached_report_path.name, f"{session_id}-coaching.html")
        coached_report_html = coached_report_path.read_text(encoding="utf-8")
        self.assertIn('id="coaching"', coached_report_html)
        self.assertIn("What you did well", coached_report_html)
        self.assertIn("What to improve", coached_report_html)

        self.assertEqual(default_index.returncode, 0, default_index.stderr)
        default_index_payload = json.loads(default_index.stdout)
        self.assertFalse(default_index_payload["coaching"]["enabled"])
        self.assertEqual(default_index_payload["coaching"]["positive"], [])
        self.assertEqual(default_index_payload["coaching"]["improvements"], [])

        self.assertEqual(coached_index.returncode, 0, coached_index.stderr)
        coached_index_path = Path(coached_index.stdout.strip())
        self.assertEqual(coached_index_path.name, "coaching.html")
        coached_index_html = coached_index_path.read_text(encoding="utf-8")
        self.assertIn('id="coaching"', coached_index_html)
        self.assertIn("What you did well", coached_index_html)
        self.assertIn("What to improve", coached_index_html)
        self.assertFalse((self.output_dir / "index.html").is_file())

    def test_history_cache_is_incremental_and_rebuildable(self) -> None:
        self.add_session()

        first = self.run_cli("index", "--format", "json")
        second = self.run_cli("index", "--format", "json")
        rollout = next((self.codex_home / "sessions").rglob("*.jsonl"))
        with rollout.open("a", encoding="utf-8") as handle:
            handle.write(json.dumps({"timestamp": "2026-07-16T12:00:10Z", "type": "event_msg", "payload": {"type": "warning"}}) + "\n")
        changed = self.run_cli("index", "--format", "json")
        rebuilt = self.run_cli("index", "--format", "json", "--rebuild-cache")

        self.assertEqual(first.returncode, 0, first.stderr)
        self.assertEqual(second.returncode, 0, second.stderr)
        first_payload = json.loads(first.stdout)
        second_payload = json.loads(second.stdout)
        changed_payload = json.loads(changed.stdout)
        rebuilt_payload = json.loads(rebuilt.stdout)
        self.assertEqual(first_payload["cache"]["misses"], 1)
        self.assertEqual(second_payload["cache"]["hits"], 1)
        self.assertEqual(second_payload["cache"]["misses"], 0)
        self.assertEqual(changed_payload["cache"]["misses"], 1)
        self.assertEqual(rebuilt_payload["cache"]["misses"], 1)
        self.assertEqual(first_payload["population"], second_payload["population"])
        self.assertEqual(first_payload["aggregates"], second_payload["aggregates"])
        self.assertEqual(first_payload["coaching"], second_payload["coaching"])
        self.assertEqual(
            first_payload["aggregates"]["tokens"]["allThreads"],
            first_payload["cache"]["stateTotalTokensAtRefresh"],
        )

        db = sqlite3.connect(self.codex_home / "state_1.sqlite")
        db.execute("DELETE FROM threads")
        db.commit()
        db.close()
        purged = self.run_cli("index", "--format", "json")
        purged_payload = json.loads(purged.stdout)
        self.assertEqual(purged_payload["cache"]["purged"], 1)
        self.assertEqual(purged_payload["population"]["total"], 0)

    def test_dashboard_separates_thread_populations_and_never_caches_content(self) -> None:
        session_ids = [
            "019f0000-0000-7000-8000-000000000010",
            "019f0000-0000-7000-8000-000000000011",
            "019f0000-0000-7000-8000-000000000012",
        ]
        sources = [
            "cli",
            json.dumps({"subagent": {"thread_spawn": {"parent_thread_id": session_ids[0], "depth": 1}}}),
            json.dumps({"subagent": {"other": "guardian"}}),
        ]
        sessions = self.codex_home / "sessions"
        sessions.mkdir()
        db = sqlite3.connect(self.codex_home / "state_1.sqlite")
        for index, (session_id, source) in enumerate(zip(session_ids, sources)):
            rollout = sessions / f"rollout-2026-07-16T12-00-0{index}-{session_id}.jsonl"
            records = [
                {"timestamp": f"2026-07-16T12:00:0{index}Z", "type": "session_meta", "payload": {"id": session_id}},
                {"timestamp": f"2026-07-16T12:00:0{index}Z", "type": "turn_context", "payload": {"turn_id": f"turn-{index}", "model": "gpt-5.6-luna", "effort": "low"}},
                {"timestamp": f"2026-07-16T12:00:0{index}Z", "type": "event_msg", "payload": {"type": "task_started", "turn_id": f"turn-{index}"}},
                {"timestamp": f"2026-07-16T12:00:0{index}Z", "type": "event_msg", "payload": {"type": "user_message", "message": "PROMPT_SENTINEL_NEVER_CACHE"}},
                {"timestamp": f"2026-07-16T12:00:0{index}Z", "type": "event_msg", "payload": {"type": "task_complete", "turn_id": f"turn-{index}"}},
            ]
            rollout.write_text("".join(json.dumps(record) + "\n" for record in records), encoding="utf-8")
            db.execute(
                """INSERT INTO threads(id,rollout_path,created_at,updated_at,source,model_provider,cwd,title,sandbox_policy,approval_mode,tokens_used,archived,model,reasoning_effort,cli_version) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)""",
                (session_id, str(rollout), 100 + index, 100 + index, source, "openai", "/Users/example/project", "TITLE_SENTINEL_NEVER_CACHE", "read-only", "never", 0, 0, "gpt-5.6-luna", "low", "0.143.0"),
            )
        db.execute(
            "INSERT INTO thread_spawn_edges(parent_thread_id,child_thread_id,status) VALUES(?,?,?)",
            (session_ids[0], session_ids[1], "closed"),
        )
        db.commit()
        db.close()

        result = self.run_cli("index", "--format", "json")

        self.assertEqual(result.returncode, 0, result.stderr)
        payload = json.loads(result.stdout)
        self.assertEqual(payload["population"]["total"], 3)
        self.assertEqual(payload["population"]["topLevel"], 1)
        self.assertEqual(payload["population"]["spawned"], 1)
        self.assertEqual(payload["population"]["internal"], 1)
        self.assertEqual(payload["population"]["analyzed"], 3)
        serialized = json.dumps(payload)
        cache_bytes = (self.cache_dir / "history-0.2.sqlite").read_bytes()
        self.assertNotIn("PROMPT_SENTINEL_NEVER_CACHE", serialized)
        self.assertNotIn("TITLE_SENTINEL_NEVER_CACHE", serialized)
        self.assertNotIn(b"PROMPT_SENTINEL_NEVER_CACHE", cache_bytes)
        self.assertNotIn(b"TITLE_SENTINEL_NEVER_CACHE", cache_bytes)

    def test_clean_can_remove_only_the_generated_history_cache(self) -> None:
        self.add_session()
        self.run_cli("index", "--format", "json")
        unrelated = self.cache_dir / "keep.txt"
        unrelated.write_text("keep", encoding="utf-8")

        result = self.run_cli("clean", "--cache", "--format", "json")

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse((self.cache_dir / "history-0.2.sqlite").exists())
        self.assertTrue(unrelated.exists())

    def test_clean_removes_only_old_generated_html(self) -> None:
        old = self.output_dir / "sessions" / "old.html"
        new = self.output_dir / "sessions" / "new.html"
        unrelated = self.output_dir / "keep.txt"
        old.parent.mkdir(parents=True)
        old.write_text("old", encoding="utf-8")
        new.write_text("new", encoding="utf-8")
        unrelated.write_text("keep", encoding="utf-8")
        old_time = time.time() - 10 * 86400
        os.utime(old, (old_time, old_time))

        result = self.run_cli("clean", "--older-than", "5", "--format", "json")

        self.assertEqual(result.returncode, 0, result.stderr)
        payload = json.loads(result.stdout)
        self.assertEqual(payload["removed"], 1)
        self.assertFalse(old.exists())
        self.assertTrue(new.exists())
        self.assertTrue(unrelated.exists())

    def test_serve_binds_to_loopback_and_serves_report_directory(self) -> None:
        self.output_dir.mkdir(parents=True)
        (self.output_dir / "index.html").write_text("observability-index", encoding="utf-8")
        env = os.environ.copy()
        env["PATH"] = f"{self.bin_dir}{os.pathsep}{env.get('PATH', '')}"
        process = subprocess.Popen(
            [
                sys.executable,
                str(CLI),
                "serve",
                "--port",
                "0",
                "--source-root",
                str(self.source_root),
                "--codex-home",
                str(self.codex_home),
                "--output-dir",
                str(self.output_dir),
                "--cache-dir",
                str(self.cache_dir),
                "--contract",
                str(self.contract),
            ],
            cwd=PLUGIN_ROOT,
            env=env,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        try:
            assert process.stdout is not None
            url = process.stdout.readline().strip()
            error = ""
            if not url and process.stderr is not None:
                error = process.stderr.read()
            if "Operation not permitted" in error:
                self.skipTest("sandbox does not permit loopback binds")
            self.assertRegex(url, re.compile(r"^http://127\.0\.0\.1:\d+/$"), error)
            with urlopen(url, timeout=3) as response:
                body = response.read().decode("utf-8")
            self.assertIn("observability-index", body)
        finally:
            if process.poll() is None:
                process.terminate()
            process.wait(timeout=3)
            if process.stdout is not None:
                process.stdout.close()
            if process.stderr is not None:
                process.stderr.close()


if __name__ == "__main__":
    unittest.main()
