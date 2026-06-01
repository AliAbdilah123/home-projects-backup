package app

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const whatsappBaileysProvider = "whatsapp_baileys"

type baileysInternalEvent struct {
	EventID   string         `json:"event_id"`
	EventType string         `json:"event_type"`
	SessionID string         `json:"session_id"`
	Message   baileysMessage `json:"message"`
}

type baileysMessage struct {
	ID        string `json:"id"`
	ChatID    string `json:"chat_id"`
	From      string `json:"from"`
	FromMe    bool   `json:"from_me"`
	PushName  string `json:"push_name"`
	Timestamp string `json:"timestamp"`
	Text      string `json:"text"`
}

type inboundProcessResult struct {
	Status    string `json:"status"`
	Duplicate bool   `json:"duplicate"`
	MessageID string `json:"message_id,omitempty"`
}

func (s *Server) handleBaileysInternalWebhook(w http.ResponseWriter, r *http.Request) {
	if !s.validInternalBearer(r.Header.Get("Authorization")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	defer r.Body.Close()
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		_ = s.repo.LogAudit(r.Context(), "webhook.invalid_payload", map[string]any{"provider": whatsappBaileysProvider, "error": "invalid json"})
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	var event baileysInternalEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		_ = s.repo.LogAudit(r.Context(), "webhook.invalid_payload", map[string]any{"provider": whatsappBaileysProvider, "error": err.Error()})
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	if err := validateBaileysEvent(event); err != nil {
		_ = s.repo.LogAudit(r.Context(), "webhook.invalid_payload", map[string]any{"provider": whatsappBaileysProvider, "event_id": event.EventID, "error": err.Error()})
		http.Error(w, "invalid payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	result, err := s.repo.ProcessBaileysInboundEvent(r.Context(), event, string(raw))
	if err != nil {
		http.Error(w, "failed to process inbound event", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) validInternalBearer(header string) bool {
	token, ok := bearerToken(header)
	if !ok || s.internalWebhookToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.internalWebhookToken)) == 1
}

func validateBaileysEvent(event baileysInternalEvent) error {
	if strings.TrimSpace(event.SessionID) == "" {
		return errors.New("session_id is required")
	}
	if strings.TrimSpace(event.EventType) == "" {
		return errors.New("event_type is required")
	}
	if strings.TrimSpace(event.Message.ID) == "" {
		return errors.New("message.id is required")
	}
	if strings.TrimSpace(event.Message.ChatID) == "" {
		return errors.New("message.chat_id is required")
	}
	if strings.TrimSpace(event.Message.Text) == "" {
		return errors.New("message.text is required")
	}
	if strings.TrimSpace(event.Message.Timestamp) != "" {
		if _, err := time.Parse(time.RFC3339, event.Message.Timestamp); err != nil {
			return fmt.Errorf("message.timestamp must be RFC3339: %w", err)
		}
	}
	return nil
}

func (r *Repository) ProcessBaileysInboundEvent(ctx context.Context, event baileysInternalEvent, rawPayload string) (inboundProcessResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return inboundProcessResult{}, fmt.Errorf("begin inbound event tx: %w", err)
	}
	defer tx.Rollback()

	messageID := strings.TrimSpace(event.Message.ID)
	webhookID, err := newEntityID(ctx, tx)
	if err != nil {
		return inboundProcessResult{}, err
	}
	created, err := createWebhookEventIfNotExistsTx(ctx, tx, WebhookEvent{
		ID:         webhookID,
		Provider:   whatsappBaileysProvider,
		ExternalID: messageID,
		EventType:  event.EventType,
		Payload:    rawPayload,
	})
	if err != nil {
		return inboundProcessResult{}, err
	}
	if !created {
		if err := tx.Commit(); err != nil {
			return inboundProcessResult{}, fmt.Errorf("commit duplicate inbound tx: %w", err)
		}
		return inboundProcessResult{Status: "duplicate", Duplicate: true, MessageID: messageID}, nil
	}

	sessionID := strings.TrimSpace(event.SessionID)
	chatID := strings.TrimSpace(event.Message.ChatID)
	contactExternalID := strings.TrimSpace(event.Message.From)
	if contactExternalID == "" || event.Message.FromMe {
		contactExternalID = chatID
	}
	conversationExternalID := sessionID + ":" + chatID
	sentAt := strings.TrimSpace(event.Message.Timestamp)
	if sentAt == "" {
		sentAt = time.Now().UTC().Format(time.RFC3339)
	}
	direction := "inbound"
	if event.Message.FromMe {
		direction = "outbound"
	}

	channelID, err := newEntityID(ctx, tx)
	if err != nil {
		return inboundProcessResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO channels (id, provider, external_id, display_name, status)
		VALUES (?, ?, ?, ?, 'active')
		ON CONFLICT(provider, external_id) DO UPDATE SET
			display_name = excluded.display_name,
			status = 'active',
			updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	`, "ch_"+channelID, whatsappBaileysProvider, sessionID, "WhatsApp Baileys "+sessionID); err != nil {
		return inboundProcessResult{}, fmt.Errorf("upsert channel: %w", err)
	}
	channelRowID, err := lookupID(ctx, tx, "channels", whatsappBaileysProvider, sessionID)
	if err != nil {
		return inboundProcessResult{}, err
	}

	contactID, err := newEntityID(ctx, tx)
	if err != nil {
		return inboundProcessResult{}, err
	}
	name := strings.TrimSpace(event.Message.PushName)
	if name == "" {
		name = contactExternalID
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO contacts (id, provider, external_id, display_name, phone, email)
		VALUES (?, ?, ?, ?, ?, NULL)
		ON CONFLICT(provider, external_id) DO UPDATE SET
			display_name = excluded.display_name,
			phone = COALESCE(excluded.phone, contacts.phone),
			updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	`, "ct_"+contactID, whatsappBaileysProvider, contactExternalID, name, nullIfEmpty(phoneFromJID(contactExternalID))); err != nil {
		return inboundProcessResult{}, fmt.Errorf("upsert contact: %w", err)
	}
	contactRowID, err := lookupID(ctx, tx, "contacts", whatsappBaileysProvider, contactExternalID)
	if err != nil {
		return inboundProcessResult{}, err
	}

	conversationID, err := newEntityID(ctx, tx)
	if err != nil {
		return inboundProcessResult{}, err
	}
	unreadIncrement := 0
	if direction == "inbound" {
		unreadIncrement = 1
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO conversations (id, provider, external_id, channel_id, contact_id, status, unread_count, last_message_at)
		VALUES (?, ?, ?, ?, ?, 'open', ?, ?)
		ON CONFLICT(provider, external_id) DO UPDATE SET
			channel_id = excluded.channel_id,
			contact_id = excluded.contact_id,
			last_message_at = excluded.last_message_at,
			unread_count = conversations.unread_count + ?,
			updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	`, "cv_"+conversationID, whatsappBaileysProvider, conversationExternalID, channelRowID, contactRowID, unreadIncrement, sentAt, unreadIncrement); err != nil {
		return inboundProcessResult{}, fmt.Errorf("upsert conversation: %w", err)
	}
	conversationRowID, err := lookupID(ctx, tx, "conversations", whatsappBaileysProvider, conversationExternalID)
	if err != nil {
		return inboundProcessResult{}, err
	}

	messageRowID, err := newEntityID(ctx, tx)
	if err != nil {
		return inboundProcessResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO messages (id, provider, external_id, conversation_id, direction, body, sent_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "msg_"+messageRowID, whatsappBaileysProvider, messageID, conversationRowID, direction, event.Message.Text, sentAt); err != nil {
		return inboundProcessResult{}, fmt.Errorf("insert message: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE webhook_events SET processed_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')) WHERE provider = ? AND external_id = ?`, whatsappBaileysProvider, messageID); err != nil {
		return inboundProcessResult{}, fmt.Errorf("mark webhook processed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return inboundProcessResult{}, fmt.Errorf("commit inbound event tx: %w", err)
	}
	return inboundProcessResult{Status: "processed", Duplicate: false, MessageID: messageID}, nil
}

func createWebhookEventIfNotExistsTx(ctx context.Context, tx *sql.Tx, e WebhookEvent) (bool, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO webhook_events (id, provider, external_id, event_type, payload)
		VALUES (?, ?, ?, ?, ?)
	`, e.ID, e.Provider, e.ExternalID, e.EventType, e.Payload)
	if err != nil {
		return false, fmt.Errorf("create webhook event: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("create webhook event rows affected: %w", err)
	}
	return affected == 1, nil
}

func (r *Repository) LogAudit(ctx context.Context, action string, metadata map[string]any) error {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal audit metadata: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO audit_logs (id, actor_type, actor_id, action, target_type, target_id, metadata)
		VALUES (lower(hex(randomblob(16))), 'system', NULL, ?, 'webhook_event', NULL, ?)
	`, action, string(metadataJSON))
	if err != nil {
		return fmt.Errorf("log audit: %w", err)
	}
	return nil
}

func newEntityID(ctx context.Context, tx *sql.Tx) (string, error) {
	var id string
	if err := tx.QueryRowContext(ctx, `SELECT lower(hex(randomblob(16)))`).Scan(&id); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return id, nil
}

func lookupID(ctx context.Context, tx *sql.Tx, table, provider, externalID string) (string, error) {
	var id string
	query := fmt.Sprintf(`SELECT id FROM %s WHERE provider = ? AND external_id = ?`, table)
	if err := tx.QueryRowContext(ctx, query, provider, externalID).Scan(&id); err != nil {
		return "", fmt.Errorf("lookup %s id: %w", table, err)
	}
	return id, nil
}

func phoneFromJID(jid string) string {
	base := strings.SplitN(jid, "@", 2)[0]
	base = strings.TrimPrefix(base, "+")
	for _, r := range base {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return base
}
