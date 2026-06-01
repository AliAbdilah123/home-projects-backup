package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newWebhookTestServer(t *testing.T) *Server {
	t.Helper()
	server, err := NewServer(Config{
		HTTPAddr:             ":0",
		DatabaseURL:          "file::memory:?cache=shared",
		InternalWebhookToken: "test-internal-token",
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server
}

func postInternalWebhook(server *Server, token string, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/whatsapp-baileys/internal", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	return rec
}

func TestBaileysInternalWebhookRequiresInternalBearerToken(t *testing.T) {
	server := newWebhookTestServer(t)

	payload := `{"event_id":"evt-auth","event_type":"messages.upsert","session_id":"session-a","message":{"id":"msg-auth","chat_id":"15551234567@s.whatsapp.net","from":"15551234567@s.whatsapp.net","from_me":false,"push_name":"Alice","timestamp":"2026-05-31T12:00:00Z","text":"hello"}}`
	for _, token := range []string{"", "wrong-token"} {
		rec := postInternalWebhook(server, token, payload)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status for token %q = %d, want %d", token, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestBaileysInternalWebhookNormalizesAndDedupeInboundMessages(t *testing.T) {
	server := newWebhookTestServer(t)

	payload := `{"event_id":"evt-1","event_type":"messages.upsert","session_id":"session-a","message":{"id":"wamid.1","chat_id":"15551234567@s.whatsapp.net","from":"15551234567@s.whatsapp.net","from_me":false,"push_name":"Alice","timestamp":"2026-05-31T12:00:00Z","text":"hello from whatsapp"}}`
	rec := postInternalWebhook(server, "test-internal-token", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var first struct {
		Status    string `json:"status"`
		Duplicate bool   `json:"duplicate"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
		t.Fatalf("first response JSON error = %v", err)
	}
	if first.Status != "processed" || first.Duplicate {
		t.Fatalf("first response = %+v, want processed duplicate=false", first)
	}

	rec = postInternalWebhook(server, "test-internal-token", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("duplicate status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var second struct {
		Status    string `json:"status"`
		Duplicate bool   `json:"duplicate"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &second); err != nil {
		t.Fatalf("duplicate response JSON error = %v", err)
	}
	if second.Status != "duplicate" || !second.Duplicate {
		t.Fatalf("duplicate response = %+v, want duplicate=true", second)
	}

	assertTableCount(t, server, "webhook_events", 1)
	assertTableCount(t, server, "contacts", 1)
	assertTableCount(t, server, "conversations", 1)
	assertTableCount(t, server, "messages", 1)

	var contactName, phone, messageBody string
	if err := server.db.QueryRow(`SELECT display_name, phone FROM contacts WHERE provider = 'whatsapp_baileys' AND external_id = ?`, "15551234567@s.whatsapp.net").Scan(&contactName, &phone); err != nil {
		t.Fatalf("query contact: %v", err)
	}
	if contactName != "Alice" || phone != "15551234567" {
		t.Fatalf("contact = name %q phone %q, want Alice/15551234567", contactName, phone)
	}
	if err := server.db.QueryRow(`SELECT body FROM messages WHERE provider = 'whatsapp_baileys' AND external_id = ?`, "wamid.1").Scan(&messageBody); err != nil {
		t.Fatalf("query message: %v", err)
	}
	if messageBody != "hello from whatsapp" {
		t.Fatalf("message body = %q", messageBody)
	}

	var unread int
	var lastMessageAt string
	if err := server.db.QueryRow(`SELECT unread_count, last_message_at FROM conversations WHERE provider = 'whatsapp_baileys' AND external_id = ?`, "session-a:15551234567@s.whatsapp.net").Scan(&unread, &lastMessageAt); err != nil {
		t.Fatalf("query conversation: %v", err)
	}
	if unread != 1 || lastMessageAt != "2026-05-31T12:00:00Z" {
		t.Fatalf("conversation unread=%d last_message_at=%q, want 1 and timestamp", unread, lastMessageAt)
	}
}

func TestBaileysInternalWebhookLogsInvalidPayloads(t *testing.T) {
	server := newWebhookTestServer(t)

	rec := postInternalWebhook(server, "test-internal-token", `{"event_id":"evt-invalid","event_type":"messages.upsert","session_id":"session-a","message":{"chat_id":"15551234567@s.whatsapp.net","text":"missing message id"}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var action, metadata string
	if err := server.db.QueryRow(`SELECT action, metadata FROM audit_logs WHERE action = 'webhook.invalid_payload'`).Scan(&action, &metadata); err != nil {
		t.Fatalf("expected invalid payload audit log: %v", err)
	}
	if action != "webhook.invalid_payload" || metadata == "" {
		t.Fatalf("audit log action=%q metadata=%q", action, metadata)
	}
	assertTableCount(t, server, "messages", 0)
}

func assertTableCount(t *testing.T, server *Server, table string, want int) {
	t.Helper()
	var got int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
	}
}
