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

type ChannelSession struct {
	ID        string
	ChannelID string
	Provider  string
	Status    string
	QRCode    string
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
	ID            string
	Provider      string
	ExternalID    string
	ChannelID     string
	ContactID     string
	Status        string
	AssignedTo    string
	UnreadCount   int
	LastMessageAt string
}

type Message struct {
	ID             string
	Provider       string
	ExternalID     string
	ConversationID string
	Direction      string
	Body           string
	SentAt         string
	Status         string
	Error          string
}

type ConversationFilters struct {
	Status     string
	AssignedTo string
	ChannelID  string
	Page       int
	Limit      int
}

type ConversationPatch struct {
	Status     *string
	AssignedTo *string
	MarkRead   bool
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

type DevInboundMessageInput struct {
	Provider               string
	ChannelExternalID      string
	ChannelDisplayName     string
	ContactExternalID      string
	ContactDisplayName     string
	ConversationExternalID string
	MessageExternalID      string
	Body                   string
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

func (r *Repository) ListChannels(ctx context.Context) ([]Channel, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, provider, external_id, display_name, status
		FROM channels
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	defer rows.Close()
	channels := make([]Channel, 0)
	for rows.Next() {
		var c Channel
		if err := rows.Scan(&c.ID, &c.Provider, &c.ExternalID, &c.DisplayName, &c.Status); err != nil {
			return nil, fmt.Errorf("scan channel: %w", err)
		}
		channels = append(channels, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list channels rows: %w", err)
	}
	return channels, nil
}

func (r *Repository) CreateChannelSession(ctx context.Context, s ChannelSession) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO channel_sessions (id, channel_id, provider, status, qr_code)
		VALUES (?, ?, ?, ?, ?)
	`, s.ID, s.ChannelID, s.Provider, s.Status, nullIfEmpty(s.QRCode))
	return wrapConstraintErr("create channel session", err)
}

func (r *Repository) GetChannelSession(ctx context.Context, sessionID string) (ChannelSession, error) {
	var s ChannelSession
	err := r.db.QueryRowContext(ctx, `
		SELECT id, channel_id, provider, status, COALESCE(qr_code, '')
		FROM channel_sessions
		WHERE id = ?
	`, sessionID).Scan(&s.ID, &s.ChannelID, &s.Provider, &s.Status, &s.QRCode)
	if err != nil {
		return ChannelSession{}, fmt.Errorf("get channel session: %w", err)
	}
	return s, nil
}

func (r *Repository) UpdateChannelStatus(ctx context.Context, channelID, status string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE channels
		SET status = ?, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		WHERE id = ?
	`, status, channelID)
	if err != nil {
		return fmt.Errorf("update channel status: %w", err)
	}
	return nil
}

func (r *Repository) DisconnectChannel(ctx context.Context, channelID string) error {
	if err := r.UpdateChannelStatus(ctx, channelID, "inactive"); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE channel_sessions
		SET status = 'disconnected', updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		WHERE channel_id = ?
	`, channelID)
	if err != nil {
		return fmt.Errorf("disconnect channel sessions: %w", err)
	}
	return nil
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
		INSERT INTO conversations (id, provider, external_id, channel_id, contact_id, status, assigned_to, unread_count, last_message_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID, c.Provider, c.ExternalID, c.ChannelID, c.ContactID, c.Status, nullIfEmpty(c.AssignedTo), c.UnreadCount, nullIfEmpty(c.LastMessageAt))
	return wrapConstraintErr("create conversation", err)
}

func (r *Repository) ListConversations(ctx context.Context, f ConversationFilters) ([]Conversation, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 20
	}
	args := make([]any, 0)
	clauses := make([]string, 0)
	if f.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, f.Status)
	}
	if f.AssignedTo != "" {
		clauses = append(clauses, "COALESCE(assigned_to,'') = ?")
		args = append(args, f.AssignedTo)
	}
	if f.ChannelID != "" {
		clauses = append(clauses, "channel_id = ?")
		args = append(args, f.ChannelID)
	}
	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM conversations"+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count conversations: %w", err)
	}

	offset := (f.Page - 1) * f.Limit
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, provider, external_id, channel_id, contact_id, status, COALESCE(assigned_to,''), unread_count, COALESCE(last_message_at,'')
		FROM conversations`+where+`
		ORDER BY COALESCE(last_message_at, created_at) DESC, id ASC
		LIMIT ? OFFSET ?
	`, append(args, f.Limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()
	out := make([]Conversation, 0)
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.Provider, &c.ExternalID, &c.ChannelID, &c.ContactID, &c.Status, &c.AssignedTo, &c.UnreadCount, &c.LastMessageAt); err != nil {
			return nil, 0, fmt.Errorf("scan conversation: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list conversations rows: %w", err)
	}
	return out, total, nil
}

func (r *Repository) GetConversationByID(ctx context.Context, id string) (Conversation, error) {
	var c Conversation
	err := r.db.QueryRowContext(ctx, `
		SELECT id, provider, external_id, channel_id, contact_id, status, COALESCE(assigned_to,''), unread_count, COALESCE(last_message_at,'')
		FROM conversations
		WHERE id = ?
	`, id).Scan(&c.ID, &c.Provider, &c.ExternalID, &c.ChannelID, &c.ContactID, &c.Status, &c.AssignedTo, &c.UnreadCount, &c.LastMessageAt)
	if err != nil {
		return Conversation{}, fmt.Errorf("get conversation: %w", err)
	}
	return c, nil
}

func (r *Repository) PatchConversation(ctx context.Context, id string, patch ConversationPatch) (Conversation, error) {
	sets := make([]string, 0)
	args := make([]any, 0)
	if patch.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *patch.Status)
	}
	if patch.AssignedTo != nil {
		sets = append(sets, "assigned_to = ?")
		args = append(args, nullIfEmpty(strings.TrimSpace(*patch.AssignedTo)))
	}
	if patch.MarkRead {
		sets = append(sets, "unread_count = 0")
	}
	if len(sets) > 0 {
		sets = append(sets, "updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))")
		args = append(args, id)
		if _, err := r.db.ExecContext(ctx, "UPDATE conversations SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil {
			return Conversation{}, fmt.Errorf("patch conversation: %w", err)
		}
	}
	return r.GetConversationByID(ctx, id)
}

