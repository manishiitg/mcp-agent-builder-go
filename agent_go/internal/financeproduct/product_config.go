package financeproduct

import (
	"bytes"
	"embed"
	"fmt"
	"sync"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/productdeps"
	"gopkg.in/yaml.v3"
)

//go:embed product.yaml prompts/system-prompt.md
var productConfigFiles embed.FS

// ProductManifest mirrors videoproduct.ProductManifest's shape, even though
// Finance leaves most sections empty (no dependencies, fixed workflows,
// commands, or UI panels).
type ProductManifest struct {
	SchemaVersion int                  `yaml:"schema_version"`
	Dependencies  productdeps.Manifest `yaml:"dependencies"`
	Prompt        struct {
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

func FinanceManifest() (ProductManifest, error) {
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
		if productManifest.SchemaVersion != 2 ||
			productManifest.Profile.ID != "finance" ||
			productManifest.Profile.Scope != agentprofiles.ProfileScopeProject ||
			productManifest.UI.Surface != "finance" ||
			productManifest.Prompt.File == "" {
			productManifestErr = fmt.Errorf("invalid Finance product manifest")
			return
		}
		if err := productdeps.Validate(productManifest.Dependencies); err != nil {
			productManifestErr = fmt.Errorf("invalid Finance dependencies: %w", err)
			return
		}
	})
	return productManifest, productManifestErr
}

// renderProductPrompt expands only declared product.yaml prompt variables,
// same as the other two products -- data-driven, not hardcoded to zero
// variables, so a future variable can be added the same way without
// changing this function.
func renderProductPrompt() string {
	manifest := mustFinanceManifest()
	contents, err := productConfigFiles.ReadFile(manifest.Prompt.File)
	if err != nil {
		panic(fmt.Errorf("read Finance prompt %q: %w", manifest.Prompt.File, err))
	}
	return agentprofiles.RenderPromptTemplate(string(contents), manifest.Prompt.Variables)
}

func mustFinanceManifest() ProductManifest {
	manifest, err := FinanceManifest()
	if err != nil {
		panic(err)
	}
	return manifest
}
