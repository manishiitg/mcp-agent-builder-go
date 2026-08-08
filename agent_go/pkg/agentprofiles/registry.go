package agentprofiles

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type Registry struct {
	mu           sync.RWMutex
	profiles     map[string]map[int]Profile
	factories    map[string]ToolFactory
	initializers map[string]RuntimeInitializer
}

func NewRegistry() *Registry {
	return &Registry{
		profiles:     make(map[string]map[int]Profile),
		factories:    make(map[string]ToolFactory),
		initializers: make(map[string]RuntimeInitializer),
	}
}

func (r *Registry) RegisterProfile(profile Profile) error {
	if r == nil {
		return fmt.Errorf("profile registry is nil")
	}
	if err := Validate(profile); err != nil {
		return err
	}
	profile = cloneProfile(profile)

	r.mu.Lock()
	defer r.mu.Unlock()
	versions := r.profiles[profile.ID]
	if versions == nil {
		versions = make(map[int]Profile)
		r.profiles[profile.ID] = versions
	}
	if _, exists := versions[profile.Version]; exists {
		return fmt.Errorf("profile %q version %d is already registered", profile.ID, profile.Version)
	}
	for _, existing := range versions {
		if existing.BuiltIn != profile.BuiltIn || existing.OwnerID != profile.OwnerID {
			return fmt.Errorf("profile %q ownership cannot change across versions", profile.ID)
		}
	}
	versions[profile.Version] = profile
	return nil
}

func (r *Registry) Resolve(id string, version int, userID string) (Profile, error) {
	if r == nil {
		return Profile{}, fmt.Errorf("profile registry is nil")
	}
	id = strings.TrimSpace(id)
	r.mu.RLock()
	defer r.mu.RUnlock()
	versions := r.profiles[id]
	if len(versions) == 0 {
		return Profile{}, fmt.Errorf("profile %q not found", id)
	}
	if version == 0 {
		for candidate := range versions {
			if candidate > version {
				version = candidate
			}
		}
	}
	profile, exists := versions[version]
	if !exists {
		return Profile{}, fmt.Errorf("profile %q version %d not found", id, version)
	}
	if !profile.BuiltIn && strings.TrimSpace(profile.OwnerID) != strings.TrimSpace(userID) {
		return Profile{}, fmt.Errorf("profile %q is not available to this user", id)
	}
	return cloneProfile(profile), nil
}

func (r *Registry) List(userID string) []Profile {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	profiles := make([]Profile, 0)
	for _, versions := range r.profiles {
		for _, profile := range versions {
			if profile.BuiltIn || profile.OwnerID == userID {
				profiles = append(profiles, cloneProfile(profile))
			}
		}
	}
	sort.Slice(profiles, func(i, j int) bool {
		if profiles[i].ID == profiles[j].ID {
			return profiles[i].Version < profiles[j].Version
		}
		return profiles[i].ID < profiles[j].ID
	})
	return profiles
}

func (r *Registry) RegisterToolFactory(id string, factory ToolFactory) error {
	if r == nil {
		return fmt.Errorf("profile registry is nil")
	}
	id = strings.TrimSpace(id)
	if !toolIDPattern.MatchString(id) {
		return fmt.Errorf("invalid tool factory id %q", id)
	}
	if factory == nil {
		return fmt.Errorf("tool factory %q is nil", id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[id]; exists {
		return fmt.Errorf("tool factory %q is already registered", id)
	}
	r.factories[id] = factory
	return nil
}

func (r *Registry) RegisterInitializer(profileID string, initializer RuntimeInitializer) error {
	if r == nil {
		return fmt.Errorf("profile registry is nil")
	}
	profileID = strings.TrimSpace(profileID)
	if !profileIDPattern.MatchString(profileID) {
		return fmt.Errorf("invalid profile id %q", profileID)
	}
	if initializer == nil {
		return fmt.Errorf("profile initializer %q is nil", profileID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.initializers[profileID]; exists {
		return fmt.Errorf("profile initializer %q is already registered", profileID)
	}
	r.initializers[profileID] = initializer
	return nil
}

func (r *Registry) Initialize(ctx context.Context, profileID string, runtime RuntimeContext) error {
	if r == nil {
		return fmt.Errorf("profile registry is nil")
	}
	r.mu.RLock()
	initializer := r.initializers[strings.TrimSpace(profileID)]
	r.mu.RUnlock()
	if initializer == nil {
		return nil
	}
	return initializer(ctx, runtime)
}

func (r *Registry) BuildTool(binding ToolBinding, runtime ToolRuntimeContext) (ToolSpec, error) {
	if r == nil {
		return ToolSpec{}, fmt.Errorf("profile registry is nil")
	}
	r.mu.RLock()
	factory := r.factories[strings.TrimSpace(binding.ID)]
	r.mu.RUnlock()
	if factory == nil {
		return ToolSpec{}, fmt.Errorf("tool factory %q is not registered", binding.ID)
	}
	config := append(json.RawMessage(nil), binding.Config...)
	return factory(runtime, config)
}

func cloneProfile(profile Profile) Profile {
	cloned := profile
	cloned.Skills = append([]string(nil), profile.Skills...)
	cloned.Tools = make([]ToolBinding, len(profile.Tools))
	for i, binding := range profile.Tools {
		cloned.Tools[i] = binding
		cloned.Tools[i].Config = append(json.RawMessage(nil), binding.Config...)
	}
	cloned.ToolPolicy.Enabled = append([]string(nil), profile.ToolPolicy.Enabled...)
	cloned.ToolPolicy.Disabled = append([]string(nil), profile.ToolPolicy.Disabled...)
	return cloned
}
