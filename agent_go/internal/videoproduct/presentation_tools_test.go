package videoproduct

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
)

// Same contract show_video is held to: the presentation kind comes from the
// profile's declaration, so a product.yaml that forgets tools[].presentation.kind
// fails loudly at the first call instead of silently writing a kind nobody
// declared. Checked before any workspace read, so no fake server is needed.
func TestPresentationToolsRequireADeclaredKind(t *testing.T) {
	for _, tc := range []struct {
		name    string
		factory agentprofiles.ToolFactory
		args    map[string]interface{}
	}{
		{
			name:    "show_character",
			factory: showCharacterFactory("http://unused"),
			args: map[string]interface{}{
				"name": "Aang", "image_path": "work/characters/aang.png", "spec_path": "work/characters/aang.md",
			},
		},
		{
			name:    "show_document",
			factory: showDocumentFactory("http://unused"),
			args:    map[string]interface{}{"path": "work/longform-script.md", "title": "Script"},
		},
	} {
		spec, err := tc.factory(agentprofiles.ToolRuntimeContext{
			UserID: "u1", SessionID: "s1", WorkspacePath: "Chats/Video Studio/projects/demo",
			// Presentation intentionally left nil.
		}, nil)
		if err != nil {
			t.Fatalf("%s: building the tool spec itself should not fail: %v", tc.name, err)
		}
		result, err := spec.Execute(context.Background(), tc.args)
		if err == nil {
			t.Fatalf("%s: expected an error with no declared presentation kind, got result %q", tc.name, result)
		}
		if !strings.Contains(err.Error(), "presentation kind") {
			t.Fatalf("%s: error should name the missing declaration, got: %v", tc.name, err)
		}
	}
}

// Input validation runs before any workspace read, so these exercise the
// early-return path without a fake server. A character shown without a real
// reference image is the exact failure the panel exists to prevent, and a
// document tool that accepted any file would make the panel a file browser.
func TestPresentationToolsRejectUnusableInput(t *testing.T) {
	declared := agentprofiles.ToolRuntimeContext{
		UserID: "u1", SessionID: "s1", WorkspacePath: "Chats/Video Studio/projects/demo",
		Presentation: &agentprofiles.PresentationBinding{Kind: "test.kind"},
	}

	character, err := showCharacterFactory("http://unused")(declared, nil)
	if err != nil {
		t.Fatalf("building show_character: %v", err)
	}
	for _, tc := range []struct {
		why  string
		args map[string]interface{}
		want string
	}{
		{
			why:  "a character with no name has no stable identity to update against",
			args: map[string]interface{}{"name": "  ", "image_path": "a.png", "spec_path": "a.md"},
			want: "character's name",
		},
		{
			why:  "the reference image is the whole point; a .md in its place is not one",
			args: map[string]interface{}{"name": "Aang", "image_path": "a.md", "spec_path": "a.md"},
			want: "reference image",
		},
		{
			why:  "the spec must be readable prose, not another image",
			args: map[string]interface{}{"name": "Aang", "image_path": "a.png", "spec_path": "a.png"},
			want: "character spec",
		},
		{
			why:  "a path escaping the project must not resolve",
			args: map[string]interface{}{"name": "Aang", "image_path": "../../etc/passwd.png", "spec_path": "a.md"},
			want: "reference image",
		},
	} {
		result, err := character.Execute(context.Background(), tc.args)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.why, err)
		}
		if !strings.Contains(result, tc.want) {
			t.Fatalf("%s: got %q, want a message naming %q", tc.why, result, tc.want)
		}
	}

	document, err := showDocumentFactory("http://unused")(declared, nil)
	if err != nil {
		t.Fatalf("building show_document: %v", err)
	}
	for _, tc := range []struct {
		why  string
		args map[string]interface{}
	}{
		{why: "a rendered video is not a document", args: map[string]interface{}{"path": "outputs/final.mp4", "title": "x"}},
		{why: "a path escaping the project must not resolve", args: map[string]interface{}{"path": "../../secrets.md", "title": "x"}},
	} {
		result, err := document.Execute(context.Background(), tc.args)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.why, err)
		}
		if !strings.Contains(result, "project-relative .md path") {
			t.Fatalf("%s: got %q, want the .md validation message", tc.why, result)
		}
	}
}

