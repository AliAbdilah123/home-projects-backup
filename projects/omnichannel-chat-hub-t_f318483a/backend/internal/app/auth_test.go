package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string      `json:"token"`
	User  currentUser `json:"user"`
}

type currentUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

func TestAuthMeRejectsUnauthenticated(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuthLoginMeAndLogoutFlow(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	loginBody, _ := json.Marshal(loginRequest{Email: defaultOwnerEmail, Password: defaultOwnerPassword})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d body=%s", loginRec.Code, http.StatusOK, loginRec.Body.String())
	}

	var lr loginResponse
	if err := json.Unmarshal(loginRec.Body.Bytes(), &lr); err != nil {
		t.Fatalf("invalid login json: %v", err)
	}
	if lr.Token == "" {
		t.Fatalf("expected non-empty token")
	}
	if lr.User.Role != roleOwner {
		t.Fatalf("role = %s, want %s", lr.User.Role, roleOwner)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+lr.Token)
	meRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("me status = %d, want %d body=%s", meRec.Code, http.StatusOK, meRec.Body.String())
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutReq.Header.Set("Authorization", "Bearer "+lr.Token)
	logoutRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want %d", logoutRec.Code, http.StatusNoContent)
	}

	meAfterLogoutReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meAfterLogoutReq.Header.Set("Authorization", "Bearer "+lr.Token)
	meAfterLogoutRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(meAfterLogoutRec, meAfterLogoutReq)
	if meAfterLogoutRec.Code != http.StatusUnauthorized {
		t.Fatalf("me-after-logout status = %d, want %d", meAfterLogoutRec.Code, http.StatusUnauthorized)
	}
}

func TestRoleMiddlewareRejectsInsufficientRole(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	if err := srv.repo.UpsertUserCredentials(context.Background(), UserCredentials{
		Email:        "agent@example.com",
		Name:         "Support Agent",
		Role:         roleAgent,
		PasswordHash: hashPassword("agent-pass"),
	}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	token, err := srv.repo.CreateSession(context.Background(), "agent@example.com")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ping", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	srv, err := NewServer(Config{HTTPAddr: ":0", DatabaseURL: "file::memory:?cache=shared"})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	return srv
}
