package virtualtools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
)

// TestImageEditToolP0Live drives the real image_edit tool surface
// (CreateImageEditExecutor) end to end. No prior test of any kind in this
// repo -- go test or manual CLI command -- exercised image_edit at all;
// cmd/testing/image_gen_providers.go only covers image_gen.
//
// Requires IMAGE_EDIT_TOOL_P0_LIVE=1, IMAGE_EDIT_TOOL_P0_WORKSPACE_URL
// pointing at a running workspace server, IMAGE_EDIT_TOOL_P0_DOCS_DIR set
// to that server's own --docs-dir, and IMAGE_EDIT_TOOL_P0_SOURCE_IMAGE
// pointing at a full absolute path (under that docs dir) of a real image
// to edit. vertex is skipped when no credentials are configured for this
// environment.
func TestImageEditToolP0Live(t *testing.T) {
	workspaceURL := strings.TrimSpace(os.Getenv("IMAGE_EDIT_TOOL_P0_WORKSPACE_URL"))
	docsDir := strings.TrimSpace(os.Getenv("IMAGE_EDIT_TOOL_P0_DOCS_DIR"))
	sourceImage := strings.TrimSpace(os.Getenv("IMAGE_EDIT_TOOL_P0_SOURCE_IMAGE"))
	if workspaceURL == "" || docsDir == "" || sourceImage == "" {
		t.Skip("set IMAGE_EDIT_TOOL_P0_LIVE=1, IMAGE_EDIT_TOOL_P0_WORKSPACE_URL, IMAGE_EDIT_TOOL_P0_DOCS_DIR, and IMAGE_EDIT_TOOL_P0_SOURCE_IMAGE to run the live image_edit tool P0")
	}
	if os.Getenv("IMAGE_EDIT_TOOL_P0_LIVE") != "1" {
		t.Skip("set IMAGE_EDIT_TOOL_P0_LIVE=1 to run the live image_edit tool P0")
	}
	if _, err := os.Stat(sourceImage); err != nil {
		t.Fatalf("IMAGE_EDIT_TOOL_P0_SOURCE_IMAGE %q does not exist: %v", sourceImage, err)
	}

	executor := CreateImageEditExecutor(ImageGenExecutorConfig{WorkspaceAPIURL: workspaceURL, UserID: "default"})
	if executor == nil {
		t.Fatal("image_edit executor is not available")
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

			outputFolder := "_users/default/Chats/image-edit-tool-p0"
			outputPath := filepath.Join(docsDir, filepath.FromSlash(outputFolder), tc.provider+"-"+fmt.Sprint(time.Now().Unix())+".png")
			sourceFolder := "."
			if rel, err := filepath.Rel(docsDir, filepath.Dir(sourceImage)); err == nil {
				sourceFolder = filepath.ToSlash(rel)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			ctx = context.WithValue(ctx, common.FolderGuardAllowedWriteFolderKey, []string{outputFolder})
			ctx = context.WithValue(ctx, common.FolderGuardReadPathsKey, []string{outputFolder, sourceFolder})
			defer cancel()

			result, err := executor(ctx, map[string]any{
				"image_path":  sourceImage,
				"output_path": outputPath,
				"prompt":      "Recolor this image to a single flat pure blue, filling the entire frame. Keep it a simple solid color image with no other elements.",
				"provider":    tc.provider,
				"model_id":    tc.modelID,
			})
			if err != nil {
				t.Fatalf("image_edit(%s) failed: %v", tc.provider, err)
			}
			if strings.TrimSpace(result) == "" {
				t.Fatal("image_edit returned an empty response")
			}
			if _, err := os.Stat(outputPath); err != nil {
				t.Fatalf("image_edit(%s) reported success but no file exists at %q: %v", tc.provider, outputPath, err)
			}
		})
	}
}
