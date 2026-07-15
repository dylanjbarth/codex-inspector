# Core wireframe state matrix

| Surface | State | What the user sees | Available action |
| --- | --- | --- | --- |
| Dashboard | Populated | Metrics, deterministic findings, recent sessions | Filter, search, inspect a session |
| Dashboard | Loading | Local indexing progress and reserved content structure | Wait or keep the page open |
| Dashboard | Empty | No indexed sessions and an explanation of local scanning | Scan local history |
| Dashboard | Error | Unsupported record format with parser and CLI versions | View diagnostic details |
| Context inspector | Populated with finding | Session finder, turn selector, context components, evidence detail | Switch session/turn/component |
| Context inspector | Populated without finding | Recorded context with no notable deterministic finding | Inspect components or switch session |
| Context inspector | Loading | Selected session identity plus context-loading status | Return to dashboard |
| Context inspector | Empty | No context records for the selected session | Switch session |
| Context inspector | Error | Context adapter failure without implying source data loss | View diagnostics or switch session |

The prototype exposes the global populated, loading, empty, and error states through a clearly labeled wireframe control. Session selection demonstrates the populated-with-finding and populated-without-finding variants.
