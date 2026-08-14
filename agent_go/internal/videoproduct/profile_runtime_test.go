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
}
