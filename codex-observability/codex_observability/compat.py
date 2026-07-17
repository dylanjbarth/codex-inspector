from __future__ import annotations

from dataclasses import dataclass
import json
from pathlib import Path
import re
import sqlite3
import subprocess
from typing import Any


class CompatibilityError(RuntimeError):
    """Raised when runtime data cannot be interpreted by the selected contract."""


@dataclass(frozen=True)
class CompatibilityResult:
    codex_cli_version: str
    source_commit: str
    state_database: Path
    contract: dict[str, Any]

    def as_dict(self) -> dict[str, Any]:
        return {
            "status": "ok",
            "codexCliVersion": self.codex_cli_version,
            "sourceCommit": self.source_commit,
            "sourceTag": self.contract.get("source_tag"),
            "adapterVersion": self.contract.get("adapter_version"),
            "stateDatabase": str(self.state_database),
        }


def load_contract(path: Path) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise CompatibilityError(f"unable to load adapter contract {path}: {exc}") from exc
    if not isinstance(value, dict):
        raise CompatibilityError(f"adapter contract must be a JSON object: {path}")
    return value


def newest_state_database(codex_home: Path) -> Path:
    candidates = [
        path
        for path in codex_home.glob("state_*.sqlite")
        if not path.name.endswith(("-shm", "-wal"))
    ]
    if not candidates:
        raise CompatibilityError(f"no state_*.sqlite database found under {codex_home}")

    def key(path: Path) -> tuple[int, int]:
        match = re.fullmatch(r"state_(\d+)\.sqlite", path.name)
        version = int(match.group(1)) if match else -1
        return version, path.stat().st_mtime_ns

    return max(candidates, key=key)


def _run_text(command: list[str], cwd: Path | None = None) -> str:
    try:
        completed = subprocess.run(
            command,
            cwd=cwd,
            check=True,
            capture_output=True,
            text=True,
        )
    except (OSError, subprocess.CalledProcessError) as exc:
        detail = getattr(exc, "stderr", None) or str(exc)
        raise CompatibilityError(f"command failed: {' '.join(command)}: {detail.strip()}") from exc
    return completed.stdout.strip()


def verify_compatibility(
    *, source_root: Path, codex_home: Path, contract_path: Path
) -> CompatibilityResult:
    contract = load_contract(contract_path)
    if not source_root.is_dir() or not (source_root / ".git").exists():
        raise CompatibilityError(f"source root is not a Git checkout: {source_root}")

    source_commit = _run_text(["git", "rev-parse", "HEAD"], cwd=source_root)
    expected_commit = str(contract.get("source_commit", ""))
    if source_commit != expected_commit:
        raise CompatibilityError(
            f"unsupported /codex commit: expected {expected_commit}, found {source_commit}"
        )

    for relative in contract.get("required_sources", []):
        if not (source_root / relative).is_file():
            raise CompatibilityError(f"required source file is missing: {relative}")

    version_output = _run_text(["codex", "--version"])
    match = re.search(r"codex-cli\s+([^\s]+)", version_output)
    if not match:
        raise CompatibilityError(f"unable to parse Codex CLI version: {version_output}")
    cli_version = match.group(1)
    expected_version = str(contract.get("codex_cli_version", ""))
    if cli_version != expected_version:
        raise CompatibilityError(
            f"unsupported Codex CLI version: expected {expected_version}, found {cli_version}"
        )

    state_db = newest_state_database(codex_home)
    uri = f"file:{state_db.as_posix()}?mode=ro"
    try:
        connection = sqlite3.connect(uri, uri=True)
        connection.execute("PRAGMA query_only = ON")
        columns = {
            row[1] for row in connection.execute("PRAGMA table_info(threads)").fetchall()
        }
        table_columns = {
            table: {
                row[1]
                for row in connection.execute(f"PRAGMA table_info({table})").fetchall()
            }
            for table in contract.get("required_tables", {})
        }
    except sqlite3.Error as exc:
        raise CompatibilityError(f"unable to inspect {state_db}: {exc}") from exc
    finally:
        if "connection" in locals():
            connection.close()

    missing = sorted(set(contract.get("required_thread_columns", [])) - columns)
    if missing:
        raise CompatibilityError(f"state database lacks required columns: {', '.join(missing)}")

    for table, required_columns in contract.get("required_tables", {}).items():
        observed = table_columns.get(table, set())
        if not observed:
            raise CompatibilityError(f"state database lacks required table: {table}")
        missing_table_columns = sorted(set(required_columns) - observed)
        if missing_table_columns:
            raise CompatibilityError(
                f"state database table {table} lacks required columns: "
                + ", ".join(missing_table_columns)
            )

    return CompatibilityResult(cli_version, source_commit, state_db, contract)
