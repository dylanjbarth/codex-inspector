from __future__ import annotations

import argparse
from functools import partial
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import sys
import time

from .compat import CompatibilityError, verify_compatibility
from .coaching import build_session_coaching, load_policy
from .data import build_session_report
from .history import CACHE_FILENAME, refresh_history, session_summary
from .render import render_index_html, render_session_html


PLUGIN_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_CONTRACT = PLUGIN_ROOT / "contracts" / "codex-0.143.0.json"
DEFAULT_POLICY = PLUGIN_ROOT / "contracts" / "coaching-policy-0.2.0.json"


def _common_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("--source-root", type=Path, default=Path("/codex"))
    parser.add_argument(
        "--codex-home",
        type=Path,
        default=Path(os.environ.get("CODEX_HOME", Path.home() / ".codex")),
    )
    parser.add_argument(
        "--output-dir",
        type=Path,
        default=Path.home() / ".codex-observability" / "reports",
    )
    parser.add_argument(
        "--cache-dir",
        type=Path,
        default=Path.home() / ".codex-observability" / "cache",
    )
    parser.add_argument("--policy", type=Path)
    parser.add_argument("--contract", type=Path, default=DEFAULT_CONTRACT)
    parser.add_argument("--format", choices=("html", "json"), default="html")
    parser.add_argument("--content", choices=("metadata", "redacted"), default="redacted")
    return parser


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="codex-observability",
        description="Generate source-verified local Codex session reports.",
    )
    subparsers = parser.add_subparsers(dest="command", required=True)
    common = _common_parser()
    subparsers.add_parser("doctor", parents=[common], help="verify source and data compatibility")
    report = subparsers.add_parser("report", parents=[common], help="generate one session report")
    report.add_argument("--session", default="latest", help="latest or a session UUID")
    report.add_argument(
        "--coaching",
        action="store_true",
        help="include rating and improvement coaching in a separate coaching report",
    )
    index = subparsers.add_parser("index", parents=[common], help="generate a recent-session index")
    index.add_argument("--limit", type=int, default=50)
    index.add_argument("--rebuild-cache", action="store_true")
    index.add_argument(
        "--coaching",
        action="store_true",
        help="include historical improvement coaching in a separate coaching dashboard",
    )
    clean = subparsers.add_parser("clean", parents=[common], help="remove old generated HTML reports")
    clean_group = clean.add_mutually_exclusive_group(required=True)
    clean_group.add_argument("--older-than", type=int, metavar="DAYS")
    clean_group.add_argument("--cache", action="store_true", help="remove the derived history cache")
    serve = subparsers.add_parser("serve", parents=[common], help="serve reports on loopback only")
    serve.add_argument("--port", type=int, default=0)
    return parser


def _resolve_policy(args: argparse.Namespace) -> dict[str, object]:
    if args.policy:
        return load_policy(args.policy.expanduser().resolve(), source_kind="user-override")
    user_policy = Path.home() / ".codex-observability" / "coaching-policy.json"
    if user_policy.is_file():
        return load_policy(user_policy, source_kind="user-override")
    return load_policy(DEFAULT_POLICY, source_kind="bundled")


