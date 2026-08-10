#!/usr/bin/env python3
"""Close platform findings whose owning fix has shipped (PLAT-072).

`external_action_required` is a state with no exit. Nothing closes these when a
platform ticket ships — `resolve_run_concern` is limited by contract to
acknowledgment or rejection, and Pulse is correctly read-only on platform
matters. Re-observation cannot correct them either: the concern upsert's reopen
clause covers `resolved`, `awaiting_verification` and `awaiting_run`, and
deliberately omits `external_action_required` so an unfixable issue stops
resurfacing every run.

The result is a board that only grows, where a fixed problem is indistinguishable
from a live one. On 2026-08-10 six findings describing a cost-attribution defect
fixed four days earlier (commit 0f6519640) were convincing enough to get a fresh
P1 filed for the same work.

Closing to `resolved` is deliberately the safe direction: `resolved` IS in the
reopen clause, so if the fix did not actually hold, the next observation flips
the finding back to `open` on its own. Closing wrongly is self-correcting;
leaving it stuck is not.

This tool does NOT decide what is stale. A theme/regex mapping was tried and
matched 11 of 81 while misattributing several, so the judgement stays with the
caller: name the ticket, the evidence, and the exact fingerprints. The tool's
job is to make that closure safe, uniform, and auditable.

This tool is for exactly one case: "a platform fix shipped, so this finding is
now stale." It cannot and must not be used for "this was never a platform
defect" (the outside world correctly blocking a request, a folder guard
correctly denying an unsanctioned access path, etc.) — that closure needs
`status='rejected'`, which is outside this tool's scope by design, since
`rejected` is NOT in the concern upsert's reopen clause and a wrong rejection
would stay wrongly closed forever. Use the `resolve_run_concern` tool
(`status="rejected"`, with a note) from within a live Pulse/workshop session
for that case instead (PLAT-073 cluster H).

Usage:
  # see what is on the board (the version column is the revision a finding was
  # FIRST observed against; "?" means unknown, which is not the same as old)
  python3 scripts/pulse_close_stale.py --list
  python3 scripts/pulse_close_stale.py --list --workflow social-media

  # dry run (default) then apply
  python3 scripts/pulse_close_stale.py --close --ticket PLAT-072 \
      --evidence "fixed by 0f6519640 (2026-08-06)" --fingerprint abc123 --fingerprint def456
  python3 scripts/pulse_close_stale.py --close ... --apply
"""

import argparse
import datetime
import os
import sqlite3
import sys

EXTERNAL = "external_action_required"
RESOLVED = "resolved"
RESOLVED_BY = "platform-triage-sweep"


def workflow_dbs(root):
    for name in sorted(os.listdir(root)):
        if name.startswith("_"):
            continue
        db = os.path.join(root, name, "db", "db.sqlite")
        if os.path.isfile(db):
            yield name, db


def list_findings(root, workflow_filter):
    total = 0
    for wf, db in workflow_dbs(root):
        if workflow_filter and wf != workflow_filter:
            continue
        try:
            conn = sqlite3.connect(db)
        except sqlite3.Error:
            continue
        try:
            rows = list(conn.execute(
                "select fingerprint, first_seen_at, seen_count, "
                "coalesce(first_seen_platform_version,''), text "
                "from run_concerns where status=? order by first_seen_at",
                (EXTERNAL,)))
        except sqlite3.OperationalError:
            # first_seen_platform_version arrives with the PLAT-072 migration,
            # which only runs when the server next opens this database. Fall back
            # rather than skipping the workflow: silently reporting zero findings
            # for an un-migrated database would hide the very board this tool
            # exists to show.
            try:
                rows = [(fp, first, seen, "", text) for fp, first, seen, text in conn.execute(
                    "select fingerprint, first_seen_at, seen_count, text "
                    "from run_concerns where status=? order by first_seen_at",
                    (EXTERNAL,))]
            except sqlite3.Error:
                continue
        except sqlite3.Error:
            continue
        if not rows:
            continue
        print(f"\n=== {wf} ({len(rows)}) ===")
        for fp, first, seen, version, text in rows:
            total += 1
            oneline = " ".join((text or "").split())
            # Findings recorded before PLAT-072 carry no revision. Shown as "?"
            # rather than blank so it reads as unknown, never as old — an unknown
            # revision must always fall back to reading the finding.
            ver = version or "?"
            print(f"  {fp}  {(first or '')[:10]}  {ver:<14} seen={seen:<3} {oneline[:96]}")
    print(f"\ntotal external_action_required: {total}")


def close_findings(root, ticket, evidence, fingerprints, apply_changes):
    wanted = set(fingerprints)
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    note = (f"Closed by platform triage sweep {stamp[:10]}: covered by {ticket} ({evidence}). "
            f"Reopens automatically if observed again.")
    found, closed = set(), 0
    for wf, db in workflow_dbs(root):
        try:
            conn = sqlite3.connect(db)
            rows = list(conn.execute(
                "select fingerprint, text from run_concerns where status=?", (EXTERNAL,)))
        except sqlite3.Error:
            continue
        for fp, text in rows:
            if fp not in wanted:
                continue
            found.add(fp)
            oneline = " ".join((text or "").split())[:90]
            print(f"  {'CLOSE' if apply_changes else 'would close'}  {wf:<22} {fp}  {oneline}")
            if apply_changes:
                conn.execute(
                    "update run_concerns set status=?, resolved_at=?, resolved_by=?, resolution_note=? "
                    "where fingerprint=? and status=?",
                    (RESOLVED, stamp, RESOLVED_BY, note, fp, EXTERNAL))
                conn.commit()
                closed += 1

    missing = wanted - found
    if missing:
        print(f"\n!! {len(missing)} fingerprint(s) not found in any external_action_required row:")
        for fp in sorted(missing):
            print(f"   {fp}")
        print("   (already closed, wrong id, or a different status — nothing was written for these)")

    print(f"\nticket:   {ticket}")
    print(f"evidence: {evidence}")
    print(f"note:     {note}")
    if apply_changes:
        print(f"\nclosed {closed} finding(s).")
    else:
        print(f"\nDRY RUN — {len(found)} finding(s) matched. Re-run with --apply to write.")
    return 0 if not missing else 1


def main():
    default_root = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
                                "workspace-docs", "Workflow")
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--workflow-root", default=default_root)
    p.add_argument("--list", action="store_true", help="list findings on the external-action board")
    p.add_argument("--workflow", help="restrict --list to one workflow")
    p.add_argument("--close", action="store_true", help="close the named fingerprints")
    p.add_argument("--ticket", help="owning PLAT ticket, e.g. PLAT-072")
    p.add_argument("--evidence", help="what fixed it, e.g. 'fixed by 0f6519640 (2026-08-06)'")
    p.add_argument("--fingerprint", action="append", default=[], help="repeatable")
    p.add_argument("--apply", action="store_true", help="write; omit for a dry run")
    args = p.parse_args()

    if not os.path.isdir(args.workflow_root):
        print(f"workflow root not found: {args.workflow_root}", file=sys.stderr)
        return 2
    if args.list:
        list_findings(args.workflow_root, args.workflow)
        return 0
    if args.close:
        # Evidence is mandatory. A closure with no stated cause is exactly the
        # unfalsifiable record this tool exists to stop producing.
        if not (args.ticket and args.evidence and args.fingerprint):
            print("--close requires --ticket, --evidence and at least one --fingerprint", file=sys.stderr)
            return 2
        return close_findings(args.workflow_root, args.ticket, args.evidence,
                              args.fingerprint, args.apply)
    p.print_help()
    return 0


if __name__ == "__main__":
    sys.exit(main())
