package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

const (
	roleOwner = "owner"
	roleAdmin = "admin"
	roleAgent = "agent"

	defaultOwnerEmail    = "owner@example.com"
	defaultOwnerPassword = "owner123"
)

type contextKey string

const currentUserKey contextKey = "current_user"

type Server struct {
	db                   *sql.DB
	repo                 *Repository
	internalWebhookToken string
}

type healthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
	Time     string `json:"time"`
}

type authResponse struct {
	Token string   `json:"token"`
	User  AuthUser `json:"user"`
}

func NewServer(cfg Config) (*Server, error) {
	if err := ensureSQLiteDirectory(cfg.DatabaseURL); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	server := &Server{db: db, repo: NewRepository(db), internalWebhookToken: cfg.InternalWebhookToken}
	if err := server.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := server.seedDefaultOwner(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return server, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	mux.Handle("POST /api/v1/auth/logout", s.authRequired(http.HandlerFunc(s.handleLogout)))
	mux.Handle("GET /api/v1/me", s.authRequired(http.HandlerFunc(s.handleMe)))
	mux.Handle("GET /api/v1/admin/ping", s.authRequired(s.requireRole(roleOwner, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))))
	mux.Handle("GET /api/v1/channels", s.authRequired(http.HandlerFunc(s.handleListChannels)))
	mux.Handle("GET /api/v1/conversations", s.authRequired(http.HandlerFunc(s.handleListConversations)))
	mux.Handle("GET /api/v1/conversations/{id}", s.authRequired(http.HandlerFunc(s.handleGetConversation)))
	mux.Handle("PATCH /api/v1/conversations/{id}", s.authRequired(http.HandlerFunc(s.handlePatchConversation)))
	mux.Handle("GET /api/v1/conversations/{id}/messages", s.authRequired(http.HandlerFunc(s.handleListConversationMessages)))
	mux.Handle("POST /api/v1/channels/whatsapp-baileys/session/start", s.authRequired(s.requireRole(roleAdmin, http.HandlerFunc(s.handleStartBaileysSession))))
	mux.Handle("GET /api/v1/channels/whatsapp-baileys/session/{id}", s.authRequired(http.HandlerFunc(s.handleGetBaileysSession)))
	mux.Handle("DELETE /api/v1/channels/{id}", s.authRequired(s.requireRole(roleAdmin, http.HandlerFunc(s.handleDisconnectChannel))))
	return mux
}

