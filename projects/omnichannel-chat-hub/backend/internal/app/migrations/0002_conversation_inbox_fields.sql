ALTER TABLE conversations ADD COLUMN assigned_to TEXT;
ALTER TABLE conversations ADD COLUMN unread_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE conversations ADD COLUMN last_message_at TEXT;

CREATE INDEX IF NOT EXISTS idx_conversations_status ON conversations(status);
CREATE INDEX IF NOT EXISTS idx_conversations_assigned_to ON conversations(assigned_to);
CREATE INDEX IF NOT EXISTS idx_conversations_channel_id ON conversations(channel_id);
CREATE INDEX IF NOT EXISTS idx_conversations_last_message_at ON conversations(last_message_at);
CREATE INDEX IF NOT EXISTS idx_messages_conversation_sent_at ON messages(conversation_id, sent_at);
