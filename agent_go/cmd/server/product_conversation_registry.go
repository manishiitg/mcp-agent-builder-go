package server

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
)

const (
	productConversationRegistryVersion = 1
	productConversationRegistryFile    = "product-conversations.json"
)

var (
	productConversationKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,255}$`)
	productConversationLocks      [64]sync.Mutex
)

// ProductConversationRecord is the durable identity binding for one product
// conversation. ConversationID is the logical chat; SessionID is the current
// AgentWorks history/runtime owner. They are intentionally separate so a
// future runtime migration does not change the product conversation identity.
type ProductConversationRecord struct {
	ConversationID  string `json:"conversation_id"`
	ConversationKey string `json:"conversation_key"`
	ProfileID       string `json:"profile_id"`
	ProfileVersion  int    `json:"profile_version"`
	SessionID       string `json:"session_id"`
	WorkspacePath   string `json:"workspace_path"`
	ResourceID      string `json:"resource_id,omitempty"`
	Title           string `json:"title,omitempty"`
	Description     string `json:"description,omitempty"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type productConversationRegistryDocument struct {
	Version int                                  `json:"version"`
	Entries map[string]ProductConversationRecord `json:"entries"`
}

type productConversationBinding struct {
	ConversationKey        string
	WorkspacePath          string
	ResourceID             string
	Title                  string
	Description            string
	AuthoritativeSessionID string
}

type productConversationRegistryStore struct {
	read  func(context.Context, string) (string, bool, error)
	write func(context.Context, string, string) error
	now   func() time.Time
	newID func() string
}

func defaultProductConversationRegistryStore() productConversationRegistryStore {
	return productConversationRegistryStore{
		read:  readFileFromWorkspace,
		write: writeRawFileToWorkspace,
		now:   time.Now,
		newID: uuid.NewString,
	}
}

func productConversationRegistryPath(userID string) string {
	return filepath.ToSlash(filepath.Join(chatHistoryRoot(userID), productConversationRegistryFile))
}

func productConversationRegistryEntryKey(profileID, conversationKey string) string {
	return strings.TrimSpace(profileID) + "/" + strings.TrimSpace(conversationKey)
}

func productConversationRegistryMutex(path string) *sync.Mutex {
	var hash uint64 = 1469598103934665603
	for i := 0; i < len(path); i++ {
		hash ^= uint64(path[i])
		hash *= 1099511628211
	}
	return &productConversationLocks[hash%uint64(len(productConversationLocks))]
}

func (store productConversationRegistryStore) resolveOrCreate(
	ctx context.Context,
	userID string,
	profile agentprofiles.Profile,
	binding productConversationBinding,
	preferredSessionID string,
) (ProductConversationRecord, error) {
	path := productConversationRegistryPath(userID)
	mutex := productConversationRegistryMutex(path)
	mutex.Lock()
	defer mutex.Unlock()

	document := productConversationRegistryDocument{
		Version: productConversationRegistryVersion,
		Entries: map[string]ProductConversationRecord{},
	}
	if raw, exists, err := store.read(ctx, path); err != nil {
		return ProductConversationRecord{}, fmt.Errorf("read product conversation registry: %w", err)
	} else if exists && strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &document); err != nil {
			return ProductConversationRecord{}, fmt.Errorf("decode product conversation registry: %w", err)
		}
		if document.Entries == nil {
			document.Entries = map[string]ProductConversationRecord{}
		}
	}

	entryKey := productConversationRegistryEntryKey(profile.ID, binding.ConversationKey)
	now := store.now().UTC().Format(time.RFC3339Nano)
	if existing, ok := document.Entries[entryKey]; ok {
		if strings.TrimSpace(existing.ConversationID) == "" || strings.TrimSpace(existing.SessionID) == "" {
			return ProductConversationRecord{}, fmt.Errorf("product conversation %q has an incomplete durable identity", entryKey)
		}
		changed := existing.ProfileVersion != profile.Version ||
			existing.WorkspacePath != binding.WorkspacePath ||
			existing.ResourceID != binding.ResourceID ||
			existing.Title != binding.Title ||
			existing.Description != binding.Description
		existing.ProfileVersion = profile.Version
		existing.WorkspacePath = binding.WorkspacePath
		existing.ResourceID = binding.ResourceID
		existing.Title = binding.Title
		existing.Description = binding.Description
		if changed {
			existing.UpdatedAt = now
			document.Entries[entryKey] = existing
			if err := store.writeDocument(ctx, path, document); err != nil {
				return ProductConversationRecord{}, err
			}
		}
		return existing, nil
	}

	sessionID := strings.TrimSpace(binding.AuthoritativeSessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(preferredSessionID)
	}
	if sessionID == "" {
		sessionID = "product-" + store.newID()
	}
	record := ProductConversationRecord{
		ConversationID:  "conversation-" + store.newID(),
		ConversationKey: binding.ConversationKey,
		ProfileID:       profile.ID,
		ProfileVersion:  profile.Version,
		SessionID:       sessionID,
		WorkspacePath:   binding.WorkspacePath,
		ResourceID:      binding.ResourceID,
		Title:           binding.Title,
		Description:     binding.Description,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	document.Version = productConversationRegistryVersion
	document.Entries[entryKey] = record
	if err := store.writeDocument(ctx, path, document); err != nil {
		return ProductConversationRecord{}, err
	}
	return record, nil
}

func (store productConversationRegistryStore) writeDocument(ctx context.Context, path string, document productConversationRegistryDocument) error {
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode product conversation registry: %w", err)
	}
	if err := store.write(ctx, path, string(data)+"\n"); err != nil {
		return fmt.Errorf("write product conversation registry: %w", err)
	}
	return nil
}

