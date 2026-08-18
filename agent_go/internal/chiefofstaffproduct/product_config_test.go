package chiefofstaffproduct

import "testing"

func TestParkedChiefOfStaffManifestHasNoRuntimeSurface(t *testing.T) {
	manifest, err := ChiefOfStaffManifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Profile.BuiltIn {
		t.Fatal("parked product must not be built in")
	}
	if len(manifest.Profile.Commands) != 0 || len(manifest.Profile.Skills) != 0 || len(manifest.Profile.Tools) != 0 {
		t.Fatalf("parked product declares runtime capabilities: commands=%v skills=%v tools=%v", manifest.Profile.Commands, manifest.Profile.Skills, manifest.Profile.Tools)
	}
	if manifest.UI.FilesPanel || manifest.UI.WorkflowPanel || manifest.UI.Secrets {
		t.Fatalf("parked product exposes UI panels: %+v", manifest.UI)
	}
	if profiles := BuiltinAgentProfiles(); len(profiles) != 0 {
		t.Fatalf("parked product registered %d profiles", len(profiles))
	}
	if err := RegisterProductSkills(); err != nil {
		t.Fatalf("parked skill registration should be a no-op: %v", err)
	}
}
