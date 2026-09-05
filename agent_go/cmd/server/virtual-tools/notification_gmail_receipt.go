package virtualtools

import (
	"github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/services"
	"strings"
)

// A requested but unavailable channel must not disappear behind Slack success.
func explainMissingGmail(results []services.ConnectorResult, expected bool, excluded []string, gmail *services.GmailService) []services.ConnectorResult {
	if !expected {
		return results
	}
	for _, channel := range excluded {
		if strings.EqualFold(strings.TrimSpace(channel), "gmail") {
			return results
		}
	}
	for i, result := range results {
		if result.Channel != "gmail" {
			continue
		}
		if result.OK && result.MsgID == "" {
			results[i].OK = false
			results[i].Err = "Gmail was requested but no eligible recipient was resolved (missing destination or all recipients blocked). No email was sent."
		}
		return results
	}
	reason := "Gmail delivery service is unavailable on this server; no email was attempted."
	if gmail != nil {
		switch {
		case !gmail.GetConfig().Enabled:
			reason = "Gmail notifications are switched off in account settings; no email was attempted. This does not mean the account is signed out."
		case !gmail.IsEnabled():
			reason = "Gmail notifications are configured, but the server cannot enable the sending service. Check the gws executable and server configuration; no email was attempted."
		default:
			reason = "Gmail is enabled but was not registered for notification delivery; no email was attempted. This is a server configuration issue."
		}
	}
	return append(results, services.ConnectorResult{Channel: "gmail", OK: false, Err: reason})
}
