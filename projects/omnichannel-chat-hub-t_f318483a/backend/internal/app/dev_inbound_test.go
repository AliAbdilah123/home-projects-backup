package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDevInboundWebhookDisabledByDefault(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	token := loginAsDefaultOwner(t, srv)

	payload := []byte(`{"channel_external_id":"dev-wa-1","contact_external_id":"15551234567","message_external_id":"msg-1","body":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/dev/inbound", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDevInboundWebhookCreatesNormalizedRecords(t *testing.T) {
	srv, err := NewServer(Config{HTTPAddr: ":0", DatabaseURL: "file::memory:?cache=shared", EnableDevWebhooks: true})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	defer srv.Close()
	token := loginAsDefaultOwner(t, srv)

	body, _ := json.Marshal(map[string]any{
		"channel_external_id":  "dev-wa-1",
		"channel_display_name": "Dev WhatsApp",
		"contact_external_id":  "15551234567",
		"contact_display_name": "Alice",
		"conversation_external_id": "chat-1",
		"message_external_id":  "msg-1",
		"body":                 "Hello from dev",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/dev/inbound", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var channelCount, contactCount, convCount, msgCount int
	if err := srv.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM channels WHERE provider='dev' AND external_id='dev-wa-1'`).Scan(&channelCount); err != nil {
		t.Fatalf("query channels: %v", err)
	}
	if err := srv.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM contacts WHERE provider='dev' AND external_id='15551234567'`).Scan(&contactCount); err != nil {
		t.Fatalf("query contacts: %v", err)
	}
	if err := srv.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM conversations WHERE provider='dev' AND external_id='chat-1'`).Scan(&convCount); err != nil {
		t.Fatalf("query conversations: %v", err)
	}
	if err := srv.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM messages WHERE provider='dev' AND external_id='msg-1'`).Scan(&msgCount); err != nil {
		t.Fatalf("query messages: %v", err)
	}

	if channelCount != 1 || contactCount != 1 || convCount != 1 || msgCount != 1 {
		t.Fatalf("expected all normalized records to be created once, got channels=%d contacts=%d conversations=%d messages=%d", channelCount, contactCount, convCount, msgCount)
	}
}
