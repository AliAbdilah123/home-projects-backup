package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	server, err := NewServer(Config{HTTPAddr: ":0", DatabaseURL: "file::memory:?cache=shared"})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	defer server.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if payload.Status != "ok" || payload.Database != "ok" {
		t.Fatalf("unexpected health payload: %+v", payload)
	}
}
