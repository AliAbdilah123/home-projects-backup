ALTER TABLE messages ADD COLUMN status TEXT NOT NULL DEFAULT 'sent';
ALTER TABLE messages ADD COLUMN error TEXT;

CREATE INDEX IF NOT EXISTS idx_messages_status ON messages(status);
