package sparkquillproduct

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"
)

// ActivitiesFolder is where every activity lives under the family root: one
// folder per activity, and each folder is also the child profile's project
// (its product.json makes it a keyed conversation).
const ActivitiesFolder = "activities"

// ActivityManifest is activity.json, the same shape the standalone app
// writes.
type ActivityManifest struct {
	Title     string   `json:"title"`
	Subject   string   `json:"subject,omitempty"`
	Topic     string   `json:"topic,omitempty"`
	Items     []string `json:"items,omitempty"`
	Goal      string   `json:"goal,omitempty"`
	Persona   string   `json:"persona,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
}

// activityProject is the product.json the platform's keyed-conversation
// resolver reads (cmd/server/product_conversation_registry.go).
type activityProject struct {
	SchemaVersion int    `json:"schema_version"`
	Product       string `json:"product"`
	ID            string `json:"id"`
	Title         string `json:"title"`
	Description   string `json:"description,omitempty"`
	SessionID     string `json:"session_id"`
}

var activitySlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,99}$`)

// activityFolder normalizes what the model passed as the activity folder to
// "activities/<slug>" relative to the family root, accepting a bare slug, a
// full "activities/<slug>", or a family-root-prefixed path.
func activityFolder(familyRoot, raw string) (rel, slug string, err error) {
	clean := strings.Trim(strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/")), "/")
	root := strings.Trim(familyRoot, "/")
	if root != "" && strings.HasPrefix(clean, root+"/") {
		clean = strings.TrimPrefix(clean, root+"/")
	}
	clean = strings.TrimPrefix(clean, ActivitiesFolder+"/")
	if clean == "" || strings.Contains(clean, "/") || strings.Contains(clean, "..") {
		return "", "", fmt.Errorf("dir must be a single activity folder under %s/, e.g. %s/2026-09-03-fractions-quick-check", ActivitiesFolder, ActivitiesFolder)
	}
	slug = strings.ToLower(clean)
	if !activitySlugPattern.MatchString(slug) {
		return "", "", fmt.Errorf("activity folder %q must be a lowercase slug (letters, digits, dots, dashes)", clean)
	}
	return path.Join(ActivitiesFolder, slug), slug, nil
}

// isAnswerKey mirrors the standalone app's rule: *-KEY.md never becomes an
// item the child sees.
func isAnswerKey(name string) bool {
	base := strings.ToLower(path.Base(name))
	return strings.HasSuffix(base, "-key.md") || strings.HasSuffix(base, "-key.markdown")
}

func encodeJSON(v interface{}) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}

func nowStamp() string { return time.Now().UTC().Format(time.RFC3339) }
