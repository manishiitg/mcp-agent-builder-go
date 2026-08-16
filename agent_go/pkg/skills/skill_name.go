package skills

import (
	"fmt"
	"path"
	"strings"
)

// ValidateSkillName is the host boundary for a skill folder name.
//
// A skill name is one directory segment under skills/. It is joined into a
// workspace path, and path.Join collapses "..", so an unvalidated name escapes
// skills/ exactly the way an unvalidated relative path does. The sibling
// relPath in NewInstalledSkillReader was already rejected for those shapes;
// this applies the same rule to the name it is joined with.
//
// A separator is rejected outright rather than only "..": a skill name is never
// multi-segment, so refusing every slash removes the whole category instead of
// the one instance of it.
//
// This matters more now that the name is model-supplied — read_skill takes it
// as a tool argument, so anything the agent reads can influence it.
func ValidateSkillName(skillName string) (string, error) {
	name := strings.TrimSpace(skillName)
	if name == "" {
		return "", fmt.Errorf("skill name is required")
	}
	if path.IsAbs(name) ||
		strings.ContainsAny(name, "/\\") ||
		strings.ContainsRune(name, '\x00') ||
		name == "." || name == ".." || strings.Contains(name, "..") {
		return "", fmt.Errorf("skill name must be a single path segment: %q", skillName)
	}
	return name, nil
}
