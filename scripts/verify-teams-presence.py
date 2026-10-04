#!/usr/bin/env python3
"""Check whether Microsoft Teams (new Teams on macOS) reported you Away.

Reads the Teams logs for a time window and lists presence changes, so you can
verify that `keepalive --active` kept you Available while you were away.

Usage: scripts/verify-teams-presence.py START [END]
  START/END are local times, YYYY-MM-DDTHH:MM[:SS]; END defaults to now.
  Example: scripts/verify-teams-presence.py 2026-10-04T12:27 2026-10-04T12:50

Exit status: 0 if Teams never reported Away while the screen was unlocked,
1 if it did, 2 on usage errors or when no logs cover the window.

Notes:
- Teams writes its log timestamps in UTC but labels them with the local
  offset. The real offset is derived from each log file's name (which is in
  local time) and its first timestamp.
- "client" lines are Teams' own window-interaction state (Active, Inactive,
  LongInactive). They change when you stop using the Teams window itself and
  do not decide presence, so they are shown for context only.
"""

import datetime as dt
import pathlib
import re
import sys

LOGDIR = pathlib.Path.home() / (
    "Library/Group Containers/UBF8T346G9.com.microsoft.teams/"
    "Library/Application Support/Logs"
)
NAME_RE = re.compile(r"MSTeams_(\d{4}-\d{2}-\d{2})_(\d{2})-(\d{2})-(\d{2})")
STAMP_RE = re.compile(r"^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})")
PRESENCE_RE = re.compile(r"OnAvailabilityUpdate Received availability update: (\w+)")
CLIENT_RE = re.compile(r"WebClientStatesModule: operator\(\): current=(\w+), new_state=(\w+)")
LOCK_RE = re.compile(r"System locked state changed: (locked|unlocked)")


def parse_local(value: str) -> dt.datetime:
    for fmt in ("%Y-%m-%dT%H:%M:%S", "%Y-%m-%dT%H:%M"):
        try:
            return dt.datetime.strptime(value, fmt)
        except ValueError:
            pass
    raise SystemExit(f"invalid time {value!r} (use YYYY-MM-DDTHH:MM[:SS])")


def log_offset(path: pathlib.Path) -> dt.timedelta | None:
    """Local time minus the log's timestamp clock, rounded to 15 minutes."""
    m = NAME_RE.search(path.name)
    if not m:
        return None
    named = dt.datetime.strptime(" ".join(m.groups()), "%Y-%m-%d %H %M %S")
    with path.open("rb") as fh:
        for raw in fh:
            s = STAMP_RE.match(raw.decode("utf-8", "replace"))
            if s:
                first = dt.datetime.strptime(s.group(1), "%Y-%m-%dT%H:%M:%S")
                minutes = round((named - first).total_seconds() / 900) * 15
                return dt.timedelta(minutes=minutes)
    return None


def events(start: dt.datetime, end: dt.datetime):
    for path in sorted(LOGDIR.glob("MSTeams_*.log")):
        offset = log_offset(path)
        if offset is None:
            continue
        with path.open("rb") as fh:
            for raw in fh:
                line = raw.decode("utf-8", "replace")
                s = STAMP_RE.match(line)
                if not s:
                    continue
                when = dt.datetime.strptime(s.group(1), "%Y-%m-%dT%H:%M:%S") + offset
                if not start <= when <= end:
                    continue
                if m := PRESENCE_RE.search(line):
                    yield when, "presence", m.group(1)
                elif m := CLIENT_RE.search(line):
                    yield when, "client", f"{m.group(1)} -> {m.group(2)}"
                elif m := LOCK_RE.search(line):
                    yield when, "lock", m.group(1)


def last_presence_before(start: dt.datetime) -> str | None:
    last = None
    for when, kind, value in events(dt.datetime.min, start):
        if kind == "presence":
            last = value
    return last


def main(argv: list[str]) -> int:
    if len(argv) not in (2, 3):
        print(__doc__.split("Notes:")[0].strip(), file=sys.stderr)
        return 2
    if not LOGDIR.is_dir():
        print(f"no Teams logs at {LOGDIR}", file=sys.stderr)
        return 2
    start = parse_local(argv[1])
    end = parse_local(argv[2]) if len(argv) == 3 else dt.datetime.now().replace(microsecond=0)

    print(f"Teams presence between {start:%Y-%m-%d %H:%M:%S} and {end:%H:%M:%S} (local time)")
    before = last_presence_before(start)
    print(f"  presence at start: {before or 'unknown'}")
    print()

    locked = False
    problems = 0
    seen = set()
    for when, kind, value in events(start, end):
        key = (when, kind, value)
        if key in seen:
            continue
        seen.add(key)
        mark = " "
        if kind == "lock":
            locked = value == "locked"
            text = f"screen {value}"
        elif kind == "presence":
            text = f"presence: {value}"
            if value.startswith("Away") and not locked:
                mark = "!"
                problems += 1
        else:
            text = f"client:   {value}  (Teams window use; not presence)"
        print(f"{mark} {when:%H:%M:%S}  {text}")
    if not seen:
        print("  (no presence changes in this window)")

    print()
    if problems:
        print(f"FAIL: Teams reported Away {problems} time(s) while the screen was unlocked (marked !)")
        return 1
    if before is None and not any(k == "presence" for _, k, _ in seen):
        print("UNKNOWN: no presence information before or inside the window")
        return 2
    print("OK: Teams never reported Away while the screen was unlocked")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
