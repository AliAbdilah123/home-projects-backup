ALTER TABLE conversations ADD COLUMN unread_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE conversations ADD COLUMN last_message_at TEXT;
ALTER TABLE webhook_events ADD COLUMN error TEXT;
