package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type startSessionRequest struct {
	DisplayName string `json:"display_name"`
}

type startSessionResponse struct {
	Channel channelDTO `json:"channel"`
	Session sessionDTO `json:"session"`
}

type channelDTO struct {
	ID          string `json:"id"`
	Provider    string `json:"provider"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
}

type sessionDTO struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	QRCode  string `json:"qr_code"`
	PollURL string `json:"poll_url"`
}

type channelsListResponse struct {
	Channels []channelDTO `json:"channels"`
}

func TestChannelsEndpointsRequireAuth(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	for _, tc := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/channels"},
		{method: http.MethodPost, path: "/api/v1/channels/whatsapp-baileys/session/start"},
		{method: http.MethodGet, path: "/api/v1/channels/whatsapp-baileys/session/session_123"},
		{method: http.MethodDelete, path: "/api/v1/channels/ch_123"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d, want %d", tc.method, tc.path, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestChannelStartPollListAndDisconnect(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	token := loginAsDefaultOwner(t, srv)

	body, _ := json.Marshal(startSessionRequest{DisplayName: "Support WA"})
	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/channels/whatsapp-baileys/session/start", bytes.NewReader(body))
	startReq.Header.Set("Authorization", "Bearer "+token)
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusCreated {
		t.Fatalf("start status = %d, want %d body=%s", startRec.Code, http.StatusCreated, startRec.Body.String())
	}

	var started startSessionResponse
	if err := json.Unmarshal(startRec.Body.Bytes(), &started); err != nil {
		t.Fatalf("invalid start response json: %v", err)
	}
	if started.Channel.ID == "" || started.Session.ID == "" {
		t.Fatalf("expected channel/session ids, got channel=%q session=%q", started.Channel.ID, started.Session.ID)
	}
	if started.Channel.Provider != "whatsapp_baileys" {
		t.Fatalf("provider = %q, want whatsapp_baileys", started.Channel.Provider)
	}
	if started.Session.Status != "qr_pending" {
		t.Fatalf("session status = %q, want qr_pending", started.Session.Status)
	}
	if started.Session.PollURL == "" {
		t.Fatal("expected poll_url")
	}

	pollReq := httptest.NewRequest(http.MethodGet, "/api/v1/channels/whatsapp-baileys/session/"+started.Session.ID, nil)
	pollReq.Header.Set("Authorization", "Bearer "+token)
	pollRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(pollRec, pollReq)
	if pollRec.Code != http.StatusOK {
		t.Fatalf("poll status = %d, want %d body=%s", pollRec.Code, http.StatusOK, pollRec.Body.String())
	}

	var polled sessionDTO
	if err := json.Unmarshal(pollRec.Body.Bytes(), &polled); err != nil {
		t.Fatalf("invalid poll response json: %v", err)
	}
	if polled.ID != started.Session.ID {
		t.Fatalf("poll id = %q, want %q", polled.ID, started.Session.ID)
	}
	if polled.Status != "qr_pending" {
		t.Fatalf("poll status = %q, want qr_pending", polled.Status)
	}
	if polled.QRCode == "" {
		t.Fatal("expected qr_code")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/channels", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d body=%s", listRec.Code, http.StatusOK, listRec.Body.String())
	}
	var listed channelsListResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("invalid channels list json: %v", err)
	}
	if len(listed.Channels) != 1 {
		t.Fatalf("channels count = %d, want 1", len(listed.Channels))
	}
	if listed.Channels[0].ID != started.Channel.ID {
		t.Fatalf("listed channel id = %q, want %q", listed.Channels[0].ID, started.Channel.ID)
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/channels/"+started.Channel.ID, nil)
	delReq.Header.Set("Authorization", "Bearer "+token)
	delRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d body=%s", delRec.Code, http.StatusNoContent, delRec.Body.String())
	}

	pollAfterReq := httptest.NewRequest(http.MethodGet, "/api/v1/channels/whatsapp-baileys/session/"+started.Session.ID, nil)
	pollAfterReq.Header.Set("Authorization", "Bearer "+token)
	pollAfterRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(pollAfterRec, pollAfterReq)
	if pollAfterRec.Code != http.StatusOK {
		t.Fatalf("poll-after-delete status = %d, want %d body=%s", pollAfterRec.Code, http.StatusOK, pollAfterRec.Body.String())
	}
	var polledAfter sessionDTO
	if err := json.Unmarshal(pollAfterRec.Body.Bytes(), &polledAfter); err != nil {
		t.Fatalf("invalid poll-after-delete json: %v", err)
	}
	if polledAfter.Status != "disconnected" {
		t.Fatalf("poll-after-delete status = %q, want disconnected", polledAfter.Status)
	}
}

func TestChannelMutationRequiresAdminRole(t *testing.T) {
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

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/channels/whatsapp-baileys/session/start", bytes.NewReader([]byte(`{"display_name":"Support"}`)))
	startReq.Header.Set("Authorization", "Bearer "+token)
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusForbidden {
		t.Fatalf("agent start status = %d, want %d", startRec.Code, http.StatusForbidden)
	}
}

func loginAsDefaultOwner(t *testing.T, srv *Server) string {
	t.Helper()
	payload, _ := json.Marshal(loginRequest{Email: defaultOwnerEmail, Password: defaultOwnerPassword})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var out loginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return out.Token
}
