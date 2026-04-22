package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type Category struct {
	ID            int64
	Name          string
	Emoji         string
	LinearTeamKey string
	ChatID        *int64 // nil = global (visible in all topics)
	ThreadID      *int   // nil = global
}

type RequestType struct {
	ID   int64
	Name string
}

type SupportPerson struct {
	ID               int64
	Name             string
	TelegramUsername string
	LinearUsername   string
	Timezone         string // "+02:00" or ""
	WorkHours        string // "08:30-18:30" or ""
	WorkDays         string // "1-5" or ""
	Status           string // "lunch", "brb", "away", or "" (no override)
}

type OnDutyResult struct {
	Person *SupportPerson
	Online bool
}

type CategoryDuty struct {
	Category Category
	Person   *SupportPerson
	Online   bool
}

type DB struct {
	db *sql.DB
}

func New(ctx context.Context, path string) (*DB, error) {
	// Open SQLite database
	dbConn, err := sql.Open("sqlite", "file:"+path+"?cache=shared&mode=rwc")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := dbConn.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Enable WAL mode and foreign keys
	if _, err := dbConn.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("failed to enable WAL: %w", err)
	}
	if _, err := dbConn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	db := &DB{db: dbConn}

	// Run migrations
	srcDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		dbConn.Close()
		return nil, fmt.Errorf("migrations source: %w", err)
	}
	dbDriver, err := sqlite.WithInstance(dbConn, &sqlite.Config{})
	if err != nil {
		dbConn.Close()
		return nil, fmt.Errorf("migrations db driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", srcDriver, "sqlite", dbDriver)
	if err != nil {
		dbConn.Close()
		return nil, fmt.Errorf("migrations init: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		dbConn.Close()
		return nil, fmt.Errorf("migrations failed: %w", err)
	}
	log.Printf("✓ Database migrations applied")

	log.Printf("✓ Database initialized at %s", path)
	return db, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

// === Topics ===

// SaveTopic persists a discovered topic to the database
func (d *DB) SaveTopic(ctx context.Context, chatID int64, threadID int, topicName string) error {
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO group_topics (chat_id, thread_id, topic_name)
		 VALUES (?, ?, ?)
		 ON CONFLICT(chat_id, thread_id) DO UPDATE SET topic_name = excluded.topic_name`,
		chatID, threadID, topicName)
	return err
}

// LoadTopicsForChat loads all topics for a specific chat from database
func (d *DB) LoadTopicsForChat(ctx context.Context, chatID int64) (map[int]string, error) {
	rows, err := d.db.QueryContext(ctx,
		"SELECT thread_id, topic_name FROM group_topics WHERE chat_id = ? ORDER BY thread_id",
		chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	topics := make(map[int]string)
	for rows.Next() {
		var threadID int
		var topicName string
		if err := rows.Scan(&threadID, &topicName); err != nil {
			return nil, err
		}
		topics[threadID] = topicName
	}
	return topics, rows.Err()
}

// LoadAllTopics loads all topics from every known chat from the database
func (d *DB) LoadAllTopics(ctx context.Context) (map[int64]map[int]string, error) {
	rows, err := d.db.QueryContext(ctx,
		"SELECT chat_id, thread_id, topic_name FROM group_topics ORDER BY chat_id, thread_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64]map[int]string)
	for rows.Next() {
		var chatID int64
		var threadID int
		var topicName string
		if err := rows.Scan(&chatID, &threadID, &topicName); err != nil {
			return nil, err
		}
		if result[chatID] == nil {
			result[chatID] = make(map[int]string)
		}
		result[chatID][threadID] = topicName
	}
	return result, rows.Err()
}

// === Categories ===

func (d *DB) ListCategories(ctx context.Context) ([]Category, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT id, name, emoji, linear_team_key, chat_id, thread_id FROM categories ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []Category
	for rows.Next() {
		var cat Category
		if err := rows.Scan(&cat.ID, &cat.Name, &cat.Emoji, &cat.LinearTeamKey, &cat.ChatID, &cat.ThreadID); err != nil {
			return nil, err
		}
		cats = append(cats, cat)
	}
	return cats, rows.Err()
}

// ListCategoriesForContext returns categories visible in a specific context (topic or main group)
// threadID=0 (main group): returns only globally-visible categories (topic_id NULL)
// threadID>0 (forum topic): returns globally-visible + topic-specific categories
func (d *DB) ListCategoriesForContext(ctx context.Context, chatID int64, threadID int) ([]Category, error) {
	var query string
	var args []interface{}

	if threadID == 0 {
		// Main group (no topic): global + group-level categories for this chat
		query = `SELECT id, name, emoji, linear_team_key, chat_id, thread_id
				 FROM categories
				 WHERE (chat_id IS NULL AND thread_id IS NULL)
				    OR (chat_id = ? AND thread_id IS NULL)
				 ORDER BY id`
		args = []interface{}{chatID}
	} else {
		// Forum topic: global + group-level + this topic's categories
		query = `SELECT id, name, emoji, linear_team_key, chat_id, thread_id
				 FROM categories
				 WHERE (chat_id IS NULL AND thread_id IS NULL)
				    OR (chat_id = ? AND thread_id IS NULL)
				    OR (chat_id = ? AND thread_id = ?)
				 ORDER BY id`
		args = []interface{}{chatID, chatID, threadID}
	}

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []Category
	for rows.Next() {
		var cat Category
		if err := rows.Scan(&cat.ID, &cat.Name, &cat.Emoji, &cat.LinearTeamKey, &cat.ChatID, &cat.ThreadID); err != nil {
			return nil, err
		}
		cats = append(cats, cat)
	}
	return cats, rows.Err()
}

// ListConfiguredTopicsForChat returns topic names that have at least one category scoped to them in the given chat.
func (d *DB) ListConfiguredTopicsForChat(ctx context.Context, chatID int64) ([]string, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT gt.topic_name
		 FROM categories c
		 JOIN group_topics gt ON gt.chat_id = c.chat_id AND gt.thread_id = c.thread_id
		 WHERE c.chat_id = ? AND c.thread_id IS NOT NULL
		 GROUP BY gt.thread_id
		 ORDER BY gt.topic_name`,
		chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func (d *DB) AddCategory(ctx context.Context, name, emoji, teamKey string) (int64, error) {
	return d.AddCategoryWithTopic(ctx, name, emoji, teamKey, nil, nil)
}

func (d *DB) AddCategoryWithTopic(ctx context.Context, name, emoji, teamKey string, chatID *int64, threadID *int) (int64, error) {
	result, err := d.db.ExecContext(ctx,
		"INSERT INTO categories (name, emoji, linear_team_key, chat_id, thread_id) VALUES (?, ?, ?, ?, ?)",
		name, emoji, teamKey, chatID, threadID)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (d *DB) GetCategory(ctx context.Context, categoryID int64) (*Category, error) {
	var cat Category
	err := d.db.QueryRowContext(ctx,
		"SELECT id, name, emoji, linear_team_key, chat_id, thread_id FROM categories WHERE id = ?",
		categoryID).Scan(&cat.ID, &cat.Name, &cat.Emoji, &cat.LinearTeamKey, &cat.ChatID, &cat.ThreadID)
	if err != nil {
		return nil, err
	}
	return &cat, nil
}

func (d *DB) UpdateCategoryScope(ctx context.Context, categoryID int64, chatID *int64, threadID *int) error {
	_, err := d.db.ExecContext(ctx,
		"UPDATE categories SET chat_id = ?, thread_id = ? WHERE id = ?",
		chatID, threadID, categoryID)
	return err
}

func (d *DB) DeleteCategory(ctx context.Context, categoryID int64) error {
	_, err := d.db.ExecContext(ctx, "DELETE FROM categories WHERE id = ?", categoryID)
	return err
}

// CloneCategory creates a new category with the same name/emoji as the source but a new scope
// and optionally a new Linear team key. All request type links are copied to the new category.
func (d *DB) CloneCategory(ctx context.Context, sourceCatID int64, chatID *int64, threadID *int, teamKey string) (int64, error) {
	src, err := d.GetCategory(ctx, sourceCatID)
	if err != nil {
		return 0, fmt.Errorf("source category not found: %w", err)
	}

	result, err := d.db.ExecContext(ctx,
		"INSERT INTO categories (name, emoji, linear_team_key, chat_id, thread_id) VALUES (?, ?, ?, ?, ?)",
		src.Name, src.Emoji, teamKey, chatID, threadID)
	if err != nil {
		return 0, err
	}
	newCatID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	// Copy all type links from source category
	types, err := d.ListRequestTypesForCategory(ctx, sourceCatID)
	if err != nil {
		return newCatID, err
	}
	for _, rt := range types {
		if _, err := d.db.ExecContext(ctx,
			"INSERT OR IGNORE INTO category_request_types (category_id, request_type_id) VALUES (?, ?)",
			newCatID, rt.ID); err != nil {
			return newCatID, err
		}
	}
	return newCatID, nil
}

// === Request Types ===

func (d *DB) ListAllRequestTypes(ctx context.Context) ([]RequestType, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT id, name FROM request_types ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var types []RequestType
	for rows.Next() {
		var rt RequestType
		if err := rows.Scan(&rt.ID, &rt.Name); err != nil {
			return nil, err
		}
		types = append(types, rt)
	}
	return types, rows.Err()
}

func (d *DB) ListRequestTypesForCategory(ctx context.Context, categoryID int64) ([]RequestType, error) {
	rows, err := d.db.QueryContext(ctx,
		"SELECT rt.id, rt.name FROM request_types rt "+
			"INNER JOIN category_request_types crt ON rt.id = crt.request_type_id "+
			"WHERE crt.category_id = ? ORDER BY rt.id", categoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var types []RequestType
	for rows.Next() {
		var rt RequestType
		if err := rows.Scan(&rt.ID, &rt.Name); err != nil {
			return nil, err
		}
		types = append(types, rt)
	}
	return types, rows.Err()
}

func (d *DB) GetRequestType(ctx context.Context, typeID int64) (*RequestType, error) {
	var rt RequestType
	err := d.db.QueryRowContext(ctx,
		"SELECT id, name FROM request_types WHERE id = ?", typeID).Scan(&rt.ID, &rt.Name)
	if err != nil {
		return nil, err
	}
	return &rt, nil
}

func (d *DB) AddRequestType(ctx context.Context, name string) (int64, error) {
	// Insert if not exists; ignore duplicate name
	_, err := d.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO request_types (name) VALUES (?)", name)
	if err != nil {
		return 0, err
	}

	// Always SELECT — LastInsertId is unreliable after INSERT OR IGNORE
	// (go-sqlite3 returns the last connection rowid, not the ignored row)
	var id int64
	err = d.db.QueryRowContext(ctx,
		"SELECT id FROM request_types WHERE name = ?", name).Scan(&id)
	return id, err
}

func (d *DB) LinkRequestTypeToCategory(ctx context.Context, categoryID, typeID int64) error {
	_, err := d.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO category_request_types (category_id, request_type_id) VALUES (?, ?)",
		categoryID, typeID)
	return err
}

// === Support Persons ===

func (d *DB) AddSupportPerson(ctx context.Context, name, telegramUsername, linearUsername string) (int64, error) {
	return d.AddSupportPersonFull(ctx, name, telegramUsername, linearUsername, "", "", "")
}

func (d *DB) AddSupportPersonFull(ctx context.Context, name, telegramUsername, linearUsername, timezone, workHours, workDays string) (int64, error) {
	result, err := d.db.ExecContext(ctx,
		"INSERT INTO support_persons (name, telegram_username, linear_username, timezone, work_hours, work_days) VALUES (?, ?, ?, ?, ?, ?)",
		name, telegramUsername, linearUsername, timezone, workHours, workDays)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (d *DB) SetPersonWorkHours(ctx context.Context, telegramUsername, timezone, workHours, workDays string) error {
	_, err := d.db.ExecContext(ctx,
		"UPDATE support_persons SET timezone = ?, work_hours = ?, work_days = ? WHERE telegram_username = ?",
		timezone, workHours, workDays, telegramUsername)
	return err
}

func (d *DB) ListSupportPersonsForCategory(ctx context.Context, categoryID int64) ([]SupportPerson, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT sp.id, sp.name, sp.telegram_username, sp.linear_username, sp.timezone, sp.work_hours, sp.work_days, COALESCE(ps.status, '')
		 FROM support_persons sp
		 INNER JOIN support_assignments sa ON sp.id = sa.support_person_id
		 LEFT JOIN person_status ps ON sp.id = ps.support_person_id
		 WHERE sa.category_id = ? ORDER BY sp.id`, categoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var persons []SupportPerson
	for rows.Next() {
		var sp SupportPerson
		if err := rows.Scan(&sp.ID, &sp.Name, &sp.TelegramUsername, &sp.LinearUsername, &sp.Timezone, &sp.WorkHours, &sp.WorkDays, &sp.Status); err != nil {
			return nil, err
		}
		persons = append(persons, sp)
	}
	return persons, rows.Err()
}

func (d *DB) ListAllSupportPersons(ctx context.Context) ([]SupportPerson, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT sp.id, sp.name, sp.telegram_username, sp.linear_username, sp.timezone, sp.work_hours, sp.work_days, COALESCE(ps.status, '')
		 FROM support_persons sp
		 LEFT JOIN person_status ps ON sp.id = ps.support_person_id
		 ORDER BY sp.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var persons []SupportPerson
	for rows.Next() {
		var sp SupportPerson
		if err := rows.Scan(&sp.ID, &sp.Name, &sp.TelegramUsername, &sp.LinearUsername, &sp.Timezone, &sp.WorkHours, &sp.WorkDays, &sp.Status); err != nil {
			return nil, err
		}
		persons = append(persons, sp)
	}
	return persons, rows.Err()
}

func (d *DB) GetSupportPersonByTelegramUsername(ctx context.Context, username string) (*SupportPerson, error) {
	var sp SupportPerson
	err := d.db.QueryRowContext(ctx,
		`SELECT sp.id, sp.name, sp.telegram_username, sp.linear_username, sp.timezone, sp.work_hours, sp.work_days, COALESCE(ps.status, '')
		 FROM support_persons sp
		 LEFT JOIN person_status ps ON sp.id = ps.support_person_id
		 WHERE sp.telegram_username = ?`,
		username).Scan(&sp.ID, &sp.Name, &sp.TelegramUsername, &sp.LinearUsername, &sp.Timezone, &sp.WorkHours, &sp.WorkDays, &sp.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sp, nil
}

func (d *DB) SetPersonStatus(ctx context.Context, personID int64, status string) error {
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO person_status (support_person_id, status, set_at)
		 VALUES (?, ?, datetime('now'))
		 ON CONFLICT(support_person_id) DO UPDATE SET status = excluded.status, set_at = excluded.set_at`,
		personID, status)
	return err
}

func (d *DB) ClearPersonStatus(ctx context.Context, personID int64) error {
	_, err := d.db.ExecContext(ctx, "DELETE FROM person_status WHERE support_person_id = ?", personID)
	return err
}

// === Assignments & Rotation ===

func (d *DB) SetRotation(ctx context.Context, categoryID int64, rotationType string) error {
	// Update all assignments for this category with the new rotation type
	_, err := d.db.ExecContext(ctx,
		"UPDATE support_assignments SET rotation_type = ? WHERE category_id = ?",
		rotationType, categoryID)
	return err
}

func (d *DB) GetOnDutyPerson(ctx context.Context, categoryID int64, today time.Time) (*SupportPerson, error) {
	result, err := d.GetOnDutyPersonResult(ctx, categoryID, today)
	if err != nil {
		return nil, err
	}
	return result.Person, nil
}

func (d *DB) GetOnDutyPersonResult(ctx context.Context, categoryID int64, now time.Time) (*OnDutyResult, error) {
	// Get all support persons for the category
	pool, err := d.ListSupportPersonsForCategory(ctx, categoryID)
	if err != nil {
		return nil, err
	}
	if len(pool) == 0 {
		return nil, fmt.Errorf("no support persons assigned to category %d", categoryID)
	}

	// Get the rotation assignment for this category
	var rotationType string
	var startDate string
	err = d.db.QueryRowContext(ctx,
		"SELECT rotation_type, start_date FROM support_assignments "+
			"WHERE category_id = ? ORDER BY id DESC LIMIT 1",
		categoryID).Scan(&rotationType, &startDate)
	if err != nil {
		return nil, err
	}

	// Parse start date
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, fmt.Errorf("invalid start_date format: %w", err)
	}

	// Calculate rotation period in days
	period := 1 // daily
	if rotationType == "weekly" {
		period = 7
	}

	// Calculate which person is on duty
	daysElapsed := int(now.Sub(start) / (24 * time.Hour))
	slot := (daysElapsed / period) % len(pool)

	// Walk forward from slot, find first online person
	for i := 0; i < len(pool); i++ {
		candidate := pool[(slot+i)%len(pool)]
		if IsPersonOnline(candidate, now) {
			return &OnDutyResult{Person: &candidate, Online: true}, nil
		}
	}

	// Nobody online: return slot person with Online=false
	return &OnDutyResult{Person: &pool[slot], Online: false}, nil
}

func (d *DB) ListAllOnDuty(ctx context.Context, today time.Time) ([]CategoryDuty, error) {
	categories, err := d.ListCategories(ctx)
	if err != nil {
		return nil, err
	}

	var duties []CategoryDuty
	for _, cat := range categories {
		result, err := d.GetOnDutyPersonResult(ctx, cat.ID, today)
		if err != nil {
			// Skip categories with no assigned persons
			log.Printf("⚠️  No on-duty person for category %s: %v", cat.Name, err)
			continue
		}
		duties = append(duties, CategoryDuty{
			Category: cat,
			Person:   result.Person,
			Online:   result.Online,
		})
	}

	return duties, nil
}

// === Group Chats ===

type GroupChat struct {
	ChatID   int64
	Title    string
	Approved bool
	AddedAt  string
}

// RegisterGroup adds a group to the database if not already present (unapproved by default).
func (d *DB) RegisterGroup(ctx context.Context, chatID int64, title string) error {
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO group_chats (chat_id, title) VALUES (?, ?)
		 ON CONFLICT(chat_id) DO UPDATE SET title = excluded.title`,
		chatID, title)
	return err
}

// SetGroupApproved sets the approved state for a group.
func (d *DB) SetGroupApproved(ctx context.Context, chatID int64, approved bool) error {
	v := 0
	if approved {
		v = 1
	}
	_, err := d.db.ExecContext(ctx, "UPDATE group_chats SET approved = ? WHERE chat_id = ?", v, chatID)
	return err
}

// IsGroupApproved returns true if the group is approved.
func (d *DB) IsGroupApproved(ctx context.Context, chatID int64) (bool, error) {
	var approved int
	err := d.db.QueryRowContext(ctx, "SELECT approved FROM group_chats WHERE chat_id = ?", chatID).Scan(&approved)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return approved == 1, err
}

// ListApprovedGroupIDs returns the chat IDs of all approved groups.
func (d *DB) ListApprovedGroupIDs(ctx context.Context) ([]int64, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT chat_id FROM group_chats WHERE approved = 1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListGroups returns all known groups ordered by approved desc, added_at asc.
func (d *DB) ListGroups(ctx context.Context) ([]GroupChat, error) {
	rows, err := d.db.QueryContext(ctx,
		"SELECT chat_id, title, approved, added_at FROM group_chats ORDER BY approved DESC, added_at ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []GroupChat
	for rows.Next() {
		var g GroupChat
		var approved int
		if err := rows.Scan(&g.ChatID, &g.Title, &approved, &g.AddedAt); err != nil {
			return nil, err
		}
		g.Approved = approved == 1
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// === User Labels ===

func (d *DB) SetUserLabel(ctx context.Context, telegramUsername, label string) error {
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO user_labels (telegram_username, label) VALUES (?, ?)
		 ON CONFLICT(telegram_username) DO UPDATE SET label = excluded.label`,
		telegramUsername, label)
	return err
}

func (d *DB) GetUserLabel(ctx context.Context, telegramUsername string) (string, error) {
	var label string
	err := d.db.QueryRowContext(ctx,
		"SELECT label FROM user_labels WHERE telegram_username = ?", telegramUsername).Scan(&label)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return label, err
}

// === Telegram User Metadata ===

type TelegramUser struct {
	UserID         int64
	Username       string
	FirstName      string
	LastName       string
	LinearUsername string
}

// UpsertUserMeta stores or updates a user's profile data seen from any message.
func (d *DB) UpsertUserMeta(ctx context.Context, userID int64, username, firstName, lastName string) error {
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO telegram_user_metadata (user_id, username, first_name, last_name, last_seen_at)
		 VALUES (?, ?, ?, ?, datetime('now'))
		 ON CONFLICT(user_id) DO UPDATE SET
		     username     = excluded.username,
		     first_name   = excluded.first_name,
		     last_name    = excluded.last_name,
		     last_seen_at = excluded.last_seen_at`,
		userID, username, firstName, lastName)
	return err
}

// LookupUserByUsername returns the stored user_id for a given @username, or 0 if not found.
func (d *DB) LookupUserByUsername(ctx context.Context, username string) (int64, error) {
	var userID int64
	err := d.db.QueryRowContext(ctx,
		"SELECT user_id FROM telegram_user_metadata WHERE username = ? LIMIT 1", username).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return userID, err
}

// GetUserByID returns a single user by their Telegram user_id, or nil if not found.
func (d *DB) GetUserByID(ctx context.Context, userID int64) (*TelegramUser, error) {
	var u TelegramUser
	err := d.db.QueryRowContext(ctx,
		"SELECT user_id, COALESCE(username,''), first_name, last_name, COALESCE(linear_username,'') FROM telegram_user_metadata WHERE user_id = ?",
		userID).Scan(&u.UserID, &u.Username, &u.FirstName, &u.LastName, &u.LinearUsername)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// ListUsers returns a page of known users ordered by last_seen_at desc.
func (d *DB) ListUsers(ctx context.Context, limit, offset int) ([]TelegramUser, error) {
	rows, err := d.db.QueryContext(ctx,
		"SELECT user_id, COALESCE(username,''), first_name, last_name, COALESCE(linear_username,'') FROM telegram_user_metadata ORDER BY last_seen_at DESC LIMIT ? OFFSET ?",
		limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []TelegramUser
	for rows.Next() {
		var u TelegramUser
		if err := rows.Scan(&u.UserID, &u.Username, &u.FirstName, &u.LastName, &u.LinearUsername); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// CountUsers returns the total number of known users.
func (d *DB) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM telegram_user_metadata").Scan(&n)
	return n, err
}

// GetUserLinearUsername returns the stored Linear username for a Telegram user_id, or "" if not set.
func (d *DB) GetUserLinearUsername(ctx context.Context, userID int64) (string, error) {
	var username string
	err := d.db.QueryRowContext(ctx,
		"SELECT COALESCE(linear_username,'') FROM telegram_user_metadata WHERE user_id = ?", userID).Scan(&username)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return username, err
}

// SetUserLinearUsername stores or updates the Linear username for a Telegram user_id.
func (d *DB) SetUserLinearUsername(ctx context.Context, userID int64, linearUsername string) error {
	_, err := d.db.ExecContext(ctx,
		"UPDATE telegram_user_metadata SET linear_username = ? WHERE user_id = ?",
		linearUsername, userID)
	return err
}

// === Helper to create initial assignment ===

func (d *DB) CreateInitialAssignment(ctx context.Context, categoryID int64, supportPersonID int64, rotationType string, startDate string) error {
	_, err := d.db.ExecContext(ctx,
		"INSERT INTO support_assignments (category_id, support_person_id, rotation_type, start_date) VALUES (?, ?, ?, ?)",
		categoryID, supportPersonID, rotationType, startDate)
	return err
}
