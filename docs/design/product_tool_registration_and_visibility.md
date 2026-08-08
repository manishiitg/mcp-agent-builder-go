# Product Tool Registration and Agent Visibility

## Problem

Product tools currently pass through two independent filters:

1. **Registration policy** decides whether a server creates a tool for a product session.
2. **Agent-visible policy** decides whether the tool is exposed to a coding agent and included in `get_api_spec`.

This separation supports real needs: a product may register a tool for its HTTP UI while withholding it from a particular agent/provider/session. However, maintaining the lists independently creates a failure mode: a tool can be correctly registered but invisible to the agent that must use it.

Video Studio exposed this with secret management. `set_user_secret` and `set_workflow_secret` were registered in the session, but the agent-visible list omitted them. The coding agent could not call either tool or discover it through `get_api_spec`; it then attempted an obsolete shell bridge path, which Video Studio intentionally disables.

## Invariant

For every product session:

> A tool registered for an agent-facing capability must be visible to that agent and represented in its API spec, unless a deliberate, documented runtime override removes it.

The inverse is also required: a tool that is not registered must never appear in the API spec.

## Design

Treat product capabilities as the source of truth. Derive the normal agent-visible set from the tools actually registered by those capabilities. Keep a small, explicit runtime override only for intentional exclusions such as:

- provider incompatibility;
- a temporary restricted session;
- UI-only tools; or
- an approval/security mode that removes an agent capability.

The API spec must be generated from this final effective set, not from a separate hard-coded allow-list.

## Secret-management requirements

When secrets are enabled for a product, the effective agent tools must include the appropriate scope operations:

- `list_secrets`
- `set_user_secret` and/or `set_workflow_secret`
- `delete_user_secret` and/or `delete_workflow_secret`

The system prompt should instruct the agent to use these tools directly. It must never pass secret values through shell commands, URLs, or ordinary logs.

## Tests

Add a contract test for each product profile that verifies:

1. expected product capabilities register their tools;
2. every agent-facing registered tool appears in the effective agent tool set;
3. `get_api_spec` exposes the same effective set; and
4. explicitly disabled tools appear in neither location.

This makes the Video Studio secret-tool mismatch impossible to reintroduce silently.
