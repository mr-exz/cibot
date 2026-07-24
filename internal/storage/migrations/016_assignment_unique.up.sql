-- A person assigned to the same category twice appears twice in the rotation
-- pool, which makes the turn generator hand every turn to that person forever.
-- Remove any existing duplicates (keeping the newest row, since rotation_type
-- and start_date are read from the row with the highest id), then guarantee
-- uniqueness going forward.
DELETE FROM support_assignments
WHERE id NOT IN (
    SELECT MAX(id) FROM support_assignments GROUP BY category_id, support_person_id
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_assignments_cat_person
    ON support_assignments(category_id, support_person_id);
