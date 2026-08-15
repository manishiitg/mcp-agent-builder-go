package videoproduct

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTmpCommandsSerialize(t *testing.T) {
	m, err := VideoStudioManifest()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(m.Profile)
	for _, name := range []string{"production", "characters", "plan", "revise", "check"} {
		if !strings.Contains(string(b), `"name":"`+name+`"`) {
			t.Fatalf("command %q missing from serialized profile", name)
		}
	}
	if strings.Contains(string(b), `"file"`) {
		t.Fatal("file path leaked to the client")
	}
	for _, c := range m.Profile.Commands {
		t.Logf("/%s -> %d chars, icon=%s", c.Name, len(c.Prompt), c.Icon)
	}
}
