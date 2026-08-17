package chiefofstaffproduct

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

// ProductManifest mirrors videoproduct.ProductManifest's shape so the two
// stay decodable the same way, even though Chief of Staff leaves most
// sections empty (no dependencies, no fixed workflows, no schedules yet).
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

func ChiefOfStaffManifest() (ProductManifest, error) {
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
			productManifest.Profile.ID != "chief-of-staff" ||
			productManifest.Profile.Scope != agentprofiles.ProfileScopeGlobal ||
			productManifest.UI.Surface != "chief-of-staff" ||
			productManifest.Prompt.File == "" {
			productManifestErr = fmt.Errorf("invalid Chief of Staff product manifest")
			return
		}
		if err := productdeps.Validate(productManifest.Dependencies); err != nil {
			productManifestErr = fmt.Errorf("invalid Chief of Staff dependencies: %w", err)
			return
		}
		// Slash-command prompts live in their own files so they read as prompts
		// rather than as manifest data; the shared resolver reads each one and
		// fills in Prompt so everything downstream -- including the profile the
		// frontend fetches -- sees a command that already carries its text. A
		// declared command whose file is missing is a broken product, not a
		// command to quietly drop.
		if err := agentprofiles.ResolveCommandPrompts(productConfigFiles, productManifest.Profile.Commands); err != nil {
			productManifestErr = fmt.Errorf("Chief of Staff %w", err)
			return
		}
	})
	return productManifest, productManifestErr
}

// renderProductPrompt expands only declared product.yaml prompt variables.
// Chief of Staff's placeholder prompt declares none today; this stays
// data-driven (not hardcoded to zero variables) so a future variable can be
// added the same way Video Studio's PRODUCT_NAME was, without changing this
// function.
func renderProductPrompt() string {
	manifest := mustChiefOfStaffManifest()
	contents, err := productConfigFiles.ReadFile(manifest.Prompt.File)
	if err != nil {
		panic(fmt.Errorf("read Chief of Staff prompt %q: %w", manifest.Prompt.File, err))
	}
	return agentprofiles.RenderPromptTemplate(string(contents), manifest.Prompt.Variables)
}

func mustChiefOfStaffManifest() ProductManifest {
	manifest, err := ChiefOfStaffManifest()
	if err != nil {
		panic(err)
	}
	return manifest
}
