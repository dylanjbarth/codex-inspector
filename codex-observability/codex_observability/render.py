from __future__ import annotations

from datetime import datetime, timezone
from html import escape
import json
from pathlib import Path
from typing import Any

from .redact import redact_text


def _display(value: Any) -> str:
    if value is None:
        return "Unavailable"
    if isinstance(value, bool):
        return "Yes" if value else "No"
    if isinstance(value, float):
        return f"{value:,.2f}"
    if isinstance(value, int):
        return f"{value:,}"
    return str(value)


def _metric(label: str, value: Any, hint: str = "") -> str:
    return (
        '<div class="metric">'
        f'<div class="metric-label">{escape(label)}</div>'
        f'<div class="metric-value">{escape(_display(value))}</div>'
        f'<div class="metric-hint">{escape(hint)}</div>'
        "</div>"
    )


def _human_duration(milliseconds: int | None) -> str:
    if milliseconds is None:
        return "Unavailable"
    total_seconds = max(0, round(milliseconds / 1000))
    days, remainder = divmod(total_seconds, 86400)
    hours, remainder = divmod(remainder, 3600)
    minutes, seconds = divmod(remainder, 60)
    if days:
        return f"{days}d {hours}h {minutes}m"
    if hours:
        return f"{hours}h {minutes}m {seconds}s"
    if minutes:
        return f"{minutes}m {seconds}s"
    return f"{seconds} seconds"


def _human_time(milliseconds: int | None) -> str:
    if not milliseconds:
        return "Unavailable"
    return datetime.fromtimestamp(milliseconds / 1000, tz=timezone.utc).strftime("%Y-%m-%d %H:%M UTC")


def _rows(values: list[tuple[str, Any]]) -> str:
    return "".join(
        f"<tr><th>{escape(label)}</th><td>{escape(_display(value))}</td></tr>"
        for label, value in values
    )


def _counter_rows(values: dict[str, int]) -> str:
    if not values:
        return '<tr><td colspan="2" class="empty">No observations available.</td></tr>'
    return "".join(
        f"<tr><th>{escape(name)}</th><td>{count:,}</td></tr>" for name, count in values.items()
    )


def _deep_redact(value: Any) -> Any:
    if isinstance(value, dict):
        return {str(key): _deep_redact(item) for key, item in value.items()}
    if isinstance(value, list):
        return [_deep_redact(item) for item in value]
    if isinstance(value, str):
        return redact_text(value)
    return value


def _safe_json(value: Any) -> str:
    text = json.dumps(_deep_redact(value), ensure_ascii=False, sort_keys=True)
    return (
        text.replace("&", "\\u0026")
        .replace("<", "\\u003c")
        .replace(">", "\\u003e")
        .replace("\u2028", "\\u2028")
        .replace("\u2029", "\\u2029")
    )


def _pretty_javascript(source: str) -> str:
    """Add conservative indentation without changing quoted content."""
    output: list[str] = []
    indent = 0
    quote: str | None = None
    escaped = False

    def newline() -> None:
        while output and output[-1] == " ":
            output.pop()
        if output and output[-1] != "\n":
            output.append("\n")
        output.append("  " * indent)

    for character in source.strip():
        if quote is not None:
            output.append(character)
            if escaped:
                escaped = False
            elif character == "\\":
                escaped = True
            elif character == quote:
                quote = None
            continue
        if character in {'"', "'", "`"}:
            quote = character
            output.append(character)
        elif character == "{":
            output.append(character)
            indent += 1
            newline()
        elif character == "}":
            indent = max(0, indent - 1)
            newline()
            output.append(character)
        elif character == "," and indent:
            output.append(character)
            newline()
        elif character == ";":
            output.append(character)
            newline()
        else:
            output.append(character)
    return "\n".join(line.rstrip() for line in "".join(output).strip().splitlines())


def _format_preview(value: Any) -> tuple[str, str]:
    if value is None or value == "":
        return "Empty", "No preview available."
    parsed: Any = value
    if isinstance(value, str):
        source = value.strip()
        try:
            parsed = json.loads(source)
        except (json.JSONDecodeError, TypeError):
            code_markers = ("const ", "let ", "var ", "await ", "return ", "async ")
            if source.startswith(code_markers) or "await tools." in source:
                return "JavaScript", _pretty_javascript(source)
            return "Text", source.replace("\r\n", "\n").replace("\r", "\n")

    if (
        isinstance(parsed, list)
        and parsed
        and all(isinstance(item, dict) and "text" in item for item in parsed)
    ):
        blocks: list[str] = []
        for index, item in enumerate(parsed, start=1):
            block_type = str(item.get("type") or f"block-{index}").replace("_", " ")
            blocks.append(f"[{index}] {block_type}")
            text_value = item.get("text")
            if isinstance(text_value, str):
                blocks.append(text_value.rstrip())
            else:
                blocks.append(json.dumps(text_value, ensure_ascii=False, indent=2, sort_keys=True))
            metadata = {key: item[key] for key in item if key not in {"type", "text"}}
            if metadata:
                blocks.append(json.dumps(metadata, ensure_ascii=False, indent=2, sort_keys=True))
        return "Content blocks", "\n\n".join(blocks)

    if isinstance(parsed, (dict, list)):
        return "JSON", json.dumps(parsed, ensure_ascii=False, indent=2, sort_keys=True)
    if isinstance(parsed, str):
        return "Text", parsed
    return "Value", json.dumps(parsed, ensure_ascii=False)


def _detail_panel(title: str, value: Any, kind: str) -> str:
    format_name, formatted = _format_preview(value)
    line_count = max(1, formatted.count("\n") + 1)
    return (
        f'<section class="detail-panel detail-{escape(kind)}">'
        '<div class="detail-toolbar">'
        f'<span class="detail-title">{escape(title)}</span>'
        f'<span class="detail-format" data-format="{escape(format_name)}">'
        f'{escape(format_name)} · {line_count:,} {"line" if line_count == 1 else "lines"}</span>'
        '</div>'
        f'<pre class="detail-code"><code>{escape(formatted)}</code></pre>'
        '</section>'
    )


