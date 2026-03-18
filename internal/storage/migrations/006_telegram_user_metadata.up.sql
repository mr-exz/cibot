CREATE TABLE telegram_user_metadata (
    user_id     INTEGER PRIMARY KEY,
    username    TEXT,
    first_name  TEXT NOT NULL DEFAULT '',
    last_name   TEXT NOT NULL DEFAULT '',
    last_seen_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_tum_username ON telegram_user_metadata(username);
