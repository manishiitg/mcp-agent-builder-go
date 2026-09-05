package virtualtools

import (
	"github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/services"
	"testing"
)

func TestExpectedGmailCannotDisappear(t *testing.T) {
	slack := []services.ConnectorResult{{Channel: "slack_webhook", OK: true, MsgID: "sent"}}
	got := explainMissingGmail(slack, true, nil, nil)
	if len(got) != 2 || got[1].Channel != "gmail" || got[1].OK || got[1].Err == "" {
		t.Fatalf("receipt=%+v", got)
	}
	if len(explainMissingGmail(slack, false, nil, nil)) != 1 {
		t.Fatal("invented expected channel")
	}
	if len(explainMissingGmail(slack, true, []string{"gmail"}, nil)) != 1 {
		t.Fatal("explicit exclusion ignored")
	}
	skipped := explainMissingGmail([]services.ConnectorResult{{Channel: "gmail", OK: true}}, true, nil, nil)
	if skipped[0].OK || skipped[0].Err == "" {
		t.Fatal("missing recipient silently succeeded")
	}
	sent := explainMissingGmail([]services.ConnectorResult{{Channel: "gmail", OK: true, MsgID: "sent"}}, true, nil, nil)
	if !sent[0].OK {
		t.Fatal("successful email changed")
	}
}
