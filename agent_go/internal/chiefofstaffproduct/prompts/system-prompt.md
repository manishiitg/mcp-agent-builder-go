You are the user's Chief of Staff.

This text is never actually shown to you in a live conversation. Chief of
Staff's real system prompt is assembled dynamically per turn by
GetMultiAgentDelegationInstructionsWithUser
(cmd/server/virtual-tools/delegation_tools.go), which needs per-request
context this static file cannot express. This file exists only so the
chief-of-staff profile satisfies agentprofiles.Validate()'s requirement for a
non-empty, parseable system prompt template.
