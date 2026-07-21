from __future__ import annotations

import json
from pathlib import Path
import re
from typing import Any


_PATTERNS: tuple[tuple[re.Pattern[str], str], ...] = (
    (re.compile(r"\bsk-[A-Za-z0-9_-]{8,}\b"), "[REDACTED:openai-key]"),
    (re.compile(r"\b(?:ghp|github_pat)_[A-Za-z0-9_]{8,}\b"), "[REDACTED:github-token]"),
    (re.compile(r"(?i)(authorization\s*[:=]\s*)([^\s,;]+)"), r"\1[REDACTED]"),
    (re.compile(r"(?i)(cookie\s*[:=]\s*)([^\r\n]+)"), r"\1[REDACTED]"),
    (
        re.compile(
            r"(?i)\b([A-Z0-9_]*(?:TOKEN|SECRET|PASSWORD|API_KEY))\s*=\s*([^\s,;]+)"
        ),
        r"\1=[REDACTED]",
    ),
    (re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----"), "[REDACTED:private-key]"),
)


def redact_text(value: Any, *, limit: int = 2000) -> str:
    if isinstance(value, (dict, list)):
        text = json.dumps(value, sort_keys=True, ensure_ascii=False)
    elif value is None:
        text = ""
    else:
        text = str(value)
    home = str(Path.home())
    if home:
        text = text.replace(home, "~")
    for pattern, replacement in _PATTERNS:
        text = pattern.sub(replacement, text)
    if len(text) > limit:
        text = text[:limit] + f"… [truncated {len(text) - limit} chars]"
    return text