def _progress(current: int, total: int) -> None:
    print(f"Codex Observability refreshed {current} of {total} session summaries", file=sys.stderr)


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        if args.command == "clean" and args.cache:
            root = args.cache_dir.expanduser().resolve()
            removed = 0
            for name in (CACHE_FILENAME, f"{CACHE_FILENAME}-shm", f"{CACHE_FILENAME}-wal"):
                path = root / name
                if path.is_file():
                    path.unlink()
                    removed += 1
            payload = {"removed": removed, "cache": str(root / CACHE_FILENAME)}
            if args.format == "json":
                print(json.dumps(payload, indent=2, sort_keys=True))
            else:
                print(f"Removed {removed} generated cache file(s).")
            return 0
        compatibility = verify_compatibility(
            source_root=args.source_root.expanduser().resolve(),
            codex_home=args.codex_home.expanduser().resolve(),
            contract_path=args.contract.expanduser().resolve(),
        )
        policy = _resolve_policy(args)
        if args.command == "doctor":
            payload = compatibility.as_dict()
            if args.format == "json":
                print(json.dumps(payload, indent=2, sort_keys=True))
            else:
                print("Codex Observability doctor: ok")
                for key, value in payload.items():
                    if key != "status":
                        print(f"{key}: {value}")
            return 0
        if args.command == "report":
            dashboard = refresh_history(
                compatibility=compatibility,
                codex_home=args.codex_home.expanduser().resolve(),
                cache_dir=args.cache_dir,
                policy=policy,
                limit=50,
                progress=_progress,
            )
            report = build_session_report(
                compatibility=compatibility,
                codex_home=args.codex_home.expanduser().resolve(),
                session=args.session,
                content=args.content,
            )
            summary = session_summary(dashboard, report["session"]["id"])
            if args.coaching and summary:
                coaching = build_session_coaching(summary, policy)
                coaching["enabled"] = True
                report["coaching"] = coaching
                report["recommendations"] = coaching["improvements"]
            elif not args.coaching:
                report["coaching"] = {
                    "enabled": False,
                    "positive": [],
                    "improvements": [],
                    "policyVersion": policy["policy_version"],
                }
                report["recommendations"] = []
            if args.format == "json":
                print(json.dumps(report, indent=2, sort_keys=True))
                return 0
            output_path = render_session_html(
                report,
                args.output_dir,
                show_coaching=args.coaching,
            )
            print(output_path)
            return 0
        if args.command == "index":
            payload = refresh_history(
                compatibility=compatibility,
                codex_home=args.codex_home.expanduser().resolve(),
                cache_dir=args.cache_dir,
                policy=policy,
                limit=args.limit,
                rebuild=args.rebuild_cache,
                progress=_progress,
            )
            if args.coaching:
                payload["coaching"]["enabled"] = True
            else:
                payload["coaching"] = {
                    "enabled": False,
                    "positive": [],
                    "improvements": [],
                    "policyVersion": policy["policy_version"],
                }
            if args.format == "json":
                print(json.dumps(payload, indent=2, sort_keys=True))
                return 0
            output_path = render_index_html(
                payload,
                args.output_dir,
                show_coaching=args.coaching,
            )
            print(output_path)
            return 0
        if args.command == "clean":
            if args.older_than is None or args.older_than < 0:
                raise CompatibilityError("--older-than must be zero or greater")
            root = args.output_dir.expanduser().resolve()
            cutoff = time.time() - args.older_than * 86400
            removed: list[str] = []
            if root.is_dir():
                for path in root.rglob("*.html"):
                    if path.is_file() and path.stat().st_mtime < cutoff:
                        path.unlink()
                        removed.append(str(path))
            payload = {"removed": len(removed), "files": removed}
            if args.format == "json":
                print(json.dumps(payload, indent=2, sort_keys=True))
            else:
                print(f"Removed {len(removed)} generated HTML report(s).")
            return 0
        if args.command == "serve":
            root = args.output_dir.expanduser().resolve()
            if not root.is_dir():
                raise CompatibilityError(f"report directory does not exist: {root}")
            if not 0 <= args.port <= 65535:
                raise CompatibilityError("--port must be between 0 and 65535")

            class ReportHandler(SimpleHTTPRequestHandler):
                def end_headers(self) -> None:
                    self.send_header("Cache-Control", "no-store")
                    self.send_header("X-Content-Type-Options", "nosniff")
                    super().end_headers()

                def log_message(self, format: str, *values: object) -> None:
                    return

                def list_directory(self, path: str):  # type: ignore[no-untyped-def]
                    self.send_error(404, "Directory listing disabled")
                    return None

            handler = partial(ReportHandler, directory=str(root))
            server = ThreadingHTTPServer(("127.0.0.1", args.port), handler)
            host, port = server.server_address[:2]
            print(f"http://{host}:{port}/", flush=True)
            try:
                server.serve_forever()
            except KeyboardInterrupt:
                pass
            finally:
                server.server_close()
            return 0
    except CompatibilityError as exc:
        if getattr(args, "format", "html") == "json":
            print(json.dumps({"status": "error", "error": str(exc)}, sort_keys=True))
        else:
            print(f"Codex Observability: {exc}", file=sys.stderr)
        return 2
    return 0
