CREATE TABLE IF NOT EXISTS channels (
	id TEXT PRIMARY KEY,
	platform TEXT NOT NULL,
	identifier TEXT NOT NULL,
	name TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'disconnected',
	connected_at TEXT,
	created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE IF NOT EXISTS conversations (
	id TEXT PRIMARY KEY,
	channel_id TEXT NOT NULL,
	external_id TEXT,
	contact TEXT NOT NULL,
	display_name TEXT,
	unread_count INTEGER NOT NULL DEFAULT 0,
	last_message_at TEXT NOT NULL DEFAULT (datetime('now')),
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	UNIQUE(channel_id, external_id)
);
CREATE TABLE IF NOT EXISTS messages (
	id TEXT PRIMARY KEY,
	conv_id TEXT NOT NULL,
	direction TEXT NOT NULL,
	sender TEXT NOT NULL,
	content TEXT NOT NULL,
	media_url TEXT,
	status TEXT NOT NULL DEFAULT 'sent',
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	FOREIGN KEY(conv_id) REFERENCES conversations(id) ON DELETE CASCADE
);
