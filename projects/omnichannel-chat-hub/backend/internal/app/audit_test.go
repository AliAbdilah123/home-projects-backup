package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type auditRow struct {
	Action   string
	Metadata string
}

func TestAuditLogsLoginChannelPatchAndDisconnect(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	loginPayload, _ := json.Marshal(loginRequest{Email: defaultOwnerEmail, Password: defaultOwnerPassword})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginPayload))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginRec.Code, loginRec.Body.String())
	}
	var loginOut loginResponse
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginOut); err != nil {
		t.Fatalf("decode login response: %v", err)
	}

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/channels/whatsapp-baileys/session/start", bytes.NewBufferString(`{"display_name":"Support"}`))
	startReq.Header.Set("Authorization", "Bearer "+loginOut.Token)
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusCreated {
		t.Fatalf("start status=%d body=%s", startRec.Code, startRec.Body.String())
	}
	var started startSessionResponse
	if err := json.Unmarshal(startRec.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode start response: %v", err)
	}

	seedConversationFixtures(t, srv)
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/conversations/cv-open-newer", bytes.NewBufferString(`{"status":"closed","assigned_to":"admin@example.com","mark_read":true}`))
	patchReq.Header.Set("Authorization", "Bearer "+loginOut.Token)
	patchReq.Header.Set("Content-Type", "application/json")
	patchRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", patchRec.Code, patchRec.Body.String())
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/channels/"+started.Channel.ID, nil)
	delReq.Header.Set("Authorization", "Bearer "+loginOut.Token)
	delRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("disconnect status=%d body=%s", delRec.Code, delRec.Body.String())
	}

	actions := auditActions(t, srv)
	for _, want := range []string{"auth.login", "channel.connect", "conversation.assignment_changed", "conversation.status_changed", "channel.disconnect"} {
		if !contains(actions, want) {
			t.Fatalf("missing audit action %q in %v", want, actions)
		}
	}
}

func TestAuditLogRedactsSecrets(t *testing.T) {
	srv := newWebhookTestServer(t)

	rec := postInternalWebhook(srv, "test-internal-token", `{"event_id":"evt-invalid","event_type":"messages.upsert","session_id":"session-a","message":{"chat_id":"15551234567@s.whatsapp.net","text":"missing message id"},"token":"super-secret-token","session_path":"/tmp/baileys/auth/session.json"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	rows := loadAuditRows(t, srv, "webhook.invalid_payload")
	if len(rows) == 0 {
		t.Fatal("expected webhook.invalid_payload audit row")
	}
	m := rows[len(rows)-1].Metadata
	for _, forbidden := range []string{"super-secret-token", "/tmp/baileys/auth/session.json"} {
		if strings.Contains(m, forbidden) {
			t.Fatalf("metadata leaked secret %q: %s", forbidden, m)
		}
	}
	if !strings.Contains(m, "[REDACTED]") {
		t.Fatalf("expected redaction marker in metadata: %s", m)
	}
}

func auditActions(t *testing.T, srv *Server) []string {
	t.Helper()
	rows, err := srv.db.Query(`SELECT action FROM audit_logs ORDER BY created_at ASC`)
	if err != nil {
		t.Fatalf("query audit actions: %v", err)
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var action string
		if err := rows.Scan(&action); err != nil {
			t.Fatalf("scan action: %v", err)
		}
		out = append(out, action)
	}
	return out
}

func loadAuditRows(t *testing.T, srv *Server, action string) []auditRow {
	t.Helper()
	rows, err := srv.db.Query(`SELECT action, COALESCE(metadata, '') FROM audit_logs WHERE action = ? ORDER BY created_at ASC`, action)
	if err != nil {
		t.Fatalf("query audit rows: %v", err)
	}
	defer rows.Close()
	out := make([]auditRow, 0)
	for rows.Next() {
		var r auditRow
		if err := rows.Scan(&r.Action, &r.Metadata); err != nil {
			t.Fatalf("scan row: %v", err)
		}
		out = append(out, r)
	}
	return out
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