func (r *Repository) ListConversationMessages(ctx context.Context, conversationID string, page, limit int) ([]Message, int, error) {
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE conversation_id = ?`, conversationID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count messages: %w", err)
	}
	offset := (page - 1) * limit
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, provider, external_id, conversation_id, direction, body, sent_at
		FROM messages
		WHERE conversation_id = ?
		ORDER BY sent_at ASC, id ASC
		LIMIT ? OFFSET ?
	`, conversationID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()
	out := make([]Message, 0)
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Provider, &m.ExternalID, &m.ConversationID, &m.Direction, &m.Body, &m.SentAt); err != nil {
			return nil, 0, fmt.Errorf("scan message: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list messages rows: %w", err)
	}
	return out, total, nil
}

func (r *Repository) CreateMessage(ctx context.Context, m Message) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO messages (id, provider, external_id, conversation_id, direction, body)
		VALUES (?, ?, ?, ?, ?, ?)
	`, m.ID, m.Provider, m.ExternalID, m.ConversationID, m.Direction, m.Body)
	return wrapConstraintErr("create message", err)
}

func (r *Repository) GetChannelByID(ctx context.Context, id string) (Channel, error) {
	var c Channel
	err := r.db.QueryRowContext(ctx, `
		SELECT id, provider, external_id, display_name, status
		FROM channels
		WHERE id = ?
	`, id).Scan(&c.ID, &c.Provider, &c.ExternalID, &c.DisplayName, &c.Status)
	if err != nil {
		return Channel{}, fmt.Errorf("get channel: %w", err)
	}
	return c, nil
}

func (r *Repository) GetMessageByID(ctx context.Context, id string) (Message, error) {
	var m Message
	err := r.db.QueryRowContext(ctx, `
		SELECT id, provider, external_id, conversation_id, direction, body, sent_at, COALESCE(status,''), COALESCE(error,'')
		FROM messages
		WHERE id = ?
	`, id).Scan(&m.ID, &m.Provider, &m.ExternalID, &m.ConversationID, &m.Direction, &m.Body, &m.SentAt, &m.Status, &m.Error)
	if err != nil {
		return Message{}, fmt.Errorf("get message: %w", err)
	}
	return m, nil
}

func (r *Repository) CreateOutboundMessage(ctx context.Context, conversationID, body string) (Message, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, fmt.Errorf("begin outbound message tx: %w", err)
	}
	defer tx.Rollback()

	entityID, err := newEntityID(ctx, tx)
	if err != nil {
		return Message{}, err
	}
	messageID := "msg_" + entityID
	now := time.Now().UTC().Format(time.RFC3339)
	message := Message{
		ID:             messageID,
		Provider:       whatsappBaileysProvider,
		ExternalID:     "pending:" + messageID,
		ConversationID: conversationID,
		Direction:      "outbound",
		Body:           body,
		SentAt:         now,
		Status:         "pending",
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO messages (id, provider, external_id, conversation_id, direction, body, sent_at, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, message.ID, message.Provider, message.ExternalID, message.ConversationID, message.Direction, message.Body, message.SentAt, message.Status); err != nil {
		return Message{}, fmt.Errorf("create outbound message: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE conversations
		SET last_message_at = ?, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		WHERE id = ?
	`, now, conversationID); err != nil {
		return Message{}, fmt.Errorf("update conversation last outbound: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Message{}, fmt.Errorf("commit outbound message: %w", err)
	}
	return message, nil
}

