package sparkquillproduct

import "testing"

// Every SparkQuill binding runs at medium reasoning effort: a family chat
// wants quick, steady replies, not the builder-grade deliberation the
// provider defaults to for coding work.
func TestEveryProviderOptionDeclaresMediumReasoningEffort(t *testing.T) {
	profiles := BuiltinAgentProfiles()
	if len(profiles) == 0 {
		t.Fatal("no built-in profiles")
	}
	for _, p := range profiles {
		if len(p.Runtime.ProviderOptions) == 0 {
			t.Fatalf("profile %s declares no provider options", p.ID)
		}
		for _, o := range p.Runtime.ProviderOptions {
			if o.Options["reasoning_effort"] != "medium" {
				t.Fatalf("profile %s option %s: reasoning_effort = %v, want medium", p.ID, o.ID, o.Options["reasoning_effort"])
			}
		}
	}
}