CATEGORY_ICONS = {
    "sessions": '<svg viewBox="0 0 24 24"><rect x="4" y="5" width="12" height="14" rx="2"/><path d="M8 2h10a2 2 0 0 1 2 2v11M8 9h4M8 13h4"/></svg>',
    "coverage": '<svg viewBox="0 0 24 24"><path d="M12 3 20 6v5c0 5-3.3 8.4-8 10-4.7-1.6-8-5-8-10V6z"/><path d="m8.5 12 2.2 2.2 4.8-5"/></svg>',
    "tokens": '<svg viewBox="0 0 24 24"><circle cx="9" cy="9" r="5"/><path d="M7 9h4M9 7v4M13 14.5A5 5 0 1 0 19 9"/></svg>',
    "tools": '<svg viewBox="0 0 24 24"><path d="M14.5 6.5a4 4 0 0 0-5-5L12 4l-3 3-2.5-2.5a4 4 0 0 0 5 5L20 18l-2 2-8.5-8.5"/></svg>',
    "delegation": '<svg viewBox="0 0 24 24"><circle cx="12" cy="5" r="2.5"/><circle cx="5" cy="17" r="2.5"/><circle cx="19" cy="17" r="2.5"/><path d="M12 7.5v3M12 10.5 6.5 14M12 10.5l5.5 3.5"/></svg>',
    "models": '<svg viewBox="0 0 24 24"><rect x="5" y="5" width="14" height="14" rx="3"/><path d="M9 1v4M15 1v4M9 19v4M15 19v4M1 9h4M1 15h4M19 9h4M19 15h4M9 9h6v6H9z"/></svg>',
    "reasoning": '<svg viewBox="0 0 24 24"><path d="M9 18h6M10 22h4M8.5 15.5A7 7 0 1 1 15.5 15.5c-1 .8-1.5 1.4-1.5 2.5h-4c0-1.1-.5-1.7-1.5-2.5z"/><path d="M12 6v5M9.5 8.5 12 11l2.5-2.5"/></svg>',
    "recent": '<svg viewBox="0 0 24 24"><path d="M4.5 9A8 8 0 1 1 4 14M4.5 9H1M4.5 9V5.5"/><path d="M12 7v5l3 2"/></svg>',
    "agents": '<svg viewBox="0 0 24 24"><circle cx="12" cy="7" r="3"/><path d="M6.5 20v-2a5.5 5.5 0 0 1 11 0v2M4 11a3 3 0 0 0 0 6M20 11a3 3 0 0 1 0 6"/></svg>',
    "verification": '<svg viewBox="0 0 24 24"><path d="M6 3h9l4 4v14H6zM15 3v5h4"/><path d="m9 14 2 2 4-4"/></svg>',
    "context": '<svg viewBox="0 0 24 24"><path d="m12 3 9 5-9 5-9-5zM3 12l9 5 9-5M3 16l9 5 9-5"/></svg>',
    "timeline": '<svg viewBox="0 0 24 24"><circle cx="6" cy="6" r="2"/><circle cx="18" cy="12" r="2"/><circle cx="8" cy="19" r="2"/><path d="M8 6h3a4 4 0 0 1 4 4M16.5 14 10 18"/></svg>',
    "provenance": '<svg viewBox="0 0 24 24"><path d="M12 3a7 7 0 0 0-7 7v2M19 10a7 7 0 0 0-4-6.3M8 21a9 9 0 0 1-5-8M12 7a3 3 0 0 0-3 3v4a5 5 0 0 1-2 4M15 10v4a8 8 0 0 1-1.2 4.2M12 10v4a8 8 0 0 1-3 6"/></svg>',
}


def _category_icon(name: str) -> str:
    return (
        f'<span class="category-icon" data-icon="{escape(name)}" aria-hidden="true">'
        f'{CATEGORY_ICONS[name]}</span>'
    )


def _category_heading(icon: str, eyebrow: str, title: str) -> str:
    return (
        '<div class="category-heading">'
        f'{_category_icon(icon)}'
        f'<div><div class="eyebrow">{escape(eyebrow)}</div><h2>{escape(title)}</h2></div>'
        '</div>'
    )


def _advanced_summary(icon: str, title: str) -> str:
    return (
        '<summary><span class="summary-heading">'
        f'{_category_icon(icon)}<span>{escape(title)}</span>'
        '</span></summary>'
    )


def _highlight_card(title: str, items: list[dict[str, Any]], kind: str) -> str:
    icon = "✓" if kind == "positive" else "!"
    body: list[str] = []
    for item in items:
        action = ""
        if item.get("nextAction"):
            action = f'<p class="next"><strong>Next:</strong> {escape(item["nextAction"])}</p>'
        evidence = ", ".join(item.get("sessionIds", []))
        body.append(
            '<div class="highlight-item">'
            f'<h3>{escape(item["title"])}</h3>'
            f'<p>{escape(item["observed"])}</p>'
            f'<p class="impact">{escape(item["impact"])}</p>'
            f'{action}'
            f'<details><summary>Evidence · {escape(item["confidence"])} confidence</summary>'
            f'<code>{escape(evidence or "No representative session available")}</code></details>'
            '</div>'
        )
    if not body:
        body.append(
            '<p class="empty">No sufficiently supported pattern was found. The plugin stays quiet instead of guessing.</p>'
        )
    return (
        f'<article class="highlight highlight-{kind}" aria-label="{escape(title)}">'
        f'<div class="highlight-heading"><span class="highlight-icon" aria-hidden="true">{icon}</span>'
        f'<div><div class="eyebrow">Evidence-backed coaching</div><h2>{escape(title)}</h2></div></div>'
        f'{"".join(body)}</article>'
    )


