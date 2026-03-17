CREATE TABLE IF NOT EXISTS group_chats (
    chat_id  INTEGER PRIMARY KEY,
    title    TEXT    NOT NULL DEFAULT '',
    approved INTEGER NOT NULL DEFAULT 0,
    added_at TEXT    NOT NULL DEFAULT (datetime('now'))
);
