-- Materialized rotation schedule.
-- Each row is a rotation "turn": the person on duty from turn_start (inclusive)
-- until the next turn's turn_start (exclusive). The on-duty base person for any
-- date D is the row with the greatest turn_start <= D for that category.
--
-- Past turns are immutable history; only turns on or after "today" are ever
-- rewritten (when the roster changes or when the next week is generated).
CREATE TABLE IF NOT EXISTS rotation_schedule (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    category_id       INTEGER NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    turn_start        TEXT NOT NULL,   -- inclusive start date, YYYY-MM-DD
    support_person_id INTEGER NOT NULL REFERENCES support_persons(id),
    UNIQUE(category_id, turn_start)
);

CREATE INDEX IF NOT EXISTS idx_rotation_schedule_lookup
    ON rotation_schedule(category_id, turn_start);
