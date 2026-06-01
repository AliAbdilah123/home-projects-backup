package app

import (
	"context"
	"database/sql"
	"testing"
)

func newTestRepository(t *testing.T) (*Repository, func()) {
	t.Helper()
	srv, err := NewServer(Config{HTTPAddr: ":0", DatabaseURL: "file::memory:?cache=shared"})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	repo := NewRepository(srv.db)
	return repo, func() {
		_ = srv.Close()
	}
}

func tableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("check table exists: %v", err)
	}
	return name == table
}

func TestRepositoryCreateEntitiesAndUniqueness(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	if err := repo.CreateUser(ctx, User{ID: "u1", Email: "alice@example.com", Name: "Alice"}); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if err := repo.CreateChannel(ctx, Channel{ID: "ch1", Provider: "whatsapp", ExternalID: "wa-1", DisplayName: "WA 1", Status: "active"}); err != nil {
		t.Fatalf("CreateChannel() error = %v", err)
	}
	if err := repo.CreateContact(ctx, Contact{ID: "ct1", Provider: "whatsapp", ExternalID: "ext-contact-1", DisplayName: "Bob"}); err != nil {
		t.Fatalf("CreateContact() error = %v", err)
	}
	if err := repo.CreateConversation(ctx, Conversation{ID: "cv1", Provider: "whatsapp", ExternalID: "ext-conv-1", ChannelID: "ch1", ContactID: "ct1", Status: "open"}); err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	if err := repo.CreateMessage(ctx, Message{ID: "m1", Provider: "whatsapp", ExternalID: "ext-msg-1", ConversationID: "cv1", Direction: "inbound", Body: "hello"}); err != nil {
		t.Fatalf("CreateMessage() error = %v", err)
	}

	if err := repo.CreateChannel(ctx, Channel{ID: "ch2", Provider: "whatsapp", ExternalID: "wa-1", DisplayName: "Duplicate", Status: "active"}); err == nil {
		t.Fatal("expected duplicate provider+external_id channel to fail")
	}
	if err := repo.CreateContact(ctx, Contact{ID: "ct2", Provider: "whatsapp", ExternalID: "ext-contact-1", DisplayName: "Duplicate"}); err == nil {
		t.Fatal("expected duplicate provider+external_id contact to fail")
	}
	if err := repo.CreateConversation(ctx, Conversation{ID: "cv2", Provider: "whatsapp", ExternalID: "ext-conv-1", ChannelID: "ch1", ContactID: "ct1", Status: "open"}); err == nil {
		t.Fatal("expected duplicate provider+external_id conversation to fail")
	}
	if err := repo.CreateMessage(ctx, Message{ID: "m2", Provider: "whatsapp", ExternalID: "ext-msg-1", ConversationID: "cv1", Direction: "outbound", Body: "yo"}); err == nil {
		t.Fatal("expected duplicate provider+external_id message to fail")
	}
}

func TestWebhookEventIdempotency(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	created, err := repo.CreateWebhookEventIfNotExists(ctx, WebhookEvent{
		ID: "we-1", Provider: "whatsapp", ExternalID: "evt-1", EventType: "message.received", Payload: `{"id":"evt-1"}`,
	})
	if err != nil {
		t.Fatalf("CreateWebhookEventIfNotExists first call error = %v", err)
	}
	if !created {
		t.Fatal("expected first webhook event insert to create row")
	}

	created, err = repo.CreateWebhookEventIfNotExists(ctx, WebhookEvent{
		ID: "we-2", Provider: "whatsapp", ExternalID: "evt-1", EventType: "message.received", Payload: `{"id":"evt-1"}`,
	})
	if err != nil {
		t.Fatalf("CreateWebhookEventIfNotExists second call error = %v", err)
	}
	if created {
		t.Fatal("expected second webhook event insert to be ignored for idempotency")
	}
}
