package server

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"
)

// productInteractionTracker remembers when each user last used each product,
// which is what the product-schedule quiet rule needs ("do not start a
// check-in while the family is in the middle of using the app").
//
// Why its own record, and not the conversation's timestamps: a scheduled run
// writes into the very same conversation, so "last message time" would count
// the check-in's own turns as family activity and the rule would defer
// itself. Only the product chat entry points a person drives stamp this.
//
// In memory for the hot path, mirrored to a small per-user file so a
// restart does not make an active family look idle for a year (the old
// behavior: every due check-in fired on the first tick after launch).
type productInteractionTracker struct {
	mu        sync.Mutex
	last      map[string]time.Time // userID + "\x1f" + product -> last interaction
	written   map[string]time.Time // last persisted value per key
	loaded    map[string]bool      // user ids whose file has been read
	now       func() time.Time
	readFile  func(context.Context, string) (string, bool, error)
	writeFile func(context.Context, string, string) error
}

const (
	productInteractionsFile = "product-interactions.json"
	// productInteractionWriteInterval bounds how often a busy conversation
	// rewrites the file; the in-memory value is always current.
	productInteractionWriteInterval = 60 * time.Second
	// productInteractionNever is what "never used" reads as: long enough
	// that no quiet rule can hold a run back.
	productInteractionNever = 365 * 24 * time.Hour
)

func newProductInteractionTracker() *productInteractionTracker {
	return &productInteractionTracker{
		last:      map[string]time.Time{},
		written:   map[string]time.Time{},
		loaded:    map[string]bool{},
		now:       time.Now,
		readFile:  readFileFromWorkspace,
		writeFile: writeFileToWorkspace,
	}
}

func productInteractionKey(userID, product string) string {
	return strings.TrimSpace(userID) + "\x1f" + strings.ToLower(strings.TrimSpace(product))
}

func productInteractionsPath(userID string) string {
	return chatHistoryRoot(userID) + "/" + productInteractionsFile
}

// Note records that a person just used product as userID. Cheap; the file
// write is deferred and rate-limited.
func (t *productInteractionTracker) Note(ctx context.Context, userID, product string) {
	if t == nil || strings.TrimSpace(userID) == "" || strings.TrimSpace(product) == "" {
		return
	}
	now := t.now()
	key := productInteractionKey(userID, product)
	t.mu.Lock()
	t.ensureLoadedLocked(ctx, userID)
	t.last[key] = now
	due := t.written[key].IsZero() || now.Sub(t.written[key]) >= productInteractionWriteInterval
	if due {
		t.written[key] = now
	}
	snapshot := t.snapshotLocked(userID)
	t.mu.Unlock()
	if due {
		t.persist(userID, snapshot)
	}
}

// SinceInteractive is how long ago userID last used product; a user with no
// record reads as idle for productInteractionNever.
func (t *productInteractionTracker) SinceInteractive(ctx context.Context, userID, product string) time.Duration {
	if t == nil {
		return productInteractionNever
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ensureLoadedLocked(ctx, userID)
	last, ok := t.last[productInteractionKey(userID, product)]
	if !ok || last.IsZero() {
		return productInteractionNever
	}
	since := t.now().Sub(last)
	if since < 0 {
		return 0
	}
	return since
}

// ensureLoadedLocked reads the user's persisted record once; memory wins
// over the file for any key already stamped in this process.
func (t *productInteractionTracker) ensureLoadedLocked(ctx context.Context, userID string) {
	if t.loaded[userID] {
		return
	}
	t.loaded[userID] = true
	if t.readFile == nil {
		return
	}
	content, exists, err := t.readFile(ctx, productInteractionsPath(userID))
	if err != nil || !exists || strings.TrimSpace(content) == "" {
		return
	}
	var stored map[string]string
	if json.Unmarshal([]byte(content), &stored) != nil {
		return
	}
	for product, stamp := range stored {
		at, err := time.Parse(time.RFC3339, stamp)
		if err != nil {
			continue
		}
		key := productInteractionKey(userID, product)
		if existing, ok := t.last[key]; !ok || at.After(existing) {
			t.last[key] = at
			t.written[key] = at
		}
	}
}

func (t *productInteractionTracker) snapshotLocked(userID string) map[string]string {
	prefix := strings.TrimSpace(userID) + "\x1f"
	out := map[string]string{}
	for key, at := range t.last {
		if strings.HasPrefix(key, prefix) {
			out[strings.TrimPrefix(key, prefix)] = at.UTC().Format(time.RFC3339)
		}
	}
	return out
}

func (t *productInteractionTracker) persist(userID string, snapshot map[string]string) {
	if t.writeFile == nil {
		return
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return
	}
	// Off the request path: a slow workspace write must not delay a turn.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := t.writeFile(ctx, productInteractionsPath(userID), string(data)); err != nil {
			log.Printf("[PRODUCT-SCHEDULE] could not persist interaction record for %s: %v", userID, err)
		}
	}()
}
