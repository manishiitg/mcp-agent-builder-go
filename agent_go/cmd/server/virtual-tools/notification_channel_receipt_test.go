package virtualtools

import (
	"github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/services"
	"testing"
)

func TestExpectedChannelErrors(t *testing.T) {
	for _, channel := range []string{"slack", "whatsapp", "gmail"} {
		t.Run(channel, func(t *testing.T) {
			missing := explainMissingChannels(nil, []string{channel}, nil)
			if len(missing) != 1 || missing[0].OK || missing[0].Err == "" {
				t.Fatalf("missing=%+v", missing)
			}
			if len(explainMissingChannels(nil, []string{channel}, []string{channel})) != 0 {
				t.Fatal("excluded channel reported failed")
			}
			original := []services.ConnectorResult{{Channel: channel, Err: "provider said invalid_auth"}}
			got := explainMissingChannels(original, []string{channel}, nil)
			if got[0].Err != "provider said invalid_auth" {
				t.Fatal("provider error replaced")
			}
			skipped := explainMissingChannels([]services.ConnectorResult{{Channel: channel, OK: true}}, []string{channel}, nil)
			if skipped[0].OK {
				t.Fatal("missing destination silently succeeded")
			}
		})
	}
	webhook := []services.ConnectorResult{{Channel: "slack_webhook", OK: true, MsgID: "sent"}}
	if got := explainMissingChannels(webhook, []string{"slack"}, nil); len(got) != 1 || !got[0].OK {
		t.Fatalf("webhook=%+v", got)
	}
}