func (s *Server) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	databaseStatus := "ok"
	if err := s.db.PingContext(r.Context()); err != nil {
		databaseStatus = "error"
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(healthResponse{
		Status:   "ok",
		Database: databaseStatus,
		Time:     time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	user, err := s.repo.GetUserByEmail(r.Context(), strings.TrimSpace(strings.ToLower(req.Email)))
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if user.PasswordHash != hashPassword(req.Password) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	token, err := s.repo.CreateSession(r.Context(), user.Email)
	if err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(authResponse{Token: token, User: sanitizeUser(user)})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	token, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := s.repo.DeleteSession(r.Context(), token); err != nil {
		http.Error(w, "failed to logout", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sanitizeUser(user))
}

func (s *Server) handleListChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := s.repo.ListChannels(r.Context())
	if err != nil {
		http.Error(w, "failed to list channels", http.StatusInternalServerError)
		return
	}
	type channelResponse struct {
		ID          string `json:"id"`
		Provider    string `json:"provider"`
		DisplayName string `json:"display_name"`
		Status      string `json:"status"`
	}
	out := struct {
		Channels []channelResponse `json:"channels"`
	}{Channels: make([]channelResponse, 0, len(channels))}
	for _, c := range channels {
		out.Channels = append(out.Channels, channelResponse{ID: c.ID, Provider: c.Provider, DisplayName: c.DisplayName, Status: c.Status})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := parseQueryInt(q.Get("page"), 1)
	limit := parseQueryInt(q.Get("limit"), 20)
	conversations, total, err := s.repo.ListConversations(r.Context(), ConversationFilters{
		Status:     strings.TrimSpace(q.Get("status")),
		AssignedTo: strings.TrimSpace(q.Get("assigned_to")),
		ChannelID:  strings.TrimSpace(q.Get("channel")),
		Page:       page,
		Limit:      limit,
	})
	if err != nil {
		http.Error(w, "failed to list conversations", http.StatusInternalServerError)
		return
	}
	type conversationResponse struct {
		ID          string `json:"id"`
		ChannelID   string `json:"channel_id"`
		Status      string `json:"status"`
		AssignedTo  string `json:"assigned_to"`
		UnreadCount int    `json:"unread_count"`
	}
	out := struct {
		Conversations []conversationResponse `json:"conversations"`
		Page          int                    `json:"page"`
		Limit         int                    `json:"limit"`
		Total         int                    `json:"total"`
	}{Conversations: make([]conversationResponse, 0, len(conversations)), Page: page, Limit: limit, Total: total}
	for _, c := range conversations {
		out.Conversations = append(out.Conversations, conversationResponse{ID: c.ID, ChannelID: c.ChannelID, Status: c.Status, AssignedTo: c.AssignedTo, UnreadCount: c.UnreadCount})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.Error(w, "conversation id required", http.StatusBadRequest)
		return
	}
	conversation, err := s.repo.GetConversationByID(r.Context(), id)
	if err != nil {
		http.Error(w, "conversation not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": conversation.ID, "channel_id": conversation.ChannelID, "status": conversation.Status, "assigned_to": conversation.AssignedTo, "unread_count": conversation.UnreadCount})
}

func (s *Server) handlePatchConversation(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.Error(w, "conversation id required", http.StatusBadRequest)
		return
	}
	var req struct {
		Status     *string `json:"status"`
		AssignedTo *string `json:"assigned_to"`
		MarkRead   bool    `json:"mark_read"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	conversation, err := s.repo.PatchConversation(r.Context(), id, ConversationPatch{Status: req.Status, AssignedTo: req.AssignedTo, MarkRead: req.MarkRead})
	if err != nil {
		http.Error(w, "failed to update conversation", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": conversation.ID, "channel_id": conversation.ChannelID, "status": conversation.Status, "assigned_to": conversation.AssignedTo, "unread_count": conversation.UnreadCount})
}

func (s *Server) handleListConversationMessages(w http.ResponseWriter, r *http.Request) {
	conversationID := strings.TrimSpace(r.PathValue("id"))
	if conversationID == "" {
		http.Error(w, "conversation id required", http.StatusBadRequest)
		return
	}
	page := parseQueryInt(r.URL.Query().Get("page"), 1)
	limit := parseQueryInt(r.URL.Query().Get("limit"), 50)
	messages, total, err := s.repo.ListConversationMessages(r.Context(), conversationID, page, limit)
	if err != nil {
		http.Error(w, "failed to list messages", http.StatusInternalServerError)
		return
	}
	type messageResponse struct {
		ID        string `json:"id"`
		Direction string `json:"direction"`
		Body      string `json:"body"`
		SentAt    string `json:"sent_at"`
	}
	out := struct {
		Messages []messageResponse `json:"messages"`
		Page     int               `json:"page"`
		Limit    int               `json:"limit"`
		Total    int               `json:"total"`
	}{Messages: make([]messageResponse, 0, len(messages)), Page: page, Limit: limit, Total: total}
	for _, m := range messages {
		out.Messages = append(out.Messages, messageResponse{ID: m.ID, Direction: m.Direction, Body: m.Body, SentAt: m.SentAt})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleStartBaileysSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.DisplayName) == "" {
		req.DisplayName = "WhatsApp"
	}
	var channelID, sessionID string
	if err := s.db.QueryRowContext(r.Context(), `SELECT 'ch_' || lower(hex(randomblob(8))), 'wabs_' || lower(hex(randomblob(8)))`).Scan(&channelID, &sessionID); err != nil {
		http.Error(w, "failed to generate ids", http.StatusInternalServerError)
		return
	}
	if err := s.repo.CreateChannel(r.Context(), Channel{ID: channelID, Provider: "whatsapp_baileys", ExternalID: sessionID, DisplayName: req.DisplayName, Status: "connecting"}); err != nil {
		http.Error(w, "failed to create channel", http.StatusInternalServerError)
		return
	}
	qrCode := "mock-qr-" + sessionID
	if err := s.repo.CreateChannelSession(r.Context(), ChannelSession{ID: sessionID, ChannelID: channelID, Provider: "whatsapp_baileys", Status: "qr_pending", QRCode: qrCode}); err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"channel": map[string]any{"id": channelID, "provider": "whatsapp_baileys", "display_name": req.DisplayName, "status": "connecting"},
		"session": map[string]any{"id": sessionID, "status": "qr_pending", "qr_code": qrCode, "poll_url": "/api/v1/channels/whatsapp-baileys/session/" + sessionID},
	})
}

func (s *Server) handleGetBaileysSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if strings.TrimSpace(sessionID) == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}
	session, err := s.repo.GetChannelSession(r.Context(), sessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": session.ID, "status": session.Status, "qr_code": session.QRCode, "poll_url": "/api/v1/channels/whatsapp-baileys/session/" + session.ID})
}

func (s *Server) handleDisconnectChannel(w http.ResponseWriter, r *http.Request) {
	channelID := r.PathValue("id")
	if strings.TrimSpace(channelID) == "" {
		http.Error(w, "channel id required", http.StatusBadRequest)
		return
	}
	if err := s.repo.DisconnectChannel(r.Context(), channelID); err != nil {
		http.Error(w, "failed to disconnect channel", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) authRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		user, err := s.repo.GetUserBySessionToken(r.Context(), token)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), currentUserKey, user)))
	})
}

func (s *Server) requireRole(minRole string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := userFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !roleAllowed(user.Role, minRole) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func roleAllowed(actual, required string) bool {
	levels := map[string]int{roleAgent: 1, roleAdmin: 2, roleOwner: 3}
	return levels[actual] >= levels[required]
}

func userFromContext(ctx context.Context) (AuthUser, bool) {
	v := ctx.Value(currentUserKey)
	if v == nil {
		return AuthUser{}, false
	}
	user, ok := v.(AuthUser)
	return user, ok
}

func sanitizeUser(u AuthUser) AuthUser {
	u.PasswordHash = ""
	return u
}

func bearerToken(h string) (string, bool) {
	parts := strings.SplitN(strings.TrimSpace(h), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", false
	}
	return strings.TrimSpace(parts[1]), true
}

func hashPassword(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

func (s *Server) seedDefaultOwner(ctx context.Context) error {
	owner, err := s.repo.GetUserByEmail(ctx, defaultOwnerEmail)
	if err == nil && owner.Email == defaultOwnerEmail {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) && !strings.Contains(err.Error(), "no rows") {
		return err
	}
	return s.repo.UpsertUserCredentials(ctx, UserCredentials{
		Email:        defaultOwnerEmail,
		Name:         "Owner",
		Role:         roleOwner,
		PasswordHash: hashPassword(defaultOwnerPassword),
	})
}

func (s *Server) migrate() error {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)
	`); err != nil {
		return fmt.Errorf("ensure schema_migrations table: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		applied, err := s.isMigrationApplied(entry.Name())
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		sqlBytes, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		if _, err := s.db.Exec(string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
		if _, err := s.db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, entry.Name()); err != nil {
			return fmt.Errorf("record migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func (s *Server) isMigrationApplied(version string) (bool, error) {
	var exists int
	err := s.db.QueryRow(`SELECT 1 FROM schema_migrations WHERE version = ? LIMIT 1`, version).Scan(&exists)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("check migration %s: %w", version, err)
}

func ensureSQLiteDirectory(databaseURL string) error {
	path := databaseURL
	if len(path) >= 5 && path[:5] == "file:" {
		path = path[5:]
	}
	if idx := indexAny(path, "?"); idx >= 0 {
		path = path[:idx]
	}
	if path == "" || path == ":memory:" {
		return nil
	}
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func parseQueryInt(raw string, fallback int) int {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		return fallback
	}
	return v
}

func indexAny(s, chars string) int {
	for i, r := range s {
		for _, c := range chars {
			if r == c {
				return i
			}
		}
	}
	return -1
}
