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
	mux.HandleFunc("POST /api/v1/webhooks/whatsapp-baileys/internal", s.handleBaileysInternalWebhook)
	mux.Handle("GET /api/v1/admin/ping", s.authRequired(s.requireRole(roleOwner, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))))
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
