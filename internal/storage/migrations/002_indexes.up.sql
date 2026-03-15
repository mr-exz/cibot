CREATE INDEX IF NOT EXISTS idx_categories_ctx       ON categories(chat_id, thread_id);
CREATE INDEX IF NOT EXISTS idx_assignments_category ON support_assignments(category_id);
CREATE INDEX IF NOT EXISTS idx_assignments_person   ON support_assignments(support_person_id);
CREATE INDEX IF NOT EXISTS idx_cat_req_types_cat    ON category_request_types(category_id);
CREATE INDEX IF NOT EXISTS idx_group_topics_chat    ON group_topics(chat_id);
