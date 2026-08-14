package videoproduct

import (
	"context"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/productdeps"
)

func syncManagedProductSkills(ctx context.Context, workspacePath string) error {
	return productdeps.Ensure(ctx, workspacePath, mustVideoStudioManifest().Dependencies)
}
