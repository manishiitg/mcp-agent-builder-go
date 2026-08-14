package videoproduct

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/workspace"
)

func TestProfileWorkspaceRootMatchesSessionFolderGuard(t *testing.T) {
	const sessionID = "video-studio:test"
	root := profileWorkspaceRoot("default", "Chats/Video Studio/projects/demo")
	if root != "_users/default/Chats/Video Studio/projects/demo" {
		t.Fatalf("unexpected scoped root: %q", root)
	}

	workspace.SetSessionFolderGuard(sessionID, []string{root}, []string{root})
	defer workspace.ClearSessionShellConfig(sessionID)
	client := workspace.NewClient("http://unused", workspace.WithExtraEnv(map[string]string{"MCP_SESSION_ID": sessionID}))
	ctx := context.WithValue(context.Background(), common.ChatSessionIDKey, sessionID)

	qaPath := filepath.ToSlash(filepath.Join(root, "work/qa/final/quality-report.json"))
	if err := client.ValidatePathWithContext(ctx, qaPath, false); err != nil {
		t.Fatalf("profile tool path should match its session guard: %v", err)
	}
	if err := client.ValidatePathWithContext(ctx, "Chats/Video Studio/projects/demo/work/qa/final/quality-report.json", false); err == nil {
		t.Fatal("unscoped profile path unexpectedly bypassed the user-scoped session guard")
	}
}

func TestProfileWorkspaceLocalPathUsesScopedWorkspaceDocsRoot(t *testing.T) {
	root := t.TempDir()
	previous := os.Getenv("WORKSPACE_DOCS_PATH")
	t.Cleanup(func() { _ = os.Setenv("WORKSPACE_DOCS_PATH", previous) })
	if err := os.Setenv("WORKSPACE_DOCS_PATH", root); err != nil {
		t.Fatal(err)
	}
	got, err := profileWorkspaceLocalPath("user-1", "Chats/Video Studio/projects/demo")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "_users", "user-1", "Chats", "Video Studio", "projects", "demo")
	if got != want {
		t.Fatalf("local profile workspace = %q, want %q", got, want)
	}
}

func TestProfileWorkspaceRootDoesNotDoublePrefixCanonicalPath(t *testing.T) {
	got := profileWorkspaceRoot("default", "_users/default/Chats/Video Studio/projects/demo")
	if got != "_users/default/Chats/Video Studio/projects/demo" {
		t.Fatalf("unexpected canonical root: %q", got)
	}
}

func TestGeneratedVideoStudioPlanRefreshesWhenCritiqueGatesAreMissing(t *testing.T) {
	plan, err := json.Marshal(planForAll([]*Pipeline{infographicPipeline}))
	if err != nil {
		t.Fatal(err)
	}
	if shouldRefreshGeneratedVideoStudioPlan(string(plan)) {
		t.Fatal("the current critique-gated product plan should not refresh repeatedly")
	}

	preCritiquePlan := `{"routes":[{"route_id": "infographic"}],"steps":[{"id":"infographic-research"}]}`
	if !shouldRefreshGeneratedVideoStudioPlan(preCritiquePlan) {
		t.Fatal("a pre-critique Video Studio infographic plan should upgrade")
	}

	userPlan := `{"steps":[{"id":"my-video-review","title":"Video review"}]}`
	if shouldRefreshGeneratedVideoStudioPlan(userPlan) {
		t.Fatal("an unrelated user-authored video plan must be preserved")
	}

	// A plan seeded after the critic gates landed but before pre-production was
	// orchestrated satisfies every older fingerprint, so nothing upgraded it and
	// the project kept a linear plan with no orchestrator — which is what the
	// running instance actually showed.
	preOrchestrator := `{"steps":[{"id":"route","routes":[{"route_id": "infographic"}]},` +
		`{"id":"infographic-research"},{"id":"infographic-creative-critique"},` +
		`{"id":"infographic-render-critique"}]}`
	if !shouldRefreshGeneratedVideoStudioPlan(preOrchestrator) {
		t.Fatal("a pre-orchestrator Video Studio infographic plan should upgrade")
	}

	// The case marker-matching cannot see: every current identifier is present,
	// so no fingerprint fires, but the stored plan is one the platform refuses
	// to load. This is what a project seeded between the orchestrator landing
	// and its next_step_id fix actually held, and run_full_workflow failed with
	// "plan.json uses an invalid or legacy format" rather than running.
	invalidButCurrent := `{"steps":[{"type":"routing","id":"route","routes":[{"route_id": "infographic","next_step_id":"infographic-preproduction"}]},` +
		`{"type":"todo_task","id":"infographic-preproduction","title":"Brief","description":"d",` +
		`"predefined_routes":[{"route_id":"infographic-research","route_name":"r","condition":"c",` +
		`"sub_agent_step":{"type":"message_sequence","id":"infographic-research","title":"t","description":"d",` +
		`"items":[{"id":"i","type":"user_message","message":"m"}]}}]},` +
		`{"type":"message_sequence","id":"infographic-render-critique","title":"t","description":"d",` +
		`"items":[{"id":"i","type":"user_message","message":"m"}],"next_step_id":"end"}]}`
	if planLoadsOnThisPlatform(invalidButCurrent) {
		t.Fatal("fixture is meant to be a plan the platform rejects; it now loads, so this asserts nothing")
	}
	if !shouldRefreshGeneratedVideoStudioPlan(invalidButCurrent) {
		t.Fatal("a stored plan the platform will not load must be re-seeded, whatever markers it carries")
	}
}
