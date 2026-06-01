package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type conversationDTO struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	AssignedTo  string `json:"assigned_to"`
	UnreadCount int    `json:"unread_count"`
	ChannelID   string `json:"channel_id"`
}

type listConversationsResponse struct {
	Conversations []conversationDTO `json:"conversations"`
	Page          int               `json:"page"`
	Limit         int               `json:"limit"`
	Total         int               `json:"total"`
}

type messageDTO struct {
	ID        string `json:"id"`
	Direction string `json:"direction"`
	Body      string `json:"body"`
	SentAt    string `json:"sent_at"`
}

type listMessagesResponse struct {
	Messages []messageDTO `json:"messages"`
	Page     int          `json:"page"`
	Limit    int          `json:"limit"`
	Total    int          `json:"total"`
}

func TestConversationsEndpointsRequireAuth(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/v1/conversations"},
		{method: http.MethodGet, path: "/api/v1/conversations/cv1"},
		{method: http.MethodPatch, path: "/api/v1/conversations/cv1", body: `{"status":"closed"}`},
		{method: http.MethodGet, path: "/api/v1/conversations/cv1/messages"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status=%d want=%d", tc.method, tc.path, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestListConversationsFiltersPaginationAndStableOrder(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	token := loginAsDefaultOwner(t, srv)
	seedConversationFixtures(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/conversations?status=open&assigned_to=agent@example.com&channel=ch-wa-1&page=1&limit=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var out listConversationsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Total != 2 {
		t.Fatalf("total=%d want=2", out.Total)
	}
	if len(out.Conversations) != 1 {
		t.Fatalf("len=%d want=1", len(out.Conversations))
	}
	if out.Conversations[0].ID != "cv-open-newer" {
		t.Fatalf("first id=%s want=cv-open-newer", out.Conversations[0].ID)
	}

	reqPage2 := httptest.NewRequest(http.MethodGet, "/api/v1/conversations?status=open&assigned_to=agent@example.com&channel=ch-wa-1&page=2&limit=1", nil)
	reqPage2.Header.Set("Authorization", "Bearer "+token)
	recPage2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recPage2, reqPage2)
	if recPage2.Code != http.StatusOK {
		t.Fatalf("page2 status=%d want=%d body=%s", recPage2.Code, http.StatusOK, recPage2.Body.String())
	}
	out = listConversationsResponse{}
	if err := json.Unmarshal(recPage2.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode page2 response: %v", err)
	}
	if len(out.Conversations) != 1 || out.Conversations[0].ID != "cv-open-older" {
		t.Fatalf("page2 conversations=%+v", out.Conversations)
	}
}

func TestPatchConversationUpdatesAssignmentStatusAndUnread(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	token := loginAsDefaultOwner(t, srv)
	seedConversationFixtures(t, srv)

	body := bytes.NewBufferString(`{"status":"closed","assigned_to":"admin@example.com","mark_read":true}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/conversations/cv-open-newer", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var out conversationDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Status != "closed" {
		t.Fatalf("status=%s want=closed", out.Status)
	}
	if out.AssignedTo != "admin@example.com" {
		t.Fatalf("assigned_to=%s want=admin@example.com", out.AssignedTo)
	}
	if out.UnreadCount != 0 {
		t.Fatalf("unread_count=%d want=0", out.UnreadCount)
	}
}

func TestConversationMessagesStableOrderingAndPagination(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	token := loginAsDefaultOwner(t, srv)
	seedConversationFixtures(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/cv-open-newer/messages?page=1&limit=2", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var out listMessagesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Total != 3 {
		t.Fatalf("total=%d want=3", out.Total)
	}
	if len(out.Messages) != 2 {
		t.Fatalf("len=%d want=2", len(out.Messages))
	}
	if out.Messages[0].ID != "m-open-newer-1" || out.Messages[1].ID != "m-open-newer-2" {
		t.Fatalf("page1 ids=%s,%s", out.Messages[0].ID, out.Messages[1].ID)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/cv-open-newer/messages?page=2&limit=2", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("status2=%d want=%d body=%s", rec2.Code, http.StatusOK, rec2.Body.String())
	}
	out = listMessagesResponse{}
	if err := json.Unmarshal(rec2.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response2: %v", err)
	}
	if len(out.Messages) != 1 || out.Messages[0].ID != "m-open-newer-3" {
		t.Fatalf("page2 messages=%+v", out.Messages)
	}
}

func seedConversationFixtures(t *testing.T, srv *Server) {
	t.Helper()
	ctx := context.Background()

	if err := srv.repo.UpsertUserCredentials(ctx, UserCredentials{Email: "admin@example.com", Name: "Admin", Role: roleAdmin, PasswordHash: hashPassword("admin-pass")}); err != nil {
		t.Fatalf("seed admin user: %v", err)
	}
	if err := srv.repo.UpsertUserCredentials(ctx, UserCredentials{Email: "agent@example.com", Name: "Agent", Role: roleAgent, PasswordHash: hashPassword("agent-pass")}); err != nil {
		t.Fatalf("seed agent user: %v", err)
	}

	must(t, srv.repo.CreateChannel(ctx, Channel{ID: "ch-wa-1", Provider: "whatsapp_baileys", ExternalID: "wa-1", DisplayName: "WA1", Status: "connected"}))
	must(t, srv.repo.CreateChannel(ctx, Channel{ID: "ch-wa-2", Provider: "whatsapp_baileys", ExternalID: "wa-2", DisplayName: "WA2", Status: "connected"}))
	must(t, srv.repo.CreateContact(ctx, Contact{ID: "ct-1", Provider: "whatsapp_baileys", ExternalID: "ext-1", DisplayName: "A"}))
	must(t, srv.repo.CreateContact(ctx, Contact{ID: "ct-2", Provider: "whatsapp_baileys", ExternalID: "ext-2", DisplayName: "B"}))
	must(t, srv.repo.CreateContact(ctx, Contact{ID: "ct-3", Provider: "whatsapp_baileys", ExternalID: "ext-3", DisplayName: "C"}))

	must(t, srv.repo.CreateConversation(ctx, Conversation{ID: "cv-open-newer", Provider: "whatsapp_baileys", ExternalID: "c1", ChannelID: "ch-wa-1", ContactID: "ct-1", Status: "open"}))
	must(t, srv.repo.CreateConversation(ctx, Conversation{ID: "cv-open-older", Provider: "whatsapp_baileys", ExternalID: "c2", ChannelID: "ch-wa-1", ContactID: "ct-2", Status: "open"}))
	must(t, srv.repo.CreateConversation(ctx, Conversation{ID: "cv-closed", Provider: "whatsapp_baileys", ExternalID: "c3", ChannelID: "ch-wa-2", ContactID: "ct-3", Status: "closed"}))

	_, err := srv.db.ExecContext(ctx, `
		UPDATE conversations
		SET assigned_to = CASE id
			WHEN 'cv-open-newer' THEN 'agent@example.com'
			WHEN 'cv-open-older' THEN 'agent@example.com'
			WHEN 'cv-closed' THEN 'admin@example.com'
		END,
		unread_count = CASE id
			WHEN 'cv-open-newer' THEN 3
			WHEN 'cv-open-older' THEN 1
			WHEN 'cv-closed' THEN 0
		END,
		last_message_at = CASE id
			WHEN 'cv-open-newer' THEN '2026-05-31T12:03:00Z'
			WHEN 'cv-open-older' THEN '2026-05-31T12:01:00Z'
			WHEN 'cv-closed' THEN '2026-05-31T12:02:00Z'
		END
	`)
	must(t, err)

	must(t, srv.repo.CreateMessage(ctx, Message{ID: "m-open-newer-1", Provider: "whatsapp_baileys", ExternalID: "m1", ConversationID: "cv-open-newer", Direction: "inbound", Body: "hello 1"}))
	must(t, srv.repo.CreateMessage(ctx, Message{ID: "m-open-newer-2", Provider: "whatsapp_baileys", ExternalID: "m2", ConversationID: "cv-open-newer", Direction: "outbound", Body: "hello 2"}))
	must(t, srv.repo.CreateMessage(ctx, Message{ID: "m-open-newer-3", Provider: "whatsapp_baileys", ExternalID: "m3", ConversationID: "cv-open-newer", Direction: "inbound", Body: "hello 3"}))

	_, err = srv.db.ExecContext(ctx, `
		UPDATE messages
		SET sent_at = CASE id
			WHEN 'm-open-newer-1' THEN '2026-05-31T12:00:00Z'
			WHEN 'm-open-newer-2' THEN '2026-05-31T12:01:00Z'
			WHEN 'm-open-newer-3' THEN '2026-05-31T12:02:00Z'
		END
		WHERE conversation_id = 'cv-open-newer'
	`)
	must(t, err)
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
