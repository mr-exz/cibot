CREATE TABLE IF NOT EXISTS person_status (
    support_person_id INTEGER PRIMARY KEY REFERENCES support_persons(id) ON DELETE CASCADE,
    status            TEXT NOT NULL,
    set_at            TEXT NOT NULL
);
