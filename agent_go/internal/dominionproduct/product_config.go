package dominionproduct

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

// DominionManifest loads and validates product.yaml once.
func DominionManifest() (ProductManifest, error) {
	productManifestOnce.Do(func() {
		manifest, err := agentprofiles.LoadProductManifest(productConfigFiles, "product.yaml")
		if err != nil {
			productManifestErr = fmt.Errorf("Dominion %w", err)
			return
		}
		if manifest.Profile.ID != "dominion" || manifest.Profile.Scope != agentprofiles.ProfileScopeProject || manifest.UI.Surface != "dominion" {
			productManifestErr = fmt.Errorf("invalid Dominion product manifest")
			return
		}
		productManifest = manifest
	})
	return productManifest, productManifestErr
}

func renderProductPrompt() string {
	manifest := mustDominionManifest()
	prompt, err := manifest.RenderPrompt(productConfigFiles, manifest.Profile, nil)
	if err != nil {
		panic(fmt.Errorf("render Dominion prompt: %w", err))
	}
	return prompt
}

func mustDominionManifest() ProductManifest {
	manifest, err := DominionManifest()
	if err != nil {
		panic(err)
	}
	return manifest
}
