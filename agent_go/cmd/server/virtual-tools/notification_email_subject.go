package virtualtools

import (
	"fmt"
	"strings"
)

func validateNotificationEmailSubject(args map[string]interface{}, expected bool, excluded []string) error {
	if !expected || containsNotificationChannel(excluded, "gmail") {
		return nil
	}
	subject, ok := args["email_subject"].(string)
	if !ok || strings.TrimSpace(subject) == "" {
		return fmt.Errorf("email_subject is required when Gmail delivery is expected. Supply a non-empty plain-text subject; summary_title is not a substitute. Nothing was sent")
	}
	if strings.ContainsAny(subject, "\r\n") || strings.Contains(subject, "=?") {
		return fmt.Errorf("email_subject must be a single plain-text line, not a MIME-encoded header. Nothing was sent")
	}
	return nil
}
