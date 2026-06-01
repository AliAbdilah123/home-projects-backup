package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

type User struct {
	ID    string
	Email string
	Name  string
}

type Channel struct {
	ID          string
	Provider    string
	ExternalID  string
	DisplayName string
	Status      string
}

type Contact struct {
	ID          string
	Provider    string
	ExternalID  string
	DisplayName string
	Phone       string
	Email       string
}

type Conversation struct {
	ID         string
	Provider   string
	ExternalID string
	ChannelID  string
	ContactID  string
	Status     string
}

type Message struct {
	ID             string
	Provider       string
	ExternalID     string
	ConversationID string
	Direction      string
	Body           string
}

type WebhookEvent struct {
	ID         string
	Provider   string
	ExternalID string
	EventType  string
	Payload    string
}

type UserCredentials struct {
	Email        string
	Name         string
	Role         string
	PasswordHash string
}

type AuthUser struct {
	ID           string
	Email        string
	Name         string
	Role         string
	PasswordHash string
}

func (r *Repository) CreateUser(ctx context.Context, u User) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO users (id, email, name, role, password_hash)
		VALUES (?, ?, ?, 'agent', '')
	`, u.ID, u.Email, u.Name)
	return wrapConstraintErr("create user", err)
}

func (r *Repository) CreateChannel(ctx context.Context, c Channel) error {
	if c.Status == "" {
		c.Status = "inactive"
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO channels (id, provider, external_id, display_name, status)
		VALUES (?, ?, ?, ?, ?)
	`, c.ID, c.Provider, c.ExternalID, c.DisplayName, c.Status)
	return wrapConstraintErr("create channel", err)
}

func (r *Repository) CreateContact(ctx context.Context, c Contact) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO contacts (id, provider, external_id, display_name, phone, email)
		VALUES (?, ?, ?, ?, ?, ?)
	`, c.ID, c.Provider, c.ExternalID, c.DisplayName, nullIfEmpty(c.Phone), nullIfEmpty(c.Email))
	return wrapConstraintErr("create contact", err)
}

func (r *Repository) CreateConversation(ctx context.Context, c Conversation) error {
	if c.Status == "" {
		c.Status = "open"
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO conversations (id, provider, external_id, channel_id, contact_id, status)
		VALUES (?, ?, ?, ?, ?, ?)
	`, c.ID, c.Provider, c.ExternalID, c.ChannelID, c.ContactID, c.Status)
	return wrapConstraintErr("create conversation", err)
}

func (r *Repository) CreateMessage(ctx context.Context, m Message) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO messages (id, provider, external_id, conversation_id, direction, body)
		VALUES (?, ?, ?, ?, ?, ?)
	`, m.ID, m.Provider, m.ExternalID, m.ConversationID, m.Direction, m.Body)
	return wrapConstraintErr("create message", err)
}

func (r *Repository) CreateWebhookEventIfNotExists(ctx context.Context, e WebhookEvent) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
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

func (r *Repository) UpsertUserCredentials(ctx context.Context, in UserCredentials) error {
	if in.Role == "" {
		in.Role = "agent"
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO users (id, email, name, role, password_hash)
		VALUES (lower(hex(randomblob(16))), ?, ?, ?, ?)
		ON CONFLICT(email) DO UPDATE SET
			name = excluded.name,
			role = excluded.role,
			password_hash = excluded.password_hash,
			updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	`, in.Email, in.Name, in.Role, in.PasswordHash)
	return wrapConstraintErr("upsert user credentials", err)
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (AuthUser, error) {
	var user AuthUser
	err := r.db.QueryRowContext(ctx, `
		SELECT id, email, name, role, password_hash
		FROM users
		WHERE email = ?
	`, email).Scan(&user.ID, &user.Email, &user.Name, &user.Role, &user.PasswordHash)
	if err != nil {
		return AuthUser{}, fmt.Errorf("get user by email: %w", err)
	}
	return user, nil
}

func (r *Repository) CreateSession(ctx context.Context, email string) (string, error) {
	var token string
	if err := r.db.QueryRowContext(ctx, `SELECT lower(hex(randomblob(32)))`).Scan(&token); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	expiresAt := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO sessions (token, user_email, expires_at)
		VALUES (?, ?, ?)
	`, token, email, expiresAt)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return token, nil
}

func (r *Repository) DeleteSession(ctx context.Context, token string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (r *Repository) GetUserBySessionToken(ctx context.Context, token string) (AuthUser, error) {
	var user AuthUser
	err := r.db.QueryRowContext(ctx, `
		SELECT u.id, u.email, u.name, u.role, u.password_hash
		FROM sessions s
		JOIN users u ON u.email = s.user_email
		WHERE s.token = ? AND s.expires_at > (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
	`, token).Scan(&user.ID, &user.Email, &user.Name, &user.Role, &user.PasswordHash)
	if err != nil {
		return AuthUser{}, fmt.Errorf("get user by session token: %w", err)
	}
	return user, nil
}

func nullIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func wrapConstraintErr(op string, err error) error {
	if err == nil {
		return nil
	}
	if isConstraintErr(err) {
		return fmt.Errorf("%s: constraint violation: %w", op, err)
	}
	return fmt.Errorf("%s: %w", op, err)
}

func isConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "constraint") || strings.Contains(msg, "unique")
}
