package step_based_workflow

import (
	"fmt"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/contractupgrade"
)

// authorizeContractVersionStamp decides whether sessionID may stamp version
// right now, spending the authorization when it may. The second return value
// reports permission; the first is the refusal to hand back when it does not.
//
// This bound lives in the executor rather than in the upgrade prompt because
// the stamp does not only arrive as a native tool call. On confida-login
// 2026-08-12 it arrived as
//
//	curl -X POST -d '{"version":"1.0.21"}' -H "$MCP_AUTH" "$MCP_CUSTOM/set_workflow_contract_version"
//
// run through execute_shell_command from a Pulse turn, ten minutes after the
// upgrade turn that owed that stamp had been adjudicated and closed. Both paths
// land here, so both are covered; no wording in a prompt could have covered the
// shell one.
func authorizeContractVersionStamp(sessionID, version string) (string, bool) {
	if contractupgrade.Consume(sessionID, version) {
		return "", true
	}
	if granted := contractupgrade.Granted(sessionID); granted != "" {
		return fmt.Sprintf(
			"Refused: this turn is authorized to stamp %s, not %s. Stamp the version its own upgrade instruction named.",
			granted, version,
		), false
	}
	return "Refused: no contract upgrade turn is open for this session, so there is no version to stamp. " +
		"This tool is accepted only inside the scheduler's upgrade turn, for the version that turn asked for. " +
		"If an earlier upgrade turn reported a blocker, resolve the blocker and let the next preflight re-run that turn — " +
		"stamping from a later turn does not complete the migration, it makes the scheduler skip it.", false
}
