package virtualtools

import (
	"fmt"
	"github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/services"
	"strings"
)

// Preserve provider errors and make missing expected delivery explicit. Workflow
// Slack webhooks satisfy the Slack destination without requiring a Slack bot.
func explainMissingChannels(results []services.ConnectorResult, expected, excluded []string) []services.ConnectorResult {
	skip := map[string]bool{}
	for _, channel := range excluded {
		skip[strings.ToLower(strings.TrimSpace(channel))] = true
	}
	for _, name := range expected {
		channel := strings.ToLower(strings.TrimSpace(name))
		if channel == "" || skip[channel] {
			continue
		}
		found := false
		for i, result := range results {
			matches := result.Channel == channel || (channel == "slack" && strings.HasPrefix(result.Channel, "slack_webhook"))
			if !matches {
				continue
			}
			found = true
			if result.OK && result.MsgID == "" {
				results[i].OK = false
				results[i].Err = fmt.Sprintf("%s was expected but no delivery destination was resolved; nothing was sent on this channel.", result.Channel)
			} else if !result.OK && result.Err == "" {
				results[i].Err = fmt.Sprintf("%s delivery failed without a provider error detail.", result.Channel)
			}
		}
		if !found {
			results = append(results, services.ConnectorResult{Channel: channel, OK: false,
				Err: fmt.Sprintf("%s was requested but its delivery connector is unavailable or disabled; no delivery was attempted. Check this channel's notification settings.", channel)})
		}
	}
	return results
}
