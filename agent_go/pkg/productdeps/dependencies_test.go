package productdeps

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMirrorAndLoadAttachedSkillUsesNormalSkillsFolder(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, ".agents", "skills", "hyperframes")
	if err := os.MkdirAll(filepath.Join(source, "references"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: hyperframes\ndescription: Compose product videos\n---\n\nUse HTML timelines."), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "references", "timeline.md"), []byte("Timeline reference"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := mirrorCLISkills(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "skills", "hyperframes", "SKILL.md")); err != nil {
		t.Fatalf("AgentWorks skills folder was not populated: %v", err)
	}
	skills, err := LoadAttachedSkills(root, Manifest{Skills: []SkillSource{{ID: "hyperframes", Installer: "skills-cli", Source: "heygen-com/hyperframes", Install: []string{"hyperframes"}, Attach: []string{"hyperframes"}, RefreshHours: 24}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 || skills[0].Name != "hyperframes" || skills[0].Source.Origin != "imported" || skills[0].Source.SourceURL != "heygen-com/hyperframes" {
		t.Fatalf("managed skill metadata = %+v", skills)
	}
	if len(skills[0].SupportingFiles) != 1 || skills[0].SupportingFiles[0].RelPath != "references/timeline.md" {
		t.Fatalf("managed skill supporting files = %+v", skills[0].SupportingFiles)
	}
}

func TestValidateMCPServerDoesNotPermitInlineSecrets(t *testing.T) {
	manifest := Manifest{MCPServers: []MCPServer{{ID: "catalog", Enabled: true, Transport: "http", URL: "https://mcp.example.test", Environment: map[string]string{"CATALOG_TOKEN": "secret://catalog-token"}}}}
	if err := Validate(manifest); err != nil {
		t.Fatal(err)
	}
	manifest.MCPServers[0].Environment["CATALOG_TOKEN"] = "plaintext-token"
	if err := Validate(manifest); err == nil {
		t.Fatal("expected inline secret to be rejected")
	}
}

func TestValidateRejectsAttachedSkillThatIsNotInstalled(t *testing.T) {
	err := Validate(Manifest{Skills: []SkillSource{{ID: "bad", Installer: "skills-cli", Source: "example/skills", Install: []string{"one"}, Attach: []string{"two"}, RefreshHours: 24}}})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestNpxCLIArgsRunsDeclaredBinaryFromDeclaredPackage(t *testing.T) {
	dependency := CLIDependency{
		Package:   Package{Ecosystem: "npm", Name: "hyperframes", Version: "latest"},
		Execution: CLIExecution{Mode: "npx", Binary: "hyperframes"},
		Verify:    CLICommand{Args: []string{"doctor", "--json"}},
	}
	want := []string{"--yes", "--package", "hyperframes@latest", "hyperframes", "doctor", "--json"}
	if got := npxCLIArgs(dependency); !reflect.DeepEqual(got, want) {
		t.Fatalf("npx args = %v, want %v", got, want)
	}
}

func TestValidateRequiredJSONChecks(t *testing.T) {
	output := "HyperFrames welcome\n" + `{"checks":[{"name":"Version","ok":true},{"name":"FFmpeg","ok":true},{"name":"Docker","ok":false}]}`
	if err := validateRequiredJSONChecks(output, []string{"Version", "FFmpeg"}); err != nil {
		t.Fatal(err)
	}
	if err := validateRequiredJSONChecks(output, []string{"Docker"}); err == nil {
		t.Fatal("expected failed required check")
	}
	if err := validateRequiredJSONChecks("not JSON", []string{"Version"}); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}
