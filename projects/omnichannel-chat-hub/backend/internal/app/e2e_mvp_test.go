package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestMVPE2EVerificationSuite(t *testing.T) {
	var workerSawPayload map[string]any
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/sessions/e2e-session/messages" {
			t.Fatalf("unexpected worker request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer e2e-secret" {
			t.Fatalf("worker auth header = %q, want bearer token", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&workerSawPayload); err != nil {
			t.Fatalf("decode worker payload: %v", err)
		}
		writeJSON(w, http.StatusOK, map[string]string{"external_message_id": "worker-msg-123", "status": "sent"})
	}))
	defer worker.Close()

	dbPath := filepath.Join(t.TempDir(), "e2e.db")
	srv, err := NewServer(Config{DatabaseURL: "file:" + dbPath, InternalWebhookToken: "e2e-secret", BaileysWorkerURL: worker.URL, EnableDevWebhooks: true})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	defer srv.Close()
	handler := srv.Handler()

	token := e2eLogin(t, handler)

	devInbound := map[string]any{
		"provider":                 "whatsapp_baileys",
		"channel_external_id":      "e2e-session",
		"channel_display_name":     "E2E WhatsApp",
		"contact_external_id":      "15551234567@s.whatsapp.net",
		"contact_display_name":     "E2E Customer",
		"conversation_external_id": "e2e-session:15551234567@s.whatsapp.net",
		"message_external_id":      "e2e-inbound-1",
		"body":                     "hello from e2e inbound",
	}
	e2eJSON(t, handler, http.MethodPost, "/api/v1/webhooks/dev/inbound", token, devInbound, http.StatusCreated, nil)

	var inbox struct {
		Conversations []struct {
			ID          string `json:"id"`
			Status      string `json:"status"`
			AssignedTo  string `json:"assigned_to"`
			UnreadCount int    `json:"unread_count"`
		} `json:"conversations"`
		Total int `json:"total"`
	}
	e2eJSON(t, handler, http.MethodGet, "/api/v1/conversations", token, nil, http.StatusOK, &inbox)
	if inbox.Total != 1 || len(inbox.Conversations) != 1 {
		t.Fatalf("inbox conversations total=%d len=%d, want one conversation", inbox.Total, len(inbox.Conversations))
	}
	conversationID := inbox.Conversations[0].ID
	if inbox.Conversations[0].Status != "open" || inbox.Conversations[0].UnreadCount != 1 {
		t.Fatalf("inbox conversation = %+v, want open with one unread", inbox.Conversations[0])
	}

	var messages struct {
		Messages []struct {
			Direction string `json:"direction"`
			Body      string `json:"body"`
		} `json:"messages"`
	}
	e2eJSON(t, handler, http.MethodGet, "/api/v1/conversations/"+conversationID+"/messages", token, nil, http.StatusOK, &messages)
	if len(messages.Messages) != 1 || messages.Messages[0].Direction != "inbound" || messages.Messages[0].Body != "hello from e2e inbound" {
		t.Fatalf("timeline messages = %+v, want inbound e2e body", messages.Messages)
	}

	assignment := "owner@example.com"
	status := "pending"
	var patched struct {
		Status     string `json:"status"`
		AssignedTo string `json:"assigned_to"`
	}
	e2eJSON(t, handler, http.MethodPatch, "/api/v1/conversations/"+conversationID, token, map[string]any{"status": status, "assigned_to": assignment, "mark_read": true}, http.StatusOK, &patched)
	if patched.Status != status || patched.AssignedTo != assignment {
		t.Fatalf("patched conversation = %+v, want status/assignment updated", patched)
	}

	var sent struct {
		Status            string `json:"status"`
		ExternalMessageID string `json:"external_message_id"`
	}
	e2eJSON(t, handler, http.MethodPost, "/api/v1/conversations/"+conversationID+"/messages", token, map[string]any{"type": "text", "body": "thanks from support"}, http.StatusOK, &sent)
	if sent.Status != "sent" || sent.ExternalMessageID != "worker-msg-123" {
		t.Fatalf("send response = %+v, want sent worker id", sent)
	}
	if workerSawPayload["chat_id"] != "15551234567@s.whatsapp.net" || workerSawPayload["body"] != "thanks from support" {
		t.Fatalf("worker payload = %+v, want chat id and body", workerSawPayload)
	}

	var started struct {
		Session struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			QRCode  string `json:"qr_code"`
			PollURL string `json:"poll_url"`
		} `json:"session"`
	}
	e2eJSON(t, handler, http.MethodPost, "/api/v1/channels/whatsapp-baileys/session/start", token, map[string]any{"display_name": "Mock WhatsApp"}, http.StatusCreated, &started)
	if started.Session.ID == "" || started.Session.Status != "qr_pending" || !strings.HasPrefix(started.Session.QRCode, "mock-qr-") || started.Session.PollURL == "" {
		t.Fatalf("started session = %+v, want mocked qr_pending status", started.Session)
	}

	var polled struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		QRCode  string `json:"qr_code"`
		PollURL string `json:"poll_url"`
	}
	e2eJSON(t, handler, http.MethodGet, started.Session.PollURL, token, nil, http.StatusOK, &polled)
	if polled.ID != started.Session.ID || polled.Status != "qr_pending" || polled.QRCode != started.Session.QRCode {
		t.Fatalf("polled session = %+v, want started session status", polled)
	}
}

func e2eLogin(t *testing.T, handler http.Handler) string {
	t.Helper()
	var login struct {
		Token string `json:"token"`
		User  struct {
			Email string `json:"email"`
			Role  string `json:"role"`
		} `json:"user"`
	}
	e2eJSON(t, handler, http.MethodPost, "/api/v1/auth/login", "", map[string]string{"email": defaultOwnerEmail, "password": defaultOwnerPassword}, http.StatusOK, &login)
	if login.Token == "" || login.User.Email != defaultOwnerEmail || login.User.Role != roleOwner {
		t.Fatalf("login response = %+v, want owner token", login)
	}
	return login.Token
}

func e2eJSON(t *testing.T, handler http.Handler, method, path, token string, payload any, wantStatus int, out any) {
	t.Helper()
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			t.Fatalf("encode payload: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &body)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s status = %d, want %d body=%s", method, path, rec.Code, wantStatus, rec.Body.String())
	}
	if out != nil {
		if err := json.NewDecoder(rec.Body).Decode(out); err != nil {
			t.Fatalf("decode %s %s response: %v body=%s", method, path, err, rec.Body.String())
		}
	}
}
