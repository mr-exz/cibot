CREATE TABLE IF NOT EXISTS tech_threads (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    linear_issue_id     TEXT    NOT NULL,
    linear_issue_url    TEXT    NOT NULL,
    tech_chat_id        INTEGER NOT NULL,
    tech_thread_id      INTEGER NOT NULL,
    source_chat_id      INTEGER NOT NULL,
    source_thread_id    INTEGER NOT NULL DEFAULT 0,
    file_path           TEXT    NOT NULL,
    created_by_user_id  INTEGER NOT NULL,
    created_at          DATETIME NOT NULL DEFAULT (datetime('now')),
    closed_at           DATETIME
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tech_threads_topic
    ON tech_threads(tech_chat_id, tech_thread_id);
