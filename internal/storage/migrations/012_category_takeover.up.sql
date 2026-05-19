CREATE TABLE IF NOT EXISTS category_takeover (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    category_id       INTEGER NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    support_person_id INTEGER NOT NULL REFERENCES support_persons(id) ON DELETE CASCADE,
    set_by            TEXT NOT NULL,
    from_date         TEXT NOT NULL,
    until_date        TEXT NOT NULL,
    created_at        DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_category_takeover_cat_date
    ON category_takeover(category_id, from_date, until_date);
