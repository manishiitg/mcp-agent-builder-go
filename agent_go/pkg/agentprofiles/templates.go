package agentprofiles

import (
	"fmt"
	"io/fs"
	"strings"
)

// RenderPromptTemplate expands `{{name}}` placeholders against the given
// variables. This is deliberately not a general template engine -- a
// product's prompt stays declarative text, not something that can execute or
// resolve arbitrary data. Every product that renders a system prompt or a
// command prompt does the same substitution; this is that one implementation
// instead of each product carrying its own copy of the loop.
func RenderPromptTemplate(template string, variables map[string]string) string {
	rendered := template
	for name, value := range variables {
		rendered = strings.ReplaceAll(rendered, "{{"+name+"}}", value)
	}
	return rendered
}

// ResolveCommandPrompts reads each CommandBinding's File from fsys and fills
// in its Prompt, in place. A product's commands are declared in its own
// product.yaml with the prompt text in a separate file -- prompts are prompts,
// they read better next to the system prompt than inline in a manifest -- so
// this is the one place that resolution happens rather than every product
// reimplementing "read the file, trim it, refuse if empty."
//
// fsys is typically a product's own //go:embed'd filesystem; embed.FS
// satisfies fs.FS, so this works without either package depending on the
// other. A missing or empty prompt is an error, not a skipped command: a
// command that reaches the menu and submits nothing is worse than one that
// was never offered, and a broken product should fail to load, not fail
// silently in front of a user.
func ResolveCommandPrompts(fsys fs.FS, commands []CommandBinding) error {
	for i, command := range commands {
		file := strings.TrimSpace(command.File)
		if file == "" {
			return fmt.Errorf("command %q declares no file", command.Name)
		}
		contents, err := fs.ReadFile(fsys, file)
		if err != nil {
			return fmt.Errorf("read command %q: %w", command.Name, err)
		}
		prompt := strings.TrimSpace(string(contents))
		if prompt == "" {
			return fmt.Errorf("command %q has an empty prompt", command.Name)
		}
		commands[i].Prompt = prompt
	}
	return nil
}