COMMON_CSS = """
:root{color-scheme:light dark;--bg:#f3f6fb;--panel:#fff;--panel2:#f8fafc;--text:#101827;--muted:#64748b;--line:#dbe3ef;--blue:#2563eb;--cyan:#06b6d4;--good:#087a55;--good-bg:#eafaf3;--good-line:#66d2a9;--bad:#b4232d;--bad-bg:#fff0f1;--bad-line:#f29aa1;--warn:#b45309;--shadow:0 18px 50px rgba(15,23,42,.08)}
@media(prefers-color-scheme:dark){:root{--bg:#07111f;--panel:#0f1b2e;--panel2:#111f34;--text:#e7eef9;--muted:#9aa9bd;--line:#273951;--blue:#6aa8ff;--cyan:#22d3ee;--good:#78e3b8;--good-bg:#0b2a22;--good-line:#21765a;--bad:#ff9aa2;--bad-bg:#32161c;--bad-line:#863d48;--warn:#fbbf24;--shadow:0 20px 55px rgba(0,0,0,.3)}}
*{box-sizing:border-box}html{scroll-behavior:smooth}body{margin:0;background:radial-gradient(circle at 10% 0%,color-mix(in srgb,var(--blue) 12%,transparent),transparent 35%),var(--bg);color:var(--text);font:15px/1.55 ui-sans-serif,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}.shell{width:min(1240px,calc(100% - 32px));margin:0 auto;padding:34px 0 72px}header{display:flex;gap:18px;align-items:center;margin-bottom:22px}.header-copy{transform:translateY(4px)}.header-copy .subtitle{transform:translateY(-11px)}.mark{width:68px;height:68px;border-radius:20px;padding:12px;background:linear-gradient(145deg,var(--blue),var(--cyan));box-shadow:var(--shadow);color:#fff;flex:0 0 auto}h1,h2,h3,p{margin-top:0}h1{margin-bottom:4px;font-size:clamp(30px,5vw,50px);letter-spacing:-.045em}h2{font-size:21px;letter-spacing:-.02em;margin-bottom:12px}h3{font-size:15px;margin-bottom:6px}.subtitle,.muted,.metric-hint,.eyebrow,.impact{color:var(--muted)}.eyebrow{text-transform:uppercase;font-size:11px;letter-spacing:.12em;font-weight:750}.notice{border:1px solid color-mix(in srgb,var(--warn) 38%,var(--line));background:color-mix(in srgb,var(--warn) 8%,var(--panel));padding:12px 16px;border-radius:14px;margin:0 0 20px}.bento{display:grid;grid-template-columns:repeat(12,minmax(0,1fr));gap:16px}.bento-card{background:color-mix(in srgb,var(--panel) 96%,transparent);border:1px solid var(--line);border-radius:22px;padding:21px;box-shadow:var(--shadow);min-width:0}.span-4{grid-column:span 4}.span-5{grid-column:span 5}.span-6{grid-column:span 6}.span-7{grid-column:span 7}.span-8{grid-column:span 8}.span-12{grid-column:span 12}.metrics{display:grid;grid-template-columns:repeat(auto-fit,minmax(145px,1fr));gap:10px}.metric{border:1px solid var(--line);border-radius:16px;padding:13px;min-height:96px;background:var(--panel2)}.metric-label{color:var(--muted);font-size:12px}.metric-value{font-size:23px;font-weight:780;letter-spacing:-.035em;margin:4px 0;overflow-wrap:anywhere}.metric-hint{font-size:11px}.highlight-grid{display:grid;grid-template-columns:1fr 1fr;gap:16px;margin:0 0 16px}.highlight{border-radius:24px;padding:22px;box-shadow:var(--shadow);border:1px solid}.highlight-positive{background:var(--good-bg);border-color:var(--good-line)}.highlight-improvement{background:var(--bad-bg);border-color:var(--bad-line)}.highlight-heading{display:flex;gap:12px;align-items:center}.highlight-icon{display:grid;place-items:center;width:36px;height:36px;border-radius:12px;font-weight:900;font-size:20px;background:var(--panel)}.highlight-positive .highlight-icon{color:var(--good)}.highlight-improvement .highlight-icon{color:var(--bad)}.highlight-item{padding:14px 0;border-top:1px solid color-mix(in srgb,currentColor 14%,transparent)}.highlight-item:first-of-type{border-top:0}.highlight-item p{margin-bottom:7px}.highlight-item .next{margin-top:10px}.highlight details{font-size:12px}.highlight code{display:block;margin-top:7px;overflow-wrap:anywhere}table{width:100%;border-collapse:collapse}th,td{padding:10px 8px;border-bottom:1px solid var(--line);text-align:left;vertical-align:top}th{color:var(--muted);font-weight:620}code,pre{font:12px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace}pre{white-space:pre-wrap;overflow-wrap:anywhere;background:var(--bg);border:1px solid var(--line);border-radius:12px;padding:12px;max-height:240px;overflow:auto}input,select{width:100%;color:var(--text);background:var(--panel2);border:1px solid var(--line);border-radius:12px;padding:11px 13px}.filters{display:grid;grid-template-columns:2fr 1fr;gap:10px;margin-bottom:12px}.table-wrap{overflow:auto}.status{display:inline-flex;padding:3px 8px;border-radius:99px;font-size:12px;background:var(--panel2)}.status-success{color:var(--good)}.status-failure{color:var(--bad)}.status-incomplete{color:var(--warn)}details summary{cursor:pointer;color:var(--blue)}.advanced>summary{font-weight:750;font-size:17px}.advanced[open]>summary{margin-bottom:16px}.timeline{list-style:none;padding:0;margin:0}.timeline-item{display:grid;grid-template-columns:minmax(160px,.6fr) minmax(130px,.5fr) 1fr;gap:12px;border-bottom:1px solid var(--line);padding:10px 0}.empty{color:var(--muted);font-style:italic}.pill{display:inline-flex;border:1px solid var(--line);border-radius:999px;padding:4px 9px;color:var(--muted);font-size:12px}footer{color:var(--muted);text-align:center;margin-top:28px}.section-gap{margin-top:16px}a{color:var(--blue)}
.category-heading{display:flex;align-items:flex-start;gap:12px;margin-bottom:14px}.category-heading h2{margin-bottom:0}.category-icon{display:grid;place-items:center;flex:0 0 auto;width:38px;height:38px;border:1px solid var(--line);border-radius:12px;background:var(--panel2);color:var(--blue)}.category-icon svg{width:21px;height:21px;fill:none;stroke:currentColor;stroke-width:1.8;stroke-linecap:round;stroke-linejoin:round}.summary-heading{display:inline-flex;align-items:center;gap:10px}.summary-heading .category-icon{width:32px;height:32px}.summary-heading .category-icon svg{width:18px;height:18px}
.tool-row td:last-child{min-width:390px;width:48%}.redacted-detail>summary{font-weight:700}.detail-panel{--detail-accent:var(--blue);margin-top:12px;border:1px solid var(--line);border-radius:15px;overflow:hidden;background:var(--bg)}.detail-output{--detail-accent:var(--cyan)}.detail-toolbar{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:9px 12px;background:color-mix(in srgb,var(--detail-accent) 8%,var(--panel2));border-bottom:1px solid var(--line)}.detail-title{font-weight:760;color:var(--text)}.detail-format{display:inline-flex;border:1px solid color-mix(in srgb,var(--detail-accent) 42%,var(--line));border-radius:999px;padding:2px 8px;color:var(--detail-accent);font-size:10px;letter-spacing:.04em;white-space:nowrap}.detail-code{margin:0;border:0;border-left:3px solid var(--detail-accent);border-radius:0;max-height:360px;padding:14px 15px;background:color-mix(in srgb,var(--bg) 92%,var(--detail-accent));font-size:12px;line-height:1.65;tab-size:2;white-space:pre-wrap;overflow-wrap:anywhere}.detail-code code{font:inherit}
.explorer{border:1px solid var(--line);border-radius:18px;overflow:hidden;background:var(--panel2)}.explorer-tabs{display:flex;gap:4px;padding:8px;border-bottom:1px solid var(--line);background:var(--panel);position:sticky;top:0;z-index:5}.explorer-tab{width:auto;border:1px solid transparent;border-radius:11px;padding:8px 13px;background:transparent;color:var(--muted);font-weight:750;cursor:pointer}.explorer-tab[aria-selected="true"]{color:var(--blue);border-color:var(--line);background:var(--panel2)}.explorer-panel{padding:14px}.explorer-panel[hidden]{display:none}.explorer-summary{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:8px;margin-bottom:12px}.explorer-summary-item{border:1px solid var(--line);border-radius:12px;padding:8px 10px;background:var(--panel)}.explorer-summary-label{display:block;color:var(--muted);font-size:10px;text-transform:uppercase;letter-spacing:.08em}.explorer-summary-value{display:block;font-size:17px;font-weight:780}.explorer-toolbar{display:grid;grid-template-columns:minmax(180px,1fr) auto;gap:8px;margin-bottom:8px}.explorer-toolbar input{min-width:0}.explorer-order{width:auto;min-width:132px;cursor:pointer}.explorer-filter-group{display:grid;grid-template-columns:76px minmax(0,1fr);align-items:center;gap:8px;margin:7px 0}.explorer-filter-label{color:var(--muted);font-size:11px;font-weight:700;text-transform:uppercase;letter-spacing:.07em}.explorer-chips{display:flex;gap:6px;overflow-x:auto;padding:2px 1px 5px;scrollbar-width:thin}.explorer-chip{width:auto;flex:0 0 auto;border:1px solid var(--line);border-radius:999px;padding:5px 9px;background:var(--panel);color:var(--muted);font-size:11px;cursor:pointer}.explorer-chip[aria-pressed="true"]{color:var(--blue);border-color:color-mix(in srgb,var(--blue) 55%,var(--line));background:color-mix(in srgb,var(--blue) 8%,var(--panel))}.explorer-result-count{color:var(--muted);font-size:11px;margin:8px 0}.explorer-viewport{height:clamp(420px,60vh,680px);overflow:auto;border:1px solid var(--line);border-radius:13px;background:var(--panel)}.explorer-table{min-width:720px}.explorer-table thead{position:sticky;top:0;z-index:3;background:var(--panel2);box-shadow:0 1px 0 var(--line)}.explorer-row{height:48px}.explorer-row:hover{background:color-mix(in srgb,var(--blue) 5%,var(--panel))}.explorer-row td{white-space:nowrap;max-width:320px;overflow:hidden;text-overflow:ellipsis}.explorer-action{width:auto;border:1px solid var(--line);border-radius:9px;padding:5px 9px;background:var(--panel2);color:var(--blue);font-size:11px;cursor:pointer}.explorer-pagination{display:grid;grid-template-columns:1fr auto 1fr;align-items:center;gap:8px;margin-top:10px}.explorer-pagination-side{display:flex;gap:6px}.explorer-pagination-side:last-child{justify-content:flex-end}.explorer-page-button{width:auto;border:1px solid var(--line);border-radius:9px;padding:6px 9px;background:var(--panel);color:var(--blue);font-size:11px;cursor:pointer}.explorer-page-button:disabled{opacity:.42;cursor:not-allowed}.explorer-page-label{color:var(--muted);font-size:12px;text-align:center}.drawer-backdrop{position:fixed;inset:0;z-index:80;background:rgba(2,8,23,.62);display:flex;justify-content:flex-end}.drawer-backdrop[hidden]{display:none}.explorer-drawer{width:clamp(440px,44vw,720px);height:100%;display:grid;grid-template-rows:auto minmax(0,1fr) auto;background:var(--panel);border-left:1px solid var(--line);box-shadow:-22px 0 60px rgba(0,0,0,.28)}.drawer-header{display:flex;align-items:flex-start;justify-content:space-between;gap:16px;padding:18px;border-bottom:1px solid var(--line)}.drawer-header h2{margin:2px 0 0}.drawer-close{width:auto;border:1px solid var(--line);border-radius:10px;padding:7px 10px;background:var(--panel2);color:var(--text);cursor:pointer}.drawer-body{padding:18px;overflow:auto}.drawer-meta{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px;margin-bottom:14px}.drawer-meta-item{border:1px solid var(--line);border-radius:12px;padding:9px;background:var(--panel2)}.drawer-meta-label{display:block;color:var(--muted);font-size:10px;text-transform:uppercase;letter-spacing:.07em}.drawer-meta-value{display:block;margin-top:2px;overflow-wrap:anywhere}.drawer-evidence{margin:12px 0}.drawer-evidence code{display:block;margin-top:5px;padding:8px;border:1px solid var(--line);border-radius:9px;background:var(--bg);overflow-wrap:anywhere}.drawer-footer{display:grid;grid-template-columns:1fr auto 1fr;align-items:center;gap:8px;padding:12px 18px;border-top:1px solid var(--line);background:var(--panel2)}.drawer-footer button{width:auto}.drawer-footer button:last-child{justify-self:end}.drawer-position{color:var(--muted);font-size:12px}.drawer-open{overflow:hidden}
@media(max-width:900px){.span-4,.span-5,.span-6,.span-7,.span-8{grid-column:span 12}.highlight-grid{grid-template-columns:1fr}}
@media(max-width:700px){.shell{width:min(100% - 20px,1240px);padding-top:20px}header{align-items:flex-start}.mark{width:54px;height:54px}.bento-card,.highlight{padding:16px}.timeline-item{grid-template-columns:1fr;gap:2px}.filters{grid-template-columns:1fr}.tool-row td:last-child{min-width:320px}.detail-toolbar{align-items:flex-start;flex-direction:column;gap:5px}.explorer-panel{padding:10px}.explorer-summary{grid-template-columns:repeat(2,minmax(0,1fr))}.explorer-toolbar{grid-template-columns:1fr}.explorer-order{width:100%}.explorer-filter-group{grid-template-columns:1fr;gap:3px}.explorer-viewport{height:60vh}.explorer-pagination{grid-template-columns:1fr;justify-items:center}.explorer-pagination-side,.explorer-pagination-side:last-child{justify-content:center}.drawer-backdrop{align-items:flex-end}.explorer-drawer{width:100%;height:min(88vh,760px);border-left:0;border-top:1px solid var(--line);border-radius:20px 20px 0 0}.drawer-meta{grid-template-columns:1fr}}
"""