type productProjectManifest struct {
	SchemaVersion int    `json:"schema_version"`
	Product       string `json:"product"`
	ID            string `json:"id"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	SessionID     string `json:"session_id"`
}

type productProjectStore struct {
	listPaths func(context.Context, string) ([]string, bool, error)
	read      func(context.Context, string) (string, bool, error)
}

func defaultProductProjectStore() productProjectStore {
	return productProjectStore{
		listPaths: func(ctx context.Context, root string) ([]string, bool, error) {
			listing, exists, err := listWorkspaceFolder(ctx, root, 3)
			if err != nil || !exists {
				return nil, exists, err
			}
			paths := []string{}
			collectWorkspaceFilePaths(listing, &paths)
			return paths, true, nil
		},
		read: readFileFromWorkspace,
	}
}

func resolveProductProjectBinding(ctx context.Context, userID string, profile agentprofiles.Profile, projectID string) (productConversationBinding, error) {
	return resolveProductProjectBindingWithStore(ctx, userID, profile, projectID, defaultProductProjectStore())
}

func resolveProductProjectBindingWithStore(
	ctx context.Context,
	userID string,
	profile agentprofiles.Profile,
	projectID string,
	store productProjectStore,
) (productConversationBinding, error) {
	projectsRoot, err := cleanAgentProfileWorkspace(profile.Runtime.Workspace.ProjectsRoot, userID)
	if err != nil {
		return productConversationBinding{}, fmt.Errorf("invalid product projects root: %w", err)
	}
	runtimeRoot := agentProfileRuntimeWorkspace(userID, projectsRoot)
	paths, exists, err := store.listPaths(ctx, runtimeRoot)
	if err != nil {
		return productConversationBinding{}, fmt.Errorf("list product projects: %w", err)
	}
	if !exists {
		return productConversationBinding{}, fmt.Errorf("product projects root does not exist")
	}
	sort.Strings(paths)

	var matched *productConversationBinding
	rootPrefix := strings.TrimSuffix(filepath.ToSlash(runtimeRoot), "/") + "/"
	seenCandidatePaths := make(map[string]struct{}, len(paths))
	for _, candidate := range paths {
		candidate = filepath.ToSlash(strings.TrimSpace(candidate))
		if !strings.HasPrefix(candidate, rootPrefix) || !strings.HasSuffix(candidate, "/product.json") {
			continue
		}
		// The workspace listing can return a folder both nested under its root
		// and as a top-level item, which repeats the same file path. That is one
		// manifest, not a duplicate project id. Different paths with the same id
		// remain a hard error below.
		if _, seen := seenCandidatePaths[candidate]; seen {
			continue
		}
		seenCandidatePaths[candidate] = struct{}{}
		raw, found, readErr := store.read(ctx, candidate)
		if readErr != nil {
			return productConversationBinding{}, fmt.Errorf("read project manifest %s: %w", candidate, readErr)
		}
		if !found {
			continue
		}
		var manifest productProjectManifest
		if json.Unmarshal([]byte(raw), &manifest) != nil ||
			strings.TrimSpace(manifest.Product) != profile.ID ||
			strings.TrimSpace(manifest.ID) != projectID {
			continue
		}
		if strings.TrimSpace(manifest.Title) == "" || strings.TrimSpace(manifest.SessionID) == "" {
			return productConversationBinding{}, fmt.Errorf("project %q has an incomplete product manifest", projectID)
		}
		if matched != nil {
			return productConversationBinding{}, fmt.Errorf("project id %q is duplicated", projectID)
		}
		workspacePath := filepath.ToSlash(filepath.Dir(candidate))
		matched = &productConversationBinding{
			ConversationKey:        projectID,
			WorkspacePath:          workspacePath,
			ResourceID:             projectID,
			Title:                  strings.TrimSpace(manifest.Title),
			Description:            strings.TrimSpace(manifest.Description),
			AuthoritativeSessionID: strings.TrimSpace(manifest.SessionID),
		}
	}
	if matched == nil {
		return productConversationBinding{}, fmt.Errorf("project %q was not found", projectID)
	}
	return *matched, nil
}

func resolveProductConversationBinding(ctx context.Context, userID string, profile agentprofiles.Profile, requestedKey string) (productConversationBinding, error) {
	mode := strings.ToLower(strings.TrimSpace(profile.Runtime.Conversation.Mode))
	switch mode {
	case agentprofiles.ConversationModeSingleton:
		if key := strings.TrimSpace(requestedKey); key != "" && key != "main" {
			return productConversationBinding{}, fmt.Errorf("singleton product does not accept conversation_key %q", key)
		}
		workspacePath, err := cleanAgentProfileWorkspace(profile.Runtime.Workspace.Root, userID)
		if err != nil {
			return productConversationBinding{}, fmt.Errorf("invalid product workspace root: %w", err)
		}
		return productConversationBinding{
			ConversationKey: "main",
			WorkspacePath:   workspacePath,
			Title:           profile.Name,
		}, nil
	case agentprofiles.ConversationModeKeyed:
		key := strings.TrimSpace(requestedKey)
		if !productConversationKeyPattern.MatchString(key) {
			return productConversationBinding{}, fmt.Errorf("conversation_key is required and must be a stable product resource id")
		}
		if strings.ToLower(strings.TrimSpace(profile.Runtime.Conversation.KeyType)) != agentprofiles.ConversationKeyTypeProject {
			return productConversationBinding{}, fmt.Errorf("unsupported product conversation key type")
		}
		return resolveProductProjectBinding(ctx, userID, profile, key)
	default:
		return productConversationBinding{}, fmt.Errorf("agent profile does not declare a product conversation policy")
	}
}
