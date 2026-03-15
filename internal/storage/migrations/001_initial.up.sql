CREATE TABLE IF NOT EXISTS categories (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    name             TEXT NOT NULL,
    emoji            TEXT NOT NULL DEFAULT '',
    linear_team_key  TEXT NOT NULL,
    chat_id          INTEGER,
    thread_id        INTEGER
);

CREATE TABLE IF NOT EXISTS request_types (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS category_request_types (
    category_id     INTEGER NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    request_type_id INTEGER NOT NULL REFERENCES request_types(id) ON DELETE CASCADE,
    PRIMARY KEY (category_id, request_type_id)
);

CREATE TABLE IF NOT EXISTS support_persons (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    name                 TEXT NOT NULL,
    telegram_username    TEXT NOT NULL UNIQUE,
    linear_username      TEXT NOT NULL,
    timezone             TEXT,
    work_hours           TEXT,
    work_days            TEXT
);

CREATE TABLE IF NOT EXISTS support_assignments (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    category_id       INTEGER NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    support_person_id INTEGER NOT NULL REFERENCES support_persons(id) ON DELETE CASCADE,
    rotation_type     TEXT NOT NULL CHECK(rotation_type IN ('daily','weekly')),
    start_date        TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS group_topics (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id     INTEGER NOT NULL,
    thread_id   INTEGER NOT NULL,
    topic_name  TEXT,
    UNIQUE(chat_id, thread_id)
);