COMMON_CSS += """
.drawer-header{margin-bottom:0}
.drawer-footer{margin-top:0;color:var(--text);text-align:initial}
@media(max-width:700px){.explorer-viewport{height:clamp(420px,60vh,680px)}}
"""


ADVANCED_EXPLORER_HTML = """
<details id="advanced-explorer" class="bento-card span-12 advanced">
  <summary><span class="summary-heading"><span class="category-icon" data-icon="timeline" aria-hidden="true"><svg viewBox="0 0 24 24"><circle cx="6" cy="6" r="2"/><circle cx="18" cy="12" r="2"/><circle cx="8" cy="19" r="2"/><path d="M8 6h3a4 4 0 0 1 4 4M16.5 14 10 18"/></svg></span><span>Granular tools and timeline</span></span></summary>
  <div class="explorer" id="session-explorer">
    <div class="explorer-tabs" role="tablist" aria-label="Session records">
      <button class="explorer-tab" id="explorer-tab-tools" type="button" role="tab" aria-selected="true" aria-controls="explorer-tools">Tools <span id="tools-tab-count"></span></button>
      <button class="explorer-tab" id="explorer-tab-timeline" type="button" role="tab" aria-selected="false" aria-controls="timeline" tabindex="-1">Timeline <span id="timeline-tab-count"></span></button>
    </div>
    <section class="explorer-panel" id="explorer-tools" role="tabpanel" aria-labelledby="explorer-tab-tools">
      <div class="explorer-summary" id="tools-summary" aria-label="Tool summary"></div>
      <div class="explorer-toolbar"><input id="tool-search" type="search" placeholder="Filter by tool, category, or status" aria-label="Filter tools"><button class="explorer-order" id="tools-order" type="button">Oldest first</button></div>
      <div class="explorer-filter-group"><span class="explorer-filter-label">Category</span><div class="explorer-chips" id="tools-category-filters" aria-label="Tool category filters"></div></div>
      <div class="explorer-filter-group"><span class="explorer-filter-label">Status</span><div class="explorer-chips" id="tools-status-filters" aria-label="Tool status filters"></div></div>
      <p class="explorer-result-count" id="tools-result-count" aria-live="polite"></p>
      <div class="explorer-viewport" id="tools-viewport"><table class="explorer-table"><thead><tr><th>Tool</th><th>Category</th><th>Status</th><th>Duration</th><th>Details</th></tr></thead><tbody id="tool-rows"></tbody></table></div>
      <nav class="explorer-pagination" aria-label="Tool pages">
        <div class="explorer-pagination-side"><button class="explorer-page-button" type="button" data-kind="tools" data-page-action="first">First</button><button class="explorer-page-button" type="button" data-kind="tools" data-page-action="previous">Previous</button></div>
        <span class="explorer-page-label" id="tools-page-label"></span>
        <div class="explorer-pagination-side"><button class="explorer-page-button" type="button" data-kind="tools" data-page-action="next">Next</button><button class="explorer-page-button" type="button" data-kind="tools" data-page-action="last">Last</button></div>
      </nav>
    </section>
    <section class="explorer-panel" id="timeline" role="tabpanel" aria-labelledby="explorer-tab-timeline" hidden>
      <div class="explorer-summary" id="timeline-summary" aria-label="Timeline summary"></div>
      <div class="explorer-toolbar"><input id="timeline-search" type="search" placeholder="Filter by event type, time, or evidence" aria-label="Filter timeline"><button class="explorer-order" id="timeline-order" type="button">Oldest first</button></div>
      <div class="explorer-filter-group"><span class="explorer-filter-label">Event type</span><div class="explorer-chips" id="timeline-type-filters" aria-label="Timeline event type filters"></div></div>
      <p class="explorer-result-count" id="timeline-result-count" aria-live="polite"></p>
      <div class="explorer-viewport" id="timeline-viewport"><table class="explorer-table"><thead><tr><th>Timestamp</th><th>Event type</th><th>Evidence</th><th>Details</th></tr></thead><tbody id="timeline-rows"></tbody></table></div>
      <nav class="explorer-pagination" aria-label="Timeline pages">
        <div class="explorer-pagination-side"><button class="explorer-page-button" type="button" data-kind="timeline" data-page-action="first">First</button><button class="explorer-page-button" type="button" data-kind="timeline" data-page-action="previous">Previous</button></div>
        <span class="explorer-page-label" id="timeline-page-label"></span>
        <div class="explorer-pagination-side"><button class="explorer-page-button" type="button" data-kind="timeline" data-page-action="next">Next</button><button class="explorer-page-button" type="button" data-kind="timeline" data-page-action="last">Last</button></div>
      </nav>
    </section>
  </div>
</details>
<div class="drawer-backdrop" id="drawer-backdrop" hidden>
  <aside class="explorer-drawer" id="record-drawer" role="dialog" aria-modal="true" aria-labelledby="drawer-title" tabindex="-1">
    <header class="drawer-header"><div><div class="eyebrow" id="drawer-kind"></div><h2 id="drawer-title">Record details</h2></div><button class="drawer-close" id="drawer-close" type="button" aria-label="Close record details">Close</button></header>
    <div class="drawer-body" id="drawer-body"></div>
    <footer class="drawer-footer"><button class="explorer-page-button" id="drawer-previous" type="button">Previous</button><span class="drawer-position" id="drawer-position"></span><button class="explorer-page-button" id="drawer-next" type="button">Next</button></footer>
  </aside>
</div>
"""


