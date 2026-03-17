package msglog

import (
	"encoding/csv"
	"io"
	"os"
	"strconv"
	"sync"
	"time"
)

// Entry holds metadata for a single captured message.
type Entry struct {
	Timestamp  time.Time
	ChatID     int64
	ChatTitle  string
	ThreadID   int
	TopicName  string
	UserID     int64
	Username   string
	FirstName  string
	LastName   string
	MessageID  int
	Text       string // message text or "[photo]", "[video]", etc.
	ChatType   string // "private", "group", "supergroup"
}

// Logger appends message entries to a CSV file.
type Logger struct {
	mu   sync.Mutex
	path string
}

var header = []string{
	"timestamp", "chat_type", "chat_id", "chat_title", "thread_id", "topic_name",
	"user_id", "username", "first_name", "last_name", "message_id", "text",
}

func New(path string) *Logger {
	return &Logger{path: path}
}

// Log appends one entry to the CSV. Creates the file and writes the header if it doesn't exist yet.
func (l *Logger) Log(e Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write header if file is empty
	info, _ := f.Stat()
	needHeader := info.Size() == 0

	w := csv.NewWriter(f)
	if needHeader {
		if err := w.Write(header); err != nil {
			return err
		}
	}

	row := []string{
		e.Timestamp.UTC().Format(time.RFC3339),
		e.ChatType,
		strconv.FormatInt(e.ChatID, 10),
		e.ChatTitle,
		strconv.Itoa(e.ThreadID),
		e.TopicName,
		strconv.FormatInt(e.UserID, 10),
		e.Username,
		e.FirstName,
		e.LastName,
		strconv.Itoa(e.MessageID),
		e.Text,
	}
	if err := w.Write(row); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

// ExportAndTruncate returns the current CSV contents and resets the file.
// The returned []byte includes the header row so it's a valid standalone CSV.
func (l *Logger) ExportAndTruncate() ([]byte, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := os.ReadFile(l.path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	// If file doesn't exist or is empty, return just the header
	if len(data) == 0 {
		pr, pw := io.Pipe()
		go func() {
			w := csv.NewWriter(pw)
			_ = w.Write(header)
			w.Flush()
			pw.Close()
		}()
		return io.ReadAll(pr)
	}

	// Truncate: rewrite with only the header
	f, err := os.OpenFile(l.path, os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return data, err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	_ = w.Write(header)
	w.Flush()

	return data, nil
}

// Path returns the configured file path.
func (l *Logger) Path() string {
	return l.path
}
