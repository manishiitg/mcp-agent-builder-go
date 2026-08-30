package virtualtools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
)

// TestImageGenToolP0Live drives the real image_gen tool surface
// (CreateImageGenExecutor) end to end against every provider it supports,
// proving the tool this repo actually exposes to agents works -- not just
// the underlying multi-llm-provider-go adapter one layer below (see
// PLAT-247's live verification for that layer). No prior test in this repo
// exercised this tool surface at all; cmd/testing/image_gen_providers.go is
// a manual operator CLI command, not a go test.
//
// Requires IMAGE_GEN_TOOL_P0_LIVE=1, IMAGE_GEN_TOOL_P0_WORKSPACE_URL
// pointing at a running workspace server, and IMAGE_GEN_TOOL_P0_DOCS_DIR
// set to that server's own --docs-dir (output_path must be a full absolute
// path under it). vertex is skipped when no credentials are configured for
// this environment, matching cmd/testing/image_gen_providers.go's own
// skip-reason logic.
func TestImageGenToolP0Live(t *testing.T) {
	workspaceURL := strings.TrimSpace(os.Getenv("IMAGE_GEN_TOOL_P0_WORKSPACE_URL"))
	docsDir := strings.TrimSpace(os.Getenv("IMAGE_GEN_TOOL_P0_DOCS_DIR"))
	if workspaceURL == "" || docsDir == "" {
		t.Skip("set IMAGE_GEN_TOOL_P0_LIVE=1, IMAGE_GEN_TOOL_P0_WORKSPACE_URL, and IMAGE_GEN_TOOL_P0_DOCS_DIR to run the live image_gen tool P0")
	}
	if os.Getenv("IMAGE_GEN_TOOL_P0_LIVE") != "1" {
		t.Skip("set IMAGE_GEN_TOOL_P0_LIVE=1 to run the live image_gen tool P0")
	}

	executor := CreateImageGenExecutor(ImageGenExecutorConfig{WorkspaceAPIURL: workspaceURL, UserID: "default"})
	if executor == nil {
		t.Fatal("image_gen executor is not available")
	}

	type providerCase struct {
		provider string
		modelID  string
	}
	cases := []providerCase{
		{"codex-cli", "codex-cli"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.provider, func(t *testing.T) {
			if reason := imageGenToolP0SkipReason(tc.provider); reason != "" {
				t.Skip(reason)
			}

			outputFolder := "_users/default/Chats/image-gen-tool-p0"
			outputPath := filepath.Join(docsDir, filepath.FromSlash(outputFolder), tc.provider+"-"+fmt.Sprint(time.Now().Unix())+".png")
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			ctx = context.WithValue(ctx, common.FolderGuardAllowedWriteFolderKey, []string{outputFolder})
			ctx = context.WithValue(ctx, common.FolderGuardReadPathsKey, []string{outputFolder})
			defer cancel()

			result, err := executor(ctx, map[string]any{
				"prompt":      "A single flat pure red square filling the entire frame, no gradients, no other colors, no text.",
				"provider":    tc.provider,
				"model_id":    tc.modelID,
				"output_path": outputPath,
			})
			if err != nil {
				t.Fatalf("image_gen(%s) failed: %v", tc.provider, err)
			}
			if strings.TrimSpace(result) == "" {
				t.Fatal("image_gen returned an empty response")
			}
			if _, err := os.Stat(outputPath); err != nil {
				t.Fatalf("image_gen(%s) reported success but no file exists at %q: %v", tc.provider, outputPath, err)
			}
		})
	}
}

func imageGenToolP0SkipReason(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "codex-cli":
		if _, err := exec.LookPath("codex"); err != nil {
			return "codex CLI is not installed or not on PATH"
		}
	case "vertex":
		if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" && os.Getenv("GOOGLE_CLOUD_PROJECT") == "" && os.Getenv("GEMINI_API_KEY") == "" {
			return "no Vertex/Gemini credentials configured in this environment"
		}
	}
	return ""
}