EXPLORER_SCRIPT = r"""
(() => {
  const PAGE_SIZE = 25;
  const report = JSON.parse(document.getElementById('report-data').textContent);
  const sources = {
    tools: Array.isArray(report.toolActivity?.calls) ? report.toolActivity.calls : [],
    timeline: Array.isArray(report.timeline) ? report.timeline : []
  };
  const states = {
    tools: {page: 1, query: '', order: 'asc', category: 'All', status: 'All'},
    timeline: {page: 1, query: '', order: 'asc', type: 'All'}
  };
  let activeTab = 'tools';
  let drawerContext = null;
  let restoreFocus = null;

  const byId = id => document.getElementById(id);
  const make = (tag, className, text) => {
    const node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined && text !== null) node.textContent = String(text);
    return node;
  };
  const display = value => value === null || value === undefined || value === '' ? 'Unavailable' : String(value);
  const countBy = (items, key) => items.reduce((counts, item) => {
    const value = display(item[key]);
    counts[value] = (counts[value] || 0) + 1;
    return counts;
  }, {});
  const orderedCounts = counts => Object.entries(counts).sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
  const recordTime = (kind, item) => {
    const raw = kind === 'tools' ? (item.startedAt || item.completedAt) : item.timestamp;
    const value = Date.parse(raw || '');
    return Number.isNaN(value) ? null : value;
  };

  function filtered(kind) {
    const state = states[kind];
    const query = state.query.trim().toLowerCase();
    const wrapped = sources[kind].map((item, index) => ({item, index})).filter(({item}) => {
      if (kind === 'tools') {
        if (state.category !== 'All' && display(item.category) !== state.category) return false;
        if (state.status !== 'All' && display(item.status) !== state.status) return false;
        const haystack = [item.name, item.category, item.status].map(display).join(' ').toLowerCase();
        return !query || haystack.includes(query);
      }
      if (state.type !== 'All' && display(item.type) !== state.type) return false;
      const haystack = [item.timestamp, item.type, item.evidence].map(display).join(' ').toLowerCase();
      return !query || haystack.includes(query);
    });
    wrapped.sort((left, right) => {
      const a = recordTime(kind, left.item);
      const b = recordTime(kind, right.item);
      if (a === null && b === null) return left.index - right.index;
      if (a === null) return 1;
      if (b === null) return -1;
      const comparison = a - b || left.index - right.index;
      return state.order === 'asc' ? comparison : -comparison;
    });
    return wrapped.map(entry => entry.item);
  }

  function renderSummary(container, entries) {
    container.replaceChildren();
    entries.forEach(([label, value]) => {
      const item = make('div', 'explorer-summary-item');
      item.append(make('span', 'explorer-summary-label', label), make('span', 'explorer-summary-value', Number(value).toLocaleString()));
      container.append(item);
    });
  }

  function renderChips(container, entries, kind, key) {
    container.replaceChildren();
    entries.forEach(([label, count]) => {
      const button = make('button', 'explorer-chip', `${label} · ${Number(count).toLocaleString()}`);
      button.type = 'button';
      button.setAttribute('aria-pressed', String(states[kind][key] === label));
      button.addEventListener('click', () => {
        closeDrawer();
        states[kind][key] = label;
        states[kind].page = 1;
        render(kind);
      });
      container.append(button);
    });
  }

  function cell(value) {
    return make('td', '', display(value));
  }

  function detailsButton(kind, index) {
    const button = make('button', 'explorer-action', 'View');
    button.type = 'button';
    button.addEventListener('click', event => openDrawer(kind, index, event.currentTarget));
    return button;
  }

  function renderToolRow(item, resultIndex) {
    const row = make('tr', 'explorer-row');
    row.append(cell(item.name), cell(item.category));
    const statusCell = make('td');
    const status = make('span', `status status-${['success','failure','incomplete'].includes(item.status) ? item.status : 'incomplete'}`, display(item.status));
    statusCell.append(status);
    const duration = Number.isFinite(item.durationMs) ? `${Number(item.durationMs).toLocaleString()} ms` : 'Unavailable';
    const actionCell = make('td');
    actionCell.append(detailsButton('tools', resultIndex));
    row.append(statusCell, cell(duration), actionCell);
    return row;
  }

  function renderTimelineRow(item, resultIndex) {
    const row = make('tr', 'explorer-row');
    const actionCell = make('td');
    actionCell.append(detailsButton('timeline', resultIndex));
    row.append(cell(item.timestamp), cell(item.type), cell(item.evidence), actionCell);
    return row;
  }

  function updatePagination(kind, resultCount, pageCount) {
    const page = states[kind].page;
    byId(`${kind}-page-label`).textContent = `Page ${page.toLocaleString()} of ${pageCount.toLocaleString()}`;
    document.querySelectorAll(`[data-kind="${kind}"][data-page-action]`).forEach(button => {
      const action = button.dataset.pageAction;
      button.disabled = action === 'first' || action === 'previous' ? page <= 1 : page >= pageCount;
    });
    const start = resultCount ? (page - 1) * PAGE_SIZE + 1 : 0;
    const end = Math.min(page * PAGE_SIZE, resultCount);
    byId(`${kind}-result-count`).textContent = `Showing ${start.toLocaleString()}–${end.toLocaleString()} of ${resultCount.toLocaleString()} records`;
  }

  function render(kind) {
    const results = filtered(kind);
    const pageCount = Math.max(1, Math.ceil(results.length / PAGE_SIZE));
    states[kind].page = Math.min(Math.max(1, states[kind].page), pageCount);
    const start = (states[kind].page - 1) * PAGE_SIZE;
    const page = results.slice(start, start + PAGE_SIZE);
    const body = byId(kind === 'tools' ? 'tool-rows' : 'timeline-rows');
    body.replaceChildren();
    if (!page.length) {
      const row = make('tr');
      const empty = make('td', 'empty', 'No matching records.');
      empty.colSpan = kind === 'tools' ? 5 : 4;
      row.append(empty);
      body.append(row);
    } else {
      page.forEach((item, offset) => body.append(kind === 'tools' ? renderToolRow(item, start + offset) : renderTimelineRow(item, start + offset)));
    }
    byId(`${kind}-viewport`).scrollTop = 0;
    byId(`${kind}-order`).textContent = states[kind].order === 'asc' ? 'Oldest first' : 'Newest first';
    updatePagination(kind, results.length, pageCount);

    if (kind === 'tools') {
      const categories = orderedCounts(countBy(sources.tools, 'category'));
      const statuses = ['success', 'failure', 'incomplete'].map(value => [value, sources.tools.filter(item => item.status === value).length]);
      renderChips(byId('tools-category-filters'), [['All', sources.tools.length], ...categories], 'tools', 'category');
      renderChips(byId('tools-status-filters'), [['All', sources.tools.length], ...statuses], 'tools', 'status');
    } else {
      const types = orderedCounts(countBy(sources.timeline, 'type'));
      renderChips(byId('timeline-type-filters'), [['All', sources.timeline.length], ...types], 'timeline', 'type');
    }
  }

  function switchTab(kind, focusTab = false) {
    activeTab = kind;
    ['tools', 'timeline'].forEach(name => {
      const selected = name === kind;
      const tab = byId(`explorer-tab-${name}`);
      const panel = byId(name === 'tools' ? 'explorer-tools' : 'timeline');
      tab.setAttribute('aria-selected', String(selected));
      tab.tabIndex = selected ? 0 : -1;
      panel.hidden = !selected;
    });
    if (focusTab) byId(`explorer-tab-${kind}`).focus();
  }

  function prettyJavaScript(source) {
    const output = [];
    let indent = 0;
    let quote = null;
    let escaped = false;
    const newline = () => {
      while (output[output.length - 1] === ' ') output.pop();
      if (output.length && output[output.length - 1] !== '\n') output.push('\n');
      output.push('  '.repeat(indent));
    };
    for (const character of source.trim()) {
      if (quote !== null) {
        output.push(character);
        if (escaped) escaped = false;
        else if (character === '\\') escaped = true;
        else if (character === quote) quote = null;
        continue;
      }
      if ('"\'`'.includes(character)) { quote = character; output.push(character); }
      else if (character === '{') { output.push(character); indent += 1; newline(); }
      else if (character === '}') { indent = Math.max(0, indent - 1); newline(); output.push(character); }
      else if (character === ',' && indent) { output.push(character); newline(); }
      else if (character === ';') { output.push(character); newline(); }
      else output.push(character);
    }
    return output.join('').trim().split('\n').map(line => line.trimEnd()).join('\n');
  }

  function formatPreview(value) {
    if (value === null || value === undefined || value === '') return ['Empty', 'No preview available.'];
    let parsed = value;
    if (typeof value === 'string') {
      const source = value.trim();
      try { parsed = JSON.parse(source); }
      catch (_) {
        const markers = ['const ', 'let ', 'var ', 'await ', 'return ', 'async '];
        if (markers.some(marker => source.startsWith(marker)) || source.includes('await tools.')) return ['JavaScript', prettyJavaScript(source)];
        return ['Text', source.replace(/\r\n?/g, '\n')];
      }
    }
    if (Array.isArray(parsed) && parsed.length && parsed.every(item => item && typeof item === 'object' && Object.prototype.hasOwnProperty.call(item, 'text'))) {
      const blocks = [];
      parsed.forEach((item, index) => {
        blocks.push(`[${index + 1}] ${display(item.type || `block-${index + 1}`).replaceAll('_', ' ')}`);
        blocks.push(typeof item.text === 'string' ? item.text.trimEnd() : JSON.stringify(item.text, null, 2));
        const metadata = Object.fromEntries(Object.entries(item).filter(([key]) => !['type', 'text'].includes(key)));
        if (Object.keys(metadata).length) blocks.push(JSON.stringify(metadata, null, 2));
      });
      return ['Content blocks', blocks.join('\n\n')];
    }
    if (parsed && typeof parsed === 'object') return ['JSON', JSON.stringify(parsed, null, 2)];
    if (typeof parsed === 'string') return ['Text', parsed];
    return ['Value', JSON.stringify(parsed)];
  }

  function detailPanel(title, value, kind) {
    const [format, formatted] = formatPreview(value);
    const section = make('section', `detail-panel detail-${kind}`);
    const toolbar = make('div', 'detail-toolbar');
    const lines = Math.max(1, formatted.split('\n').length);
    const badge = make('span', 'detail-format', `${format} · ${lines.toLocaleString()} ${lines === 1 ? 'line' : 'lines'}`);
    badge.dataset.format = format;
    toolbar.append(make('span', 'detail-title', title), badge);
    const pre = make('pre', 'detail-code');
    pre.append(make('code', '', formatted));
    section.append(toolbar, pre);
    return section;
  }

  function metaItem(label, value) {
    const item = make('div', 'drawer-meta-item');
    item.append(make('span', 'drawer-meta-label', label), make('span', 'drawer-meta-value', display(value)));
    return item;
  }

  function evidenceBlock(values) {
    const section = make('section', 'drawer-evidence');
    section.append(make('h3', '', 'Evidence'));
    const evidence = Array.isArray(values) ? values : [values];
    evidence.filter(Boolean).forEach(value => section.append(make('code', '', value)));
    if (!evidence.filter(Boolean).length) section.append(make('p', 'empty', 'No evidence locator available.'));
    return section;
  }

  function populateDrawer() {
    if (!drawerContext) return;
    const {kind, results, index} = drawerContext;
    const item = results[index];
    const body = byId('drawer-body');
    body.replaceChildren();
    byId('drawer-kind').textContent = kind === 'tools' ? 'Tool call' : 'Timeline event';
    byId('drawer-title').textContent = display(kind === 'tools' ? item.name : item.type);
    const meta = make('div', 'drawer-meta');
    if (kind === 'tools') {
      meta.append(metaItem('Category', item.category), metaItem('Status', item.status), metaItem('Duration', Number.isFinite(item.durationMs) ? `${Number(item.durationMs).toLocaleString()} ms` : null), metaItem('Call ID', item.id), metaItem('Started', item.startedAt), metaItem('Completed', item.completedAt));
      body.append(meta, evidenceBlock(item.evidence));
      if (Object.prototype.hasOwnProperty.call(item, 'inputPreview') || Object.prototype.hasOwnProperty.call(item, 'outputPreview')) {
        body.append(detailPanel('Input', item.inputPreview, 'input'), detailPanel('Output', item.outputPreview, 'output'));
      } else body.append(make('p', 'empty', 'Redacted content previews are unavailable in metadata mode.'));
    } else {
      meta.append(metaItem('Timestamp', item.timestamp), metaItem('Event type', item.type));
      body.append(meta, evidenceBlock(item.evidence));
    }
    byId('drawer-position').textContent = `${(index + 1).toLocaleString()} of ${results.length.toLocaleString()}`;
    byId('drawer-previous').disabled = index <= 0;
    byId('drawer-next').disabled = index >= results.length - 1;
  }

  function openDrawer(kind, index, trigger) {
    const results = filtered(kind);
    if (!results[index]) return;
    restoreFocus = trigger;
    drawerContext = {kind, index, results};
    populateDrawer();
    byId('drawer-backdrop').hidden = false;
    document.body.classList.add('drawer-open');
    byId('drawer-close').focus();
  }

  function closeDrawer() {
    if (!drawerContext) return;
    byId('drawer-backdrop').hidden = true;
    document.body.classList.remove('drawer-open');
    drawerContext = null;
    if (restoreFocus && restoreFocus.isConnected) restoreFocus.focus();
    restoreFocus = null;
  }

  function moveDrawer(delta) {
    if (!drawerContext) return;
    const next = drawerContext.index + delta;
    if (next < 0 || next >= drawerContext.results.length) return;
    drawerContext.index = next;
    populateDrawer();
  }

  byId('tools-tab-count').textContent = `(${sources.tools.length.toLocaleString()})`;
  byId('timeline-tab-count').textContent = `(${sources.timeline.length.toLocaleString()})`;
  renderSummary(byId('tools-summary'), [['Total', sources.tools.length], ['Success', sources.tools.filter(item => item.status === 'success').length], ['Failure', sources.tools.filter(item => item.status === 'failure').length], ['Incomplete', sources.tools.filter(item => item.status === 'incomplete').length]]);
  const timelineCounts = orderedCounts(countBy(sources.timeline, 'type')).slice(0, 3);
  renderSummary(byId('timeline-summary'), [['Total', sources.timeline.length], ...timelineCounts]);

  ['tools', 'timeline'].forEach(kind => {
    byId(kind === 'tools' ? 'tool-search' : 'timeline-search').addEventListener('input', event => {
      closeDrawer();
      states[kind].query = event.currentTarget.value;
      states[kind].page = 1;
      render(kind);
    });
    byId(`${kind}-order`).addEventListener('click', () => {
      closeDrawer();
      states[kind].order = states[kind].order === 'asc' ? 'desc' : 'asc';
      states[kind].page = 1;
      render(kind);
    });
  });

  document.querySelectorAll('[data-page-action]').forEach(button => button.addEventListener('click', () => {
    closeDrawer();
    const kind = button.dataset.kind;
    const pageCount = Math.max(1, Math.ceil(filtered(kind).length / PAGE_SIZE));
    const action = button.dataset.pageAction;
    if (action === 'first') states[kind].page = 1;
    else if (action === 'previous') states[kind].page -= 1;
    else if (action === 'next') states[kind].page += 1;
    else states[kind].page = pageCount;
    render(kind);
  }));

  ['tools', 'timeline'].forEach(kind => byId(`explorer-tab-${kind}`).addEventListener('click', () => switchTab(kind)));
  document.querySelector('.explorer-tabs').addEventListener('keydown', event => {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
    event.preventDefault();
    if (event.key === 'Home' || event.key === 'ArrowLeft') switchTab('tools', true);
    else switchTab('timeline', true);
  });

  byId('drawer-close').addEventListener('click', closeDrawer);
  byId('drawer-previous').addEventListener('click', () => moveDrawer(-1));
  byId('drawer-next').addEventListener('click', () => moveDrawer(1));
  byId('drawer-backdrop').addEventListener('click', event => { if (event.target === byId('drawer-backdrop')) closeDrawer(); });
  document.addEventListener('keydown', event => {
    if (!drawerContext) return;
    if (event.key === 'Escape') { event.preventDefault(); closeDrawer(); return; }
    if (event.key !== 'Tab') return;
    const focusable = [...byId('record-drawer').querySelectorAll('button:not(:disabled), [href], input:not(:disabled), [tabindex]:not([tabindex="-1"])')];
    if (!focusable.length) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
  });

  render('tools');
  render('timeline');
  switchTab(activeTab);
  window.__codexExplorer = {PAGE_SIZE, states, filteredCount: kind => filtered(kind).length, switchTab};
})();
"""


