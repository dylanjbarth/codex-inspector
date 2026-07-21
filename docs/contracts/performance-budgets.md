# Phase 0 performance and security budgets

These are hard upper bounds for the demo. A later phase may tighten them but
must not silently weaken them.

| Area | Budget |
| --- | --- |
| hook wall time | p95 <= 50 ms, hard timeout 2 s |
| hook marker size | <= 4 KiB; identifiers and locator only |
| index parsing concurrency | min(4, logical CPU count), configurable downward |
| SQLite writers | exactly 1 |
| normalization transaction | <= 500 records or 8 MiB parsed input, whichever comes first |
| cooperative yield | after each transaction and at least every 100 ms of parsing |
| API list page | default 50, maximum 200 items |
| metric buckets | maximum 2,000 per response |
| metric series | maximum 32 per response |
| JSON response body | maximum 2 MiB except chunked evidence |
| evidence range request | default 64 KiB, maximum 256 KiB |
| review report | maximum 1 MiB; at most 5 findings |
| SSE event | maximum 16 KiB; no raw payloads |
| browser heartbeat | every 15 s; stale after 45 s |
| idle shutdown | 120 s without heartbeat, indexing, or queued work |
| authenticated startup reuse probe | <= 500 ms |
| source fingerprint prefix | first 64 KiB plus stable metadata |

All loopback responses set a restrictive CSP, prohibit remote assets, and use
opaque evidence IDs. Diagnostics, status, logs, hook markers, and SSE events
must never contain raw message/tool payloads, access tokens, cookie values, or
absolute source paths.
