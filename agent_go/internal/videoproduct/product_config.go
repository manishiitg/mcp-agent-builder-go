package videoproduct

import (
	"bytes"
	"embed"
	"fmt"
	"sync"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/productdeps"
	"gopkg.in/yaml.v3"
)

//go:embed product.yaml prompts/system-prompt.md commands/*.md
var productConfigFiles embed.FS

// ProductManifest is the declarative, reusable product contract. It references
// registered UI surfaces and backend tool factories rather than embedding code.
type ProductManifest struct {
	SchemaVersion int `yaml:"schema_version"`
	// Dependencies are provisioned per isolated project workspace by the shared
	// product dependency harness. Their declarations remain portable across
	// products instead of becoming Video Studio-specific Go configuration.
	Dependencies productdeps.Manifest `yaml:"dependencies"`
	Prompt       struct {
		File      string            `yaml:"file"`
		Variables map[string]string `yaml:"variables"`
	} `yaml:"prompt"`
	Profile  agentprofiles.Profile `yaml:"profile"`
	Branding struct {
		Icon    string `yaml:"icon"`
		Favicon string `yaml:"favicon"`
	} `yaml:"branding"`
	UI struct {
		Surface       string `yaml:"surface"`
		FilesPanel    bool   `yaml:"files_panel"`
		WorkflowPanel bool   `yaml:"workflow_panel"`
		Secrets       bool   `yaml:"secrets"`
		Streaming     string `yaml:"streaming"`
	} `yaml:"ui"`
	Workflows struct {
		Enabled        []string `yaml:"enabled"`
		SelectedSkills []string `yaml:"selected_skills"`
		BrowserMode    string   `yaml:"browser_mode"`
	} `yaml:"workflows"`
}

var (
	productManifestOnce sync.Once
	productManifest     ProductManifest
	productManifestErr  error
)

func VideoStudioManifest() (ProductManifest, error) {
	productManifestOnce.Do(func() {
		contents, err := productConfigFiles.ReadFile("product.yaml")
		if err != nil {
			productManifestErr = fmt.Errorf("read product manifest: %w", err)
			return
		}
		decoder := yaml.NewDecoder(bytes.NewReader(contents))
		decoder.KnownFields(true)
		if err := decoder.Decode(&productManifest); err != nil {
			productManifestErr = fmt.Errorf("parse product manifest: %w", err)
			return
		}
		if productManifest.SchemaVersion != 2 || productManifest.Profile.ID != "video-studio" || productManifest.UI.Surface != "video-studio" || productManifest.Prompt.File == "" {
			productManifestErr = fmt.Errorf("invalid Video Studio product manifest")
			return
		}
		if err := productdeps.Validate(productManifest.Dependencies); err != nil {
			productManifestErr = fmt.Errorf("invalid Video Studio dependencies: %w", err)
			return
		}
		// Slash-command prompts live in their own files so they read as prompts
		// rather than as manifest data; the shared resolver reads each one and
		// fills in Prompt so everything downstream -- including the profile the
		// frontend fetches -- sees a command that already carries its text. A
		// declared command whose file is missing is a broken product, not a
		// command to quietly drop.
		if err := agentprofiles.ResolveCommandPrompts(productConfigFiles, productManifest.Profile.Commands); err != nil {
			productManifestErr = fmt.Errorf("Video Studio %w", err)
			return
		}
	})
	return productManifest, productManifestErr
}

// renderProductPrompt expands only known product/runtime values. This is
// deliberately not a general template engine: product YAML stays declarative
// and cannot execute or resolve arbitrary data.
func renderProductPrompt(_ string, localTime string) string {
	manifest := mustVideoStudioManifest()
	contents, err := productConfigFiles.ReadFile(manifest.Prompt.File)
	if err != nil {
		panic(fmt.Errorf("read Video Studio prompt %q: %w", manifest.Prompt.File, err))
	}
	values := map[string]string{"TIME": localTime}
	for name, value := range manifest.Prompt.Variables {
		values[name] = value
	}
	return agentprofiles.RenderPromptTemplate(string(contents), values)
}

func mustVideoStudioManifest() ProductManifest {
	manifest, err := VideoStudioManifest()
	if err != nil {
		panic(err)
	}
	return manifest
}
