package app

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type AuditEntry struct {
	ActorType  string
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Metadata   map[string]any
}

var bearerTokenPattern = regexp.MustCompile(`(?i)(bearer\s+)([^\s"']+)`)

func (r *Repository) LogAuditEvent(ctx context.Context, entry AuditEntry) error {
	if strings.TrimSpace(entry.ActorType) == "" {
		entry.ActorType = "system"
	}
	if strings.TrimSpace(entry.TargetType) == "" {
		entry.TargetType = "system"
	}
	metadataJSON, err := json.Marshal(redactMap(entry.Metadata))
	if err != nil {
		return fmt.Errorf("marshal audit metadata: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO audit_logs (id, actor_type, actor_id, action, target_type, target_id, metadata)
		VALUES (lower(hex(randomblob(16))), ?, ?, ?, ?, ?, ?)
	`, entry.ActorType, nullIfEmpty(strings.TrimSpace(entry.ActorID)), entry.Action, entry.TargetType, nullIfEmpty(strings.TrimSpace(entry.TargetID)), string(metadataJSON))
	if err != nil {
		return fmt.Errorf("log audit: %w", err)
	}
	return nil
}

func redactMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = redactValue(k, v)
	}
	return out
}

func redactValue(key string, value any) any {
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	if shouldRedactKey(lowerKey) {
		return "[REDACTED]"
	}
	switch typed := value.(type) {
	case string:
		return sanitizeString(typed)
	case map[string]any:
		return redactMap(typed)
	case []any:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, redactValue("", item))
		}
		return items
	default:
		return value
	}
}

func shouldRedactKey(key string) bool {
	if key == "" {
		return false
	}
	keywords := []string{"token", "secret", "password", "session_path", "cookie", "authorization", "auth_header"}
	for _, kw := range keywords {
		if strings.Contains(key, kw) {
			return true
		}
	}
	return false
}

func sanitizeString(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return value
	}
	if strings.Contains(strings.ToLower(trimmed), "session") && strings.Contains(trimmed, "/") {
		return "[REDACTED]"
	}
	if strings.Contains(trimmed, "Bearer ") || strings.Contains(trimmed, "bearer ") {
		trimmed = bearerTokenPattern.ReplaceAllString(trimmed, "$1[REDACTED]")
	}
	return trimmed
}

func auditActorFromContext(ctx context.Context) (actorType, actorID string) {
	if user, ok := userFromContext(ctx); ok {
		return "user", user.Email
	}
	return "system", ""
}
