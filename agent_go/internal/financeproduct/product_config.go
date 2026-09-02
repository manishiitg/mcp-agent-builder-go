package financeproduct

import (
	"embed"
	"fmt"
	"sync"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
)

//go:embed product.yaml prompts/system-prompt.md
var productConfigFiles embed.FS

// ProductManifest is the shared product.yaml shape (pkg/agentprofiles).
type ProductManifest = agentprofiles.ProductManifest

var (
	productManifestOnce sync.Once
	productManifest     ProductManifest
	productManifestErr  error
)

// FinanceManifest loads and validates product.yaml once.
func FinanceManifest() (ProductManifest, error) {
	productManifestOnce.Do(func() {
		manifest, err := agentprofiles.LoadProductManifest(productConfigFiles, "product.yaml")
		if err != nil {
			productManifestErr = fmt.Errorf("Finance %w", err)
			return
		}
		if manifest.Profile.ID != "finance" || manifest.Profile.Scope != agentprofiles.ProfileScopeProject || manifest.UI.Surface != "finance" {
			productManifestErr = fmt.Errorf("invalid Finance product manifest")
			return
		}
		productManifest = manifest
	})
	return productManifest, productManifestErr
}

func renderProductPrompt() string {
	manifest := mustFinanceManifest()
	prompt, err := manifest.RenderPrompt(productConfigFiles, manifest.Profile, nil)
	if err != nil {
		panic(fmt.Errorf("render Finance prompt: %w", err))
	}
	return prompt
}

func mustFinanceManifest() ProductManifest {
	manifest, err := FinanceManifest()
	if err != nil {
		panic(err)
	}
	return manifest
}
