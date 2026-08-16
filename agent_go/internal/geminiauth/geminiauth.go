// Package geminiauth validates Gemini API keys used to scope a Video Studio
// project's Pi CLI (Gemini) turns to one person's own key instead of
// whichever key happens to be configured on the server.
package geminiauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// modelsEndpoint lists models with the given key. It is the standard
// lightweight way to check a Gemini API key: it costs no generation tokens
// and Google rejects an invalid key with 400/403 before any model runs.
const modelsEndpoint = "https://generativelanguage.googleapis.com/v1beta/models"

// ValidateAPIKey reports whether Google accepts key for the Gemini API.
func ValidateAPIKey(parent context.Context, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("Gemini API key is required")
	}

	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()

	endpoint := modelsEndpoint + "?key=" + url.QueryEscape(key) + "&pageSize=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("could not build the Gemini validation request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("Gemini timed out while checking this API key")
		}
		return fmt.Errorf("could not reach Gemini to check this API key: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	switch resp.StatusCode {
	case http.StatusBadRequest, http.StatusForbidden, http.StatusUnauthorized:
		return fmt.Errorf("Gemini rejected this API key")
	default:
		detail := strings.TrimSpace(string(body))
		if detail == "" {
			return fmt.Errorf("Gemini could not verify this API key (status %d)", resp.StatusCode)
		}
		return fmt.Errorf("Gemini could not verify this API key (status %d): %s", resp.StatusCode, firstLine(detail))
	}
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
