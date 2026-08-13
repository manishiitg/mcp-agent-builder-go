[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-047 — grouped execution artifacts lack an immutable physical run identity

| Coordination | Value |
|---|---|
| Assigned agent | `Unassigned` |
| Ticket state | `design_required` |
| Last synchronized | `2026-08-07` |

- **Priority:** P1
- **Owner:** run-folder retention and execution identity
- **Source workflow:** Upwork

## Problem

Grouped work can reuse a logical folder such as `iteration-0` across producing
runs. SQLite lifecycle rows and cost records can retain durable execution IDs,
but physical review evidence under the reused folder may be overwritten or
become ambiguous. This is distinct from PLAT-031: that ticket fixed cost-ledger
identity across archive rotation; it did not make every run artifact immutable.

## Required design

Choose one immutable physical artifact identity per producing execution, keep a
small latest pointer for compatibility, and link findings/reviews to that
identity. Migration and retention limits must be defined before changing folder
layout. No implementation is claimed by the 2026-08-07 reviewer fixes.
