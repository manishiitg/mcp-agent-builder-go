package videoproduct

import (
	"context"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/productdeps"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

// managedProductSkills preserves the Video Studio call site while delegating
// the reusable YAML dependency lifecycle to productdeps. It makes configured
// external skills look exactly like ordinary local skills to every provider.
func managedProductSkills(ctx context.Context, workspacePath string) ([]*llmtypes.Skill, error) {
	manifest := mustVideoStudioManifest()
	if err := productdeps.Ensure(ctx, workspacePath, manifest.Dependencies); err != nil {
		return nil, err
	}
	return productdeps.LoadAttachedSkills(workspacePath, manifest.Dependencies)
}

func syncManagedProductSkills(ctx context.Context, workspacePath string) error {
	return productdeps.Ensure(ctx, workspacePath, mustVideoStudioManifest().Dependencies)
}