def _header(kicker: str, subtitle: str) -> str:
    kicker_html = f'<div class="eyebrow">{escape(kicker)}</div>' if kicker else ""
    return f"""
    <header>
      <svg class="mark" viewBox="0 0 64 64" role="img" aria-label="Telescope"><path fill="none" stroke="currentColor" stroke-width="5" stroke-linecap="round" stroke-linejoin="round" d="M9 24l31-12 7 17-31 12zM43 14l8-3 5 12-8 3M27 37l6 10M33 47l-11 10M33 47l15 10M33 47v11"/></svg>
      <div class="header-copy">{kicker_html}<h1>Codex Observability</h1><p class="subtitle">{subtitle}</p></div>
    </header>"""


def render_session_html(
    report: dict[str, Any],
    output_dir: Path,
    *,
    show_coaching: bool = False,
) -> Path:
    output_dir = output_dir.expanduser().resolve()
    session_id = str(report["session"]["id"])
    report_dir = output_dir / "sessions"
    report_dir.mkdir(parents=True, exist_ok=True)
    suffix = "-coaching" if show_coaching else ""
    output_path = report_dir / f"{session_id}{suffix}.html"
    session = report["session"]
    tokens = report["tokenEfficiency"]
    speed = report["sessionSpeed"]
    tools = report["toolActivity"]
    context = report["contextAndReasoning"]
    skills = report["skillsAndAgents"]
    files = report["filesAndVerification"]
    reliability = report["reliability"]
    provenance = report["provenance"]
    coaching = report["coaching"]
    speed_hint = {
        "Fast": "priority service tier observed",
        "Standard": "default routing observed",
        "Mixed": "fast and standard both observed",
        "Unavailable": "no supported speed evidence",
    }.get(speed["classification"], "")
    status_label = {
        "in-progress-or-incomplete": "In progress",
        "complete": "Complete",
        "aborted": "Aborted",
    }.get(str(session["status"]), session["status"])
    coaching_html = ""
    if show_coaching:
        coaching_html = (
            '<div class="highlight-grid" id="coaching">'
            f'{_highlight_card("What you did well", coaching["positive"], "positive")}'
            f'{_highlight_card("What to improve", coaching["improvements"], "improvement")}'
            '</div><div id="recommendations" hidden aria-hidden="true"></div>'
        )

    skill_names = ", ".join(item["name"] for item in skills.get("skills", [])) or "None observed"
    subagent_names = ", ".join(
        str(item.get("nickname") or item.get("role") or item.get("threadId"))
        for item in skills.get("subagents", [])
    ) or "None observed"
    unknown_events = ", ".join(reliability["unknownEventTypes"]) or "None"
    embedded = _safe_json(report)
    html = f"""<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; img-src data:"><title>Codex Observability · {escape(session_id)}</title><style>{COMMON_CSS}</style></head><body><main class="shell">
    {_header('', f'Session <code>{escape(session_id)}</code>')}
    <p class="notice"><strong>Privacy:</strong> prompts and assistant messages are omitted. Tool details are redacted and truncated. Coaching is deterministic and based only on observable operational evidence.</p>
    {coaching_html}
    <div class="bento">
      <section id="session-overview" class="bento-card span-8">{_category_heading('sessions','Session overview','What ran')}<div class="metrics">{_metric('Status',status_label)}{_metric('Model',session['model'])}{_metric('Reasoning',session['reasoningEffort'])}{_metric('Speed',speed['classification'],speed_hint)}{_metric('Turns',session['turnCount'])}{_metric('Observed duration',_human_duration(session['observedDurationMs']))}{_metric('Workspace',session['workspace'])}</div></section>
      <section id="reliability" class="bento-card span-4">{_category_heading('coverage','Reliability','Coverage')}<div class="metrics">{_metric('Coverage',reliability['coverage'])}{_metric('Malformed',reliability['malformedLines'])}{_metric('Orphaned',reliability['orphanedToolCalls'])}{_metric('Token mismatch',reliability['tokenMismatch'])}</div></section>
      <section id="token-efficiency" class="bento-card span-12">{_category_heading('tokens','Token efficiency','Exact usage')}<div class="metrics">{_metric('Total tokens',tokens['totalTokens'],'authoritative state total')}{_metric('Input',tokens['inputTokens'])}{_metric('Cached input',tokens['cachedInputTokens'])}{_metric('Uncached input',tokens['uncachedInputTokens'])}{_metric('Output',tokens['outputTokens'])}{_metric('Reasoning output',tokens['reasoningOutputTokens'])}{_metric('Cache ratio',None if tokens['cacheRatio'] is None else f"{tokens['cacheRatio']}%")} {_metric('Reconciliation',tokens['reconciliation'])}</div></section>
      <section id="tool-activity" class="bento-card span-7">{_category_heading('tools','Tool activity','Operational health')}<div class="metrics">{_metric('Calls',tools['totalCalls'])}{_metric('Succeeded',tools['successes'])}{_metric('Failed',tools['failures'])}{_metric('Incomplete',tools['incomplete'])}</div><h3 class="section-gap">Calls by bucket</h3><table>{_counter_rows(tools['byCategory'])}</table></section>
      <section id="skills-agents" class="bento-card span-5">{_category_heading('agents','Skills and agents','Coordination')}<table>{_rows([('Skill observation',skills['skillObservation']),('Loaded instructions inferred',skill_names),('Subagents observed',len(skills['subagents'])),('Subagent identifiers',subagent_names)])}</table></section>
      <section id="files-verification" class="bento-card span-4">{_category_heading('verification','Files and verification','Completion evidence')}<div class="metrics">{_metric('Changes observed',files['fileChangesObserved'])}{_metric('Verification observed',files['verificationObserved'])}</div></section>
      <section id="context-reasoning" class="bento-card span-8">{_category_heading('context','Context and reasoning','Session shape')}<div class="metrics">{_metric('Compactions',context['compactions'])}{_metric('Turn contexts',len(context['turnContexts']))}{_metric('Model changed',context.get('modelChanged'))}{_metric('Effort changed',context.get('effortChanged'))}{_metric('Speed changes',speed['changes'])}</div><p class="muted">Reasoning usage metadata is shown; private reasoning content is not.</p></section>
      {ADVANCED_EXPLORER_HTML}
      <details id="provenance" class="bento-card span-12 advanced">{_advanced_summary('provenance','Provenance and coverage details')}<p><strong>Unknown event types:</strong> {escape(unknown_events)}</p><table>{_rows([('Codex CLI',provenance['codexCliVersion']),('Source tag',provenance['sourceTag']),('Source commit',provenance['sourceCommit']),('Adapter',provenance['adapterVersion']),('State database',redact_text(provenance['stateDatabase'])),('Rollout',redact_text(provenance['rollout'])),('Coaching policy',coaching['policyVersion'])])}</table></details>
    </div><footer>Generated locally by Codex Observability. No network resources are loaded.</footer></main>
    <script id="report-data" type="application/json">{embedded}</script><script>{EXPLORER_SCRIPT}</script></body></html>"""
    output_path.write_text(html, encoding="utf-8")
    return output_path


