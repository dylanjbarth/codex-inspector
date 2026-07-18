---
name: inspect-session
description: Open Codex Inspector for the current session when safely identifiable.
---

Run `codex-inspector open --current-session`. Inspector may use an in-scope
hook marker. Never guess a session from cwd or timestamps. If no reliable ID
exists, leave Inspector on its discovery fallback with the unavailable notice.