// Slash commands are declared in product.yaml with their prompts in separate
// files, so two things can silently break: a declared command whose file went
// missing, and the internal file path leaking to the browser. The manifest
// loader treats a missing or empty prompt as a broken product rather than
// dropping the command, because a command that appears in the menu and submits
// nothing is worse than one that was never offered.
func TestProductCommandsCarryTheirPromptsAndHideTheirPaths(t *testing.T) {
	manifest, err := VideoStudioManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Profile.Commands) == 0 {
		t.Fatal("the product declares no slash commands")
	}

	seen := map[string]bool{}
	for _, command := range manifest.Profile.Commands {
		if seen[command.Name] {
			t.Fatalf("duplicate command name %q; the menu would show two identical entries", command.Name)
		}
		seen[command.Name] = true
		if strings.TrimSpace(command.Description) == "" {
			t.Fatalf("command %q has no description, so the menu cannot say what it does", command.Name)
		}
		if strings.TrimSpace(command.Prompt) == "" {
			t.Fatalf("command %q resolved to an empty prompt; it would submit nothing", command.Name)
		}
	}

	// The command that starts a production is the one that has to hold the
	// checkpoints, since a user reaching for it is asking to begin spending.
	var production string
	for _, command := range manifest.Profile.Commands {
		if command.Name == "production" {
			production = command.Prompt
		}
	}
	if production == "" {
		t.Fatal("no /production command; the interview has no entry point")
	}
	for _, want := range []string{"ceiling", "recur", "wait"} {
		if !strings.Contains(strings.ToLower(production), want) {
			t.Fatalf("/production no longer asks about %q before spending", want)
		}
	}

	// File is the product's own layout detail. Serialising it would ship an
	// internal path to every browser that loads the profile.
	encoded, err := json.Marshal(manifest.Profile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "commands/production.md") {
		t.Fatal("the command's source file path is serialized to the client")
	}
	if !strings.Contains(string(encoded), `"prompt"`) {
		t.Fatal("commands serialize without their prompt, so the client has nothing to submit")
	}
}

// Before this, a product had no way to say which credentials its skills
// need, so nothing could answer "what does this workspace still need" before
// a paid run started -- a user found that out only when a generation call
// failed partway through. This pins that both credentials this product
// actually reads (see fal-ai and google-ai's Authentication sections) are
// declared, and that they are grouped rather than each marked independently
// required -- either alone is enough to carry a production, so requiring
// both would be a false constraint no config would ever satisfy exactly.
func TestProductDeclaresTheSecretsItsSkillsRead(t *testing.T) {
	manifest, err := VideoStudioManifest()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]agentprofiles.SecretBinding{}
	for _, secret := range manifest.Profile.Secrets {
		byName[secret.Name] = secret
	}
	for _, name := range []string{"FAL_KEY", "GEMINI_API_KEY"} {
		secret, ok := byName[name]
		if !ok {
			t.Fatalf("product.yaml does not declare %s, but fal-ai/google-ai read it from the environment", name)
		}
		if strings.TrimSpace(secret.Description) == "" {
			t.Fatalf("%s has no description; a user deciding whether to configure it has nothing to read", name)
		}
	}
	if byName["FAL_KEY"].Group == "" || byName["FAL_KEY"].Group != byName["GEMINI_API_KEY"].Group {
		t.Fatal("FAL_KEY and GEMINI_API_KEY must share a group -- either alone carries a production, so neither should be independently required")
	}
}

// The tools are useless unless the product both declares a kind for them and
// admits them through its allowlist -- the two halves live in different parts
// of product.yaml and each is silently inert without the other.
func TestProductDeclaresAndAdmitsThePresentationTools(t *testing.T) {
	manifest, err := VideoStudioManifest()
	if err != nil {
		t.Fatal(err)
	}

	kinds := map[string]string{}
	for _, tool := range manifest.Profile.Tools {
		if tool.Presentation == nil {
			continue
		}
		kinds[tool.ID] = tool.Presentation.Kind
	}
	for id, want := range map[string]string{
		"video.show-video":     "media.video",
		"video.show-character": "media.character",
		"video.show-document":  "document.markdown",
	} {
		if kinds[id] != want {
			t.Fatalf("%s declares presentation kind %q, want %q", id, kinds[id], want)
		}
	}

	admitted := map[string]bool{}
	for _, name := range manifest.Profile.ToolPolicy.Enabled {
		admitted[name] = true
	}
	for _, name := range []string{"show_video", "show_character", "show_document"} {
		if !admitted[name] {
			t.Fatalf("%s is not in the tool allowlist, so the agent never receives it: %v", name, manifest.Profile.ToolPolicy.Enabled)
		}
	}
}
