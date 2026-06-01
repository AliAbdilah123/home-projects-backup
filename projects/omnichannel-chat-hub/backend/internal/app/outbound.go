package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type outboundSendRequest struct {
	Type string `json:"type"`
	Body string `json:"body"`
}

type outboundSendResponse struct {
	MessageID         string `json:"message_id"`
	Provider          string `json:"provider"`
	ExternalMessageID string `json:"external_message_id"`
	Status            string `json:"status"`
	Error             string `json:"error,omitempty"`
}

type workerSendResponse struct {
	ExternalMessageID string `json:"external_message_id"`
	Status            string `json:"status"`
	Error             string `json:"error"`
}

func (s *Server) handleSendConversationMessage(w http.ResponseWriter, r *http.Request) {
	conversationID := strings.TrimSpace(r.PathValue("id"))
	if conversationID == "" {
		http.Error(w, "conversation id required", http.StatusBadRequest)
		return
	}

	var req outboundSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	req.Type = strings.TrimSpace(req.Type)
	req.Body = strings.TrimSpace(req.Body)
	if req.Type != "text" {
		http.Error(w, "only text messages are supported", http.StatusBadRequest)
		return
	}
	if req.Body == "" {
		http.Error(w, "body is required", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(s.baileysWorkerURL) == "" {
		http.Error(w, "baileys worker url is not configured", http.StatusBadGateway)
		return
	}

	conversation, err := s.repo.GetConversationByID(r.Context(), conversationID)
	if err != nil {
		http.Error(w, "conversation not found", http.StatusNotFound)
		return
	}
	if conversation.Provider != whatsappBaileysProvider {
		http.Error(w, "conversation provider is not supported for outbound sends", http.StatusBadRequest)
		return
	}
	channel, err := s.repo.GetChannelByID(r.Context(), conversation.ChannelID)
	if err != nil {
		http.Error(w, "channel not found", http.StatusNotFound)
		return
	}
	if channel.Provider != whatsappBaileysProvider || strings.TrimSpace(channel.ExternalID) == "" {
		http.Error(w, "channel is not a WhatsApp Baileys session", http.StatusBadRequest)
		return
	}

	message, err := s.repo.CreateOutboundMessage(r.Context(), conversation.ID, req.Body)
	if err != nil {
		http.Error(w, "failed to persist outbound message", http.StatusInternalServerError)
		return
	}
	actorType, actorID := auditActorFromContext(r.Context())

	chatID := conversation.ExternalID
	prefix := channel.ExternalID + ":"
	if strings.HasPrefix(chatID, prefix) {
		chatID = strings.TrimPrefix(chatID, prefix)
	}
	externalID, err := s.sendBaileysText(r.Context(), channel.ExternalID, message.ID, chatID, req.Body)
	if err != nil {
		failed, markErr := s.repo.MarkOutboundMessageFailed(r.Context(), message.ID, err.Error())
		_ = s.repo.LogAuditEvent(r.Context(), AuditEntry{ActorType: actorType, ActorID: actorID, Action: "outbound.send_failed", TargetType: "message", TargetID: message.ID, Metadata: map[string]any{"provider": message.Provider, "conversation_id": conversation.ID, "error": err.Error()}})
		if markErr != nil {
			http.Error(w, "worker send failed and status update failed", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusBadGateway, outboundSendResponse{MessageID: failed.ID, Provider: failed.Provider, ExternalMessageID: failed.ExternalID, Status: failed.Status, Error: failed.Error})
		return
	}

	sent, err := s.repo.MarkOutboundMessageSent(r.Context(), message.ID, externalID)
	if err != nil {
		http.Error(w, "worker sent but status update failed", http.StatusInternalServerError)
		return
	}
	_ = s.repo.LogAuditEvent(r.Context(), AuditEntry{ActorType: actorType, ActorID: actorID, Action: "outbound.send", TargetType: "message", TargetID: sent.ID, Metadata: map[string]any{"provider": sent.Provider, "conversation_id": conversation.ID, "external_message_id": sent.ExternalID}})
	writeJSON(w, http.StatusOK, outboundSendResponse{MessageID: sent.ID, Provider: sent.Provider, ExternalMessageID: sent.ExternalID, Status: sent.Status})
}

func (s *Server) sendBaileysText(ctx context.Context, sessionID, messageID, chatID, body string) (string, error) {
	payload, err := json.Marshal(map[string]string{
		"message_id": messageID,
		"chat_id":    chatID,
		"type":       "text",
		"body":       body,
	})
	if err != nil {
		return "", fmt.Errorf("marshal worker send request: %w", err)
	}
	url := strings.TrimRight(s.baileysWorkerURL, "/") + "/v1/sessions/" + sessionID + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create worker send request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.internalWebhookToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.internalWebhookToken)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("worker send request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var out workerSendResponse
	if len(respBody) > 0 {
		_ = json.Unmarshal(respBody, &out)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if out.Error != "" {
			return "", fmt.Errorf("worker send failed: %d %s", resp.StatusCode, out.Error)
		}
		return "", fmt.Errorf("worker send failed: %d %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if strings.TrimSpace(out.ExternalMessageID) == "" {
		return "", fmt.Errorf("worker send response missing external_message_id")
	}
	return strings.TrimSpace(out.ExternalMessageID), nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
