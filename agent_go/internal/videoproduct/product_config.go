package videoproduct

import (
	"embed"
	"fmt"
	"sync"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
)

//go:embed product.yaml prompts/system-prompt.md commands/*.md
var productConfigFiles embed.FS

// ProductManifest is the declarative, reusable product contract. It references
// registered UI surfaces and backend tool factories rather than embedding code.
// ProductManifest is the shared product.yaml shape (pkg/agentprofiles).
type ProductManifest = agentprofiles.ProductManifest

var (
	productManifestOnce sync.Once
	productManifest     ProductManifest
	productManifestErr  error
)

func VideoStudioManifest() (ProductManifest, error) {
	productManifestOnce.Do(func() {
		manifest, err := agentprofiles.LoadProductManifest(productConfigFiles, "product.yaml")
		if err != nil {
			productManifestErr = fmt.Errorf("Video Studio %w", err)
			return
		}
		if manifest.Profile.ID != "video-studio" || manifest.UI.Surface != "video-studio" {
			productManifestErr = fmt.Errorf("invalid Video Studio product manifest")
			return
		}
		productManifest = manifest
	})
	return productManifest, productManifestErr
}

// renderProductPrompt renders the primary profile's prompt with the
// per-turn placeholders that survive to the second (text/template) stage.
func renderProductPrompt(_ string, localTime string) string {
	manifest := mustVideoStudioManifest()
	prompt, err := manifest.RenderPrompt(productConfigFiles, manifest.Profile, map[string]string{"TIME": localTime})
	if err != nil {
		panic(fmt.Errorf("render Video Studio prompt: %w", err))
	}
	return prompt
}

func mustVideoStudioManifest() ProductManifest {
	manifest, err := VideoStudioManifest()
	if err != nil {
		panic(err)
	}
	return manifest
}