def render_index_html(
    dashboard: dict[str, Any],
    output_dir: Path,
    *,
    show_coaching: bool = False,
) -> Path:
    output_dir = output_dir.expanduser().resolve()
    output_dir.mkdir(parents=True, exist_ok=True)
    output_path = output_dir / ("coaching.html" if show_coaching else "index.html")
    population = dashboard["population"]
    aggregates = dashboard["aggregates"]
    reliability = dashboard["reliability"]
    coaching = dashboard["coaching"]
    coaching_html = ""
    if show_coaching:
        coaching_html = (
            '<div class="highlight-grid" id="coaching">'
            f'{_highlight_card("What you did well", coaching["positive"], "positive")}'
            f'{_highlight_card("What to improve", coaching["improvements"], "improvement")}'
            '</div>'
        )
    sessions = dashboard["sessions"]
    session_rows: list[str] = []
    for session in sessions:
        detail = output_dir / "sessions" / f"{session['id']}.html"
        cell = (
            f'<a href="sessions/{escape(session["id"])}.html"><code>{escape(session["id"])}</code></a>'
            if detail.is_file() else f'<code>{escape(session["id"])}</code>'
        )
        search = " ".join(str(session.get(key) or "") for key in ("id","workspace","model","reasoningEffort","taskSignature")).lower()
        session_rows.append(
            f'<tr class="session-row" data-search="{escape(search)}" data-workspace="{escape(session["workspace"])}"><td>{cell}</td><td>{escape(session["workspace"])}</td><td>{escape(_display(session["model"]))}</td><td>{escape(_display(session["taskSignature"]))}</td><td>{session["totalTokens"]:,}</td><td>{_human_time(session["updatedAt"])}</td></tr>'
        )
    if not session_rows:
        session_rows.append('<tr><td colspan="6" class="empty">No top-level sessions were found.</td></tr>')
    workspace_options = ''.join(
        f'<option value="{escape(workspace)}">{escape(workspace)}</option>'
        for workspace in aggregates["workspaces"]
    )
    changed = aggregates["changedSessions"]
    verification_rate = round(aggregates["verifiedChangeSessions"] / changed * 100, 1) if changed else None
    token_input = aggregates["tokens"]["input"]
    cache_ratio = round(aggregates["tokens"]["cachedInput"] / token_input * 100, 1) if token_input else None
    embedded = _safe_json(dashboard)
    html = f"""<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; img-src data:"><title>Codex Observability · Historical dashboard</title><style>{COMMON_CSS}</style></head><body><main class="shell">
    {_header('Local · all-history · source-verified', f'{population["topLevel"]:,} top-level tasks analyzed across retained Codex data')}
    <p class="notice"><strong>Privacy:</strong> this dashboard contains sanitized derived metrics only. It does not inspect or retain prompt, response, title, command, or complete path content.</p>
    {coaching_html}
    <div class="bento">
      <section class="bento-card span-8">{_category_heading('sessions','Session population','Every retained thread, correctly separated')}<div class="metrics">{_metric('All threads',population['total'])}{_metric('Top-level tasks',population['topLevel'])}{_metric('Spawned agents',population['spawned'])}{_metric('Internal review',population['internal'])}{_metric('Archived',population['archived'])}{_metric('Analyzed',population['analyzed'])}</div></section>
      <section class="bento-card span-4">{_category_heading('coverage','Analysis coverage','Data completeness')}<div class="metrics">{_metric('Complete evidence',reliability['coachingEligible'])}{_metric('Partial',reliability['partial'])}{_metric('Unavailable',reliability['unavailable'])}</div></section>
      <section class="bento-card span-4">{_category_heading('tokens','Token efficiency','Top-level usage')}<div class="metrics">{_metric('Total tokens',aggregates['tokens']['topLevel'])}{_metric('Cached input ratio',None if cache_ratio is None else f"{cache_ratio}%")}{_metric('Output tokens',aggregates['tokens']['output'])}{_metric('Reasoning output',aggregates['tokens']['reasoningOutput'])}</div></section>
      <section class="bento-card span-4">{_category_heading('tools','Tool health','Execution quality')}<div class="metrics">{_metric('Tool calls',aggregates['toolCalls'])}{_metric('Failures',aggregates['toolFailures'])}{_metric('Failure rate',None if not aggregates['toolCalls'] else f"{round(aggregates['toolFailures']/aggregates['toolCalls']*100,1)}%")}</div></section>
      <section class="bento-card span-4">{_category_heading('delegation','Delegation and verification','Workflow discipline')}<div class="metrics">{_metric('Spawned children',aggregates['spawnedChildren'])}{_metric('Change sessions',changed)}{_metric('Verification rate',None if verification_rate is None else f"{verification_rate}%")}</div></section>
      <section class="bento-card span-6">{_category_heading('models','Models','Model mix')}<table>{_counter_rows(aggregates['models'])}</table></section>
      <section class="bento-card span-6">{_category_heading('reasoning','Reasoning','Effort mix')}<table>{_counter_rows(aggregates['reasoningEffort'])}</table></section>
      <section class="bento-card span-12">{_category_heading('recent','Recent sessions','Explore top-level tasks')}<div class="filters"><input id="session-search" type="search" placeholder="Filter by session, workspace, model, or task signature" aria-label="Filter sessions"><select id="workspace-filter" aria-label="Filter by workspace"><option value="">All workspaces</option>{workspace_options}</select></div><div class="table-wrap"><table><thead><tr><th>Session</th><th>Workspace</th><th>Model</th><th>Signature</th><th>Total tokens</th><th>Updated</th></tr></thead><tbody>{''.join(session_rows)}</tbody></table></div></section>
      <details class="bento-card span-12 advanced">{_advanced_summary('provenance','Advanced tool, cache, and provenance details')}<div class="bento"><div class="span-6"><h2>Tools by bucket</h2><table>{_counter_rows(aggregates['tools'])}</table></div><div class="span-6"><h2>Cache diagnostics</h2><table>{_rows([('Path',dashboard['cache']['path']),('Cache hits',dashboard['cache']['hits']),('Cache misses',dashboard['cache']['misses']),('Purged',dashboard['cache']['purged']),('Forced rebuild',dashboard['cache']['rebuild']),('State threads at refresh',dashboard['cache']['stateThreadsAtRefresh']),('State tokens at refresh',dashboard['cache']['stateTotalTokensAtRefresh']),('Refreshed',dashboard['cache']['refreshedAt']),('Adapter',dashboard['provenance']['adapterVersion']),('Policy',coaching['policyVersion'])])}</table></div></div></details>
    </div><footer>Generated locally by Codex Observability. No network resources are loaded.</footer></main>
    <script id="index-data" type="application/json">{embedded}</script><script>
    const search=document.getElementById('session-search');const workspace=document.getElementById('workspace-filter');const rows=[...document.querySelectorAll('.session-row')];function filterRows(){{const q=search.value.trim().toLowerCase();const w=workspace.value;rows.forEach(row=>{{row.hidden=Boolean((q&&!row.dataset.search.includes(q))||(w&&row.dataset.workspace!==w));}});}}search.addEventListener('input',filterRows);workspace.addEventListener('change',filterRows);
    </script></body></html>"""
    output_path.write_text(html, encoding="utf-8")
    return output_path
