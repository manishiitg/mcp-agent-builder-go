package events

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// Observer represents an event observer
type Observer struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	LastActivity time.Time `json:"last_activity"`
	Status       string    `json:"status"` // "active", "inactive"
	SessionID    string    `json:"session_id,omitempty"`
}

// ObserverManager manages observer lifecycle and registration
type ObserverManager struct {
	observers map[string]*Observer
	store     *EventStore
	mu        sync.RWMutex
}

// NewObserverManager creates a new observer manager
func NewObserverManager(store *EventStore) *ObserverManager {
	return &ObserverManager{
		observers: make(map[string]*Observer),
		store:     store,
	}
}

// RegisterObserver creates a new observer
func (om *ObserverManager) RegisterObserver(sessionID string) *Observer {
	om.mu.Lock()
	defer om.mu.Unlock()

	// Generate unique observer ID
	observerID := generateObserverID()

	observer := &Observer{
		ID:           observerID,
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		Status:       "active",
		SessionID:    sessionID,
	}

	om.observers[observerID] = observer

	// Initialize the observer in the event store
	om.store.InitializeObserver(observerID)

	return observer
}

// GetObserver retrieves an observer by ID
func (om *ObserverManager) GetObserver(observerID string) (*Observer, bool) {
	om.mu.RLock()
	defer om.mu.RUnlock()

	observer, exists := om.observers[observerID]
	if !exists {
		return nil, false
	}

	// Update last activity
	observer.LastActivity = time.Now()

	return observer, true
}

// RemoveObserver removes an observer and its events
func (om *ObserverManager) RemoveObserver(observerID string) bool {
	om.mu.Lock()
	defer om.mu.Unlock()

	if _, exists := om.observers[observerID]; !exists {
		return false
	}

	// Remove from observer manager
	delete(om.observers, observerID)

	// Remove from event store
	om.store.RemoveObserver(observerID)

	return true
}

// UpdateObserverActivity updates the last activity time for an observer
func (om *ObserverManager) UpdateObserverActivity(observerID string) bool {
	om.mu.Lock()
	defer om.mu.Unlock()

	observer, exists := om.observers[observerID]
	if !exists {
		return false
	}

	observer.LastActivity = time.Now()
	return true
}

// GetActiveObservers returns all active observers
func (om *ObserverManager) GetActiveObservers() []*Observer {
	om.mu.RLock()
	defer om.mu.RUnlock()

	activeObservers := make([]*Observer, 0)
	for _, observer := range om.observers {
		if observer.Status == "active" {
			activeObservers = append(activeObservers, observer)
		}
	}

	return activeObservers
}

// CleanupInactiveObservers removes observers that haven't been active recently
func (om *ObserverManager) CleanupInactiveObservers(maxInactiveTime time.Duration) int {
	om.mu.Lock()
	defer om.mu.Unlock()

	cutoff := time.Now().Add(-maxInactiveTime)
	removedCount := 0

	for observerID, observer := range om.observers {
		if observer.LastActivity.Before(cutoff) {
			// Remove from observer manager
			delete(om.observers, observerID)
			// Remove from event store
			om.store.RemoveObserver(observerID)
			removedCount++
		}
	}

	return removedCount
}

// GetObserverStats returns statistics about observers
func (om *ObserverManager) GetObserverStats() map[string]interface{} {
	om.mu.RLock()
	defer om.mu.RUnlock()

	activeCount := 0
	inactiveCount := 0

	for _, observer := range om.observers {
		if observer.Status == "active" {
			activeCount++
		} else {
			inactiveCount++
		}
	}

	return map[string]interface{}{
		"total_observers":    len(om.observers),
		"active_observers":   activeCount,
		"inactive_observers": inactiveCount,
	}
}

// generateObserverID generates a unique observer ID
func generateObserverID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return "observer_" + hex.EncodeToString(bytes)
}
