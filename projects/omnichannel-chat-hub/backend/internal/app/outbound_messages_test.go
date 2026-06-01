package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type sendMessageResponse struct {
	MessageID         string `json:"message_id"`
	Provider          string `json:"provider"`
	ExternalMessageID string `json:"external_message_id"`
	Status            string `json:"status"`
	Error             string `json:"error,omitempty"`
}

func TestPostConversationMessageSendsThroughWorkerAndPersistsStatus(t *testing.T) {
	var workerAuth string
	var workerBody struct {
		MessageID string `json:"message_id"`
		ChatID    string `json:"chat_id"`
		Type      string `json:"type"`
		Body      string `json:"body"`
	}
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/sessions/wa-1/messages" {
			t.Fatalf("worker request = %s %s", r.Method, r.URL.Path)
		}
		workerAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&workerBody); err != nil {
			t.Fatalf("decode worker body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"external_message_id": "wamid.worker-123", "status": "sent"})
	}))
	defer worker.Close()

	srv := newTestServerWithConfig(t, Config{HTTPAddr: ":0", DatabaseURL: "file::memory:?cache=shared", InternalWebhookToken: "worker-secret", BaileysWorkerURL: worker.URL})
	defer srv.Close()
	token := loginAsDefaultOwner(t, srv)
	seedConversationFixtures(t, srv)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/cv-open-newer/messages", bytes.NewBufferString(`{"type":"text","body":"Thanks for reaching out"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var out sendMessageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.MessageID == "" || !strings.HasPrefix(out.MessageID, "msg_") {
		t.Fatalf("message_id=%q, want generated msg_ id", out.MessageID)
	}
	if out.Provider != "whatsapp_baileys" || out.ExternalMessageID != "wamid.worker-123" || out.Status != "sent" {
		t.Fatalf("unexpected response: %+v", out)
	}
	if workerAuth != "Bearer worker-secret" {
		t.Fatalf("worker auth=%q", workerAuth)
	}
	if workerBody.MessageID != out.MessageID || workerBody.ChatID != "c1" || workerBody.Type != "text" || workerBody.Body != "Thanks for reaching out" {
		t.Fatalf("unexpected worker body: %+v", workerBody)
	}

	msg, err := srv.repo.GetMessageByID(context.Background(), out.MessageID)
	if err != nil {
		t.Fatalf("get message: %v", err)
	}
	if msg.Direction != "outbound" || msg.Body != "Thanks for reaching out" || msg.ExternalID != "wamid.worker-123" || msg.Status != "sent" || msg.Error != "" {
		t.Fatalf("unexpected stored message: %+v", msg)
	}
}

func TestPostConversationMessageMarksFailedWhenWorkerFails(t *testing.T) {
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "baileys disconnected", http.StatusServiceUnavailable)
	}))
	defer worker.Close()

	srv := newTestServerWithConfig(t, Config{HTTPAddr: ":0", DatabaseURL: "file::memory:?cache=shared", InternalWebhookToken: "worker-secret", BaileysWorkerURL: worker.URL})
	defer srv.Close()
	token := loginAsDefaultOwner(t, srv)
	seedConversationFixtures(t, srv)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/cv-open-newer/messages", bytes.NewBufferString(`{"type":"text","body":"retry later"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}

	var out sendMessageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.MessageID == "" || out.Status != "failed" || !strings.Contains(out.Error, "503") {
		t.Fatalf("unexpected failed response: %+v", out)
	}
	msg, err := srv.repo.GetMessageByID(context.Background(), out.MessageID)
	if err != nil {
		t.Fatalf("get message: %v", err)
	}
	if msg.Status != "failed" || !strings.Contains(msg.Error, "503") || !strings.HasPrefix(msg.ExternalID, "pending:") {
		t.Fatalf("unexpected failed stored message: %+v", msg)
	}
}