func (r *Repository) MarkOutboundMessageSent(ctx context.Context, id, externalID string) (Message, error) {
	_, err := r.db.ExecContext(ctx, `
		UPDATE messages
		SET external_id = ?, status = 'sent', error = NULL
		WHERE id = ? AND direction = 'outbound'
	`, externalID, id)
	if err != nil {
		return Message{}, fmt.Errorf("mark outbound sent: %w", err)
	}
	return r.GetMessageByID(ctx, id)
}

func (r *Repository) MarkOutboundMessageFailed(ctx context.Context, id, sendErr string) (Message, error) {
	_, err := r.db.ExecContext(ctx, `
		UPDATE messages
		SET status = 'failed', error = ?
		WHERE id = ? AND direction = 'outbound'
	`, sendErr, id)
	if err != nil {
		return Message{}, fmt.Errorf("mark outbound failed: %w", err)
	}
	return r.GetMessageByID(ctx, id)
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

func (r *Repository) IngestDevInboundMessage(ctx context.Context, in DevInboundMessageInput) (string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	provider := strings.TrimSpace(in.Provider)
	if provider == "" {
		provider = "dev"
	}
	if provider != "dev" && provider != whatsappBaileysProvider {
		return "", fmt.Errorf("unsupported dev inbound provider: %s", provider)
	}

	var channelID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM channels WHERE provider=? AND external_id=? LIMIT 1`, provider, in.ChannelExternalID).Scan(&channelID)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.QueryRowContext(ctx, `SELECT 'ch_' || lower(hex(randomblob(8)))`).Scan(&channelID); err != nil {
			return "", fmt.Errorf("generate channel id: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO channels (id, provider, external_id, display_name, status) VALUES (?, ?, ?, ?, 'active')`, channelID, provider, in.ChannelExternalID, in.ChannelDisplayName); err != nil {
			return "", fmt.Errorf("insert channel: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("find channel: %w", err)
	}

	var contactID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM contacts WHERE provider=? AND external_id=? LIMIT 1`, provider, in.ContactExternalID).Scan(&contactID)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.QueryRowContext(ctx, `SELECT 'ct_' || lower(hex(randomblob(8)))`).Scan(&contactID); err != nil {
			return "", fmt.Errorf("generate contact id: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO contacts (id, provider, external_id, display_name, phone) VALUES (?, ?, ?, ?, ?)`, contactID, provider, in.ContactExternalID, in.ContactDisplayName, in.ContactExternalID); err != nil {
			return "", fmt.Errorf("insert contact: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("find contact: %w", err)
	}

	var existingMessageID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM messages WHERE provider=? AND external_id=? LIMIT 1`, provider, in.MessageExternalID).Scan(&existingMessageID)
	if err == nil {
		if err := tx.Commit(); err != nil {
			return "", fmt.Errorf("commit existing message: %w", err)
		}
		return existingMessageID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("find message: %w", err)
	}

	var conversationID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM conversations WHERE provider=? AND external_id=? LIMIT 1`, provider, in.ConversationExternalID).Scan(&conversationID)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.QueryRowContext(ctx, `SELECT 'cv_' || lower(hex(randomblob(8)))`).Scan(&conversationID); err != nil {
			return "", fmt.Errorf("generate conversation id: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO conversations (id, provider, external_id, channel_id, contact_id, status, unread_count, last_message_at) VALUES (?, ?, ?, ?, ?, 'open', 1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`, conversationID, provider, in.ConversationExternalID, channelID, contactID); err != nil {
			return "", fmt.Errorf("insert conversation: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("find conversation: %w", err)
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE conversations SET unread_count = unread_count + 1, last_message_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, conversationID); err != nil {
			return "", fmt.Errorf("update conversation unread count: %w", err)
		}
	}

	var messageID string
	if err := tx.QueryRowContext(ctx, `SELECT 'msg_' || lower(hex(randomblob(8)))`).Scan(&messageID); err != nil {
		return "", fmt.Errorf("generate message id: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO messages (id, provider, external_id, conversation_id, direction, body) VALUES (?, ?, ?, ?, 'inbound', ?)`, messageID, provider, in.MessageExternalID, conversationID, in.Body); err != nil {
		return "", fmt.Errorf("insert message: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit tx: %w", err)
	}
	return messageID, nil
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
