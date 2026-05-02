package telegram

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/mr-exz/cibot/internal/storage"
)

// activeThread pairs a TechThread with a WaitGroup tracking in-flight media downloads.
type activeThread struct {
	tt        *storage.TechThread
	downloads sync.WaitGroup
}

// techThreadKey returns the map key used for the in-memory tech thread cache.
func techThreadKey(chatID int64, threadID int) string {
	return fmt.Sprintf("%d:%d", chatID, threadID)
}

// handleThread starts the /thread flow. Shows a category picker — the selected
// category's Linear team key is used to create the issue.
func (h *Handler) handleThread(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	if h.cfg.TechGroupID == 0 {
		h.sendMessage(ctx, b, msg, "⚠️ TECH_GROUP_ID is not configured. Ask the administrator to set it.")
		return
	}
	if msg.ReplyToMessage == nil {
		h.sendMessage(ctx, b, msg, "⚠️ /thread requires a reply. Reply to a message to open a technical thread for it.")
		return
	}
	if msg.MessageThreadID != 0 && msg.ReplyToMessage.ID == msg.MessageThreadID {
		h.sendMessage(ctx, b, msg, "⚠️ /thread requires a reply to a user message, not the topic header.")
		return
	}

	replied := msg.ReplyToMessage
	body := replied.Text
	if body == "" {
		body = replied.Caption
	}

	var ticketMedia []*threadMedia
	if meta := extractThreadMedia(replied); meta != nil {
		ticketMedia = append(ticketMedia, meta)
	}

	reporterName := ""
	reporterUsername := ""
	if replied.From != nil {
		reporterName = strings.TrimSpace(replied.From.FirstName + " " + replied.From.LastName)
		reporterUsername = replied.From.Username
	}

	categories, err := h.storage.ListCategoriesForContext(ctx, msg.Chat.ID, msg.MessageThreadID)
	if err != nil {
		h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ Failed to load categories: %v", err))
		return
	}
	if len(categories) == 0 {
		h.sendMessage(ctx, b, msg, "⚠️ No categories configured for this context.")
		return
	}

	sentMsg, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		MessageThreadID: msg.MessageThreadID,
		Text:            "🗂️ Select category for this thread:",
		ReplyMarkup:     buildCategoryKeyboard(categories),
	})
	if err != nil {
		log.Printf("❌ /thread: failed to send message: %v", err)
		return
	}

	session := &pendingSession{
		Flow:             FlowThread,
		Step:             StepCategory,
		UserID:           msg.From.ID,
		ChatID:           msg.Chat.ID,
		ThreadID:         msg.MessageThreadID,
		MessageID:        sentMsg.ID,
		CreatedAt:        time.Now(),
		TicketMsgLink:    formatTelegramLink(msg.Chat.ID, msg.MessageThreadID, replied.ID),
		TicketMsgBody:    body,
		TicketMsgDate:    time.Unix(int64(replied.Date), 0),
		SourceMsgID:      replied.ID,
		TicketMedia:      ticketMedia,
		ReporterName:     reporterName,
		ReporterUsername: reporterUsername,
	}

	key := stateKey{UserID: msg.From.ID}
	h.mu.Lock()
	h.states[key] = session
	h.mu.Unlock()
}

// completeTechThread is called from handleCategoryCallback when FlowThread.
// It creates the Linear issue + Telegram topic + thread file and saves the DB record.
func (h *Handler) completeTechThread(ctx context.Context, b *tgbot.Bot, pending *pendingSession) {
	reporter := pending.ReporterName
	if pending.ReporterUsername != "" {
		reporter = fmt.Sprintf("[%s](https://t.me/%s)", pending.ReporterName, pending.ReporterUsername)
	}

	title := ticketTitle(pending.TicketMsgBody, pending.TicketMsgDate)
	if strings.TrimSpace(pending.TicketMsgBody) == "" {
		title = fmt.Sprintf("Thread from Telegram (%s)", pending.TicketMsgDate.Format("2006-01-02 15:04"))
	}

	mediaMarkdown := uploadTicketMedia(ctx, b, h, pending.TicketMedia)
	description := fmt.Sprintf("**Reporter:** %s\n**Category:** %s\n**Source:** %s\n\n%s%s",
		reporter, pending.CategoryName, pending.TicketMsgLink, pending.TicketMsgBody, mediaMarkdown)

	onDutyResult, err := h.storage.GetOnDutyPersonResult(ctx, pending.CategoryID, time.Now())
	if err != nil {
		log.Printf("⚠️  Failed to get on-duty person for thread: %v", err)
		onDutyResult = nil
	}

	assignee := ""
	if onDutyResult != nil && onDutyResult.Person != nil {
		assignee = onDutyResult.Person.LinearUsername
	}
	if onDutyResult != nil && !onDutyResult.Online {
		description += "\n\n⚠️ **Note:** Assigned person is currently outside working hours."
	}

	issue, err := h.linear.CreateIssue(ctx, title, description, pending.TeamKey, assignee, nil, 0)
	if err != nil {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    pending.ChatID,
			MessageID: pending.MessageID,
			Text:      fmt.Sprintf("❌ Failed to create Linear issue: %v", err),
		})
		return
	}

	topic, err := b.CreateForumTopic(ctx, &tgbot.CreateForumTopicParams{
		ChatID: h.cfg.TechGroupID,
		Name:   issue.Identifier,
	})
	if err != nil {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    pending.ChatID,
			MessageID: pending.MessageID,
			Text:      fmt.Sprintf("❌ Failed to create Telegram topic: %v", err),
		})
		return
	}

	b.ForwardMessage(ctx, &tgbot.ForwardMessageParams{
		ChatID:          h.cfg.TechGroupID,
		MessageThreadID: topic.MessageThreadID,
		FromChatID:      pending.ChatID,
		MessageID:       pending.SourceMsgID,
	})

	onCallLine := "On call: (unassigned)"
	var pingButton *models.InlineKeyboardButton
	if onDutyResult != nil && onDutyResult.Person != nil {
		p := onDutyResult.Person
		status := "🟢"
		if !onDutyResult.Online {
			status = "🔴"
		}
		onCallLine = fmt.Sprintf("On call: %s %s", p.Name, status)
		btn := models.InlineKeyboardButton{Text: "Ping " + p.Name, CallbackData: "ping:" + p.TelegramUsername}
		pingButton = &btn
	}

	topicMsgParams := &tgbot.SendMessageParams{
		ChatID:          h.cfg.TechGroupID,
		MessageThreadID: topic.MessageThreadID,
		Text:            fmt.Sprintf("Linear: %s\n%s\n\nUse /close when done to dump this thread to the issue.", issue.URL, onCallLine),
	}
	if pingButton != nil {
		topicMsgParams.ReplyMarkup = &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{{*pingButton}},
		}
	}
	b.SendMessage(ctx, topicMsgParams)

	folderPath := filepath.Join(filepath.Dir(h.cfg.CSVPath), issue.Identifier)
	if err := os.MkdirAll(folderPath, 0755); err != nil {
		log.Printf("⚠️  Failed to create thread folder: %v", err)
	}

	tt := &storage.TechThread{
		LinearIssueID:   issue.ID,
		LinearIssueURL:  issue.URL,
		TechChatID:      h.cfg.TechGroupID,
		TechThreadID:    topic.MessageThreadID,
		SourceChatID:    pending.ChatID,
		SourceThreadID:  pending.ThreadID,
		FilePath:        folderPath,
		CreatedByUserID: pending.UserID,
	}

	id, err := h.storage.CreateTechThread(ctx, tt)
	if err != nil {
		log.Printf("⚠️  Failed to save tech thread to DB: %v", err)
	}
	tt.ID = id

	h.mu.Lock()
	h.techThreads[techThreadKey(tt.TechChatID, tt.TechThreadID)] = &activeThread{tt: tt}
	delete(h.states, stateKey{UserID: pending.UserID})
	h.mu.Unlock()

	confirmText := "✅ Thread opened! Click \"Join tech group\" below to join and continue the conversation in the dedicated topic."
	if onDutyResult != nil && !onDutyResult.Online {
		confirmText += "\n\n⚠️ Assigned person is currently outside working hours."
	}

	topicLink := telegramTopicLink(h.cfg.TechGroupID, topic.MessageThreadID)
	inviteLink := h.getTechGroupInviteLink(ctx, b)

	buttons := []models.InlineKeyboardButton{
		{Text: "Linear: " + issue.Identifier, URL: issue.URL},
		{Text: "Telegram: " + issue.Identifier, URL: topicLink},
	}
	if inviteLink != "" {
		buttons = append(buttons, models.InlineKeyboardButton{Text: "Join tech group", URL: inviteLink})
	}

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    pending.ChatID,
		MessageID: pending.MessageID,
		Text:      confirmText,
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{buttons},
		},
	})
	log.Printf("✓ Tech thread created: %s (topic %d in chat %d)", issue.Identifier, topic.MessageThreadID, h.cfg.TechGroupID)
}

// handleCloseThread closes the current tech thread topic and dumps messages to Linear.
// Must be used inside a tech group topic.
func (h *Handler) handleCloseThread(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	if h.cfg.TechGroupID == 0 {
		h.sendMessage(ctx, b, msg, "⚠️ TECH_GROUP_ID is not configured.")
		return
	}
	if msg.Chat.ID != h.cfg.TechGroupID || msg.MessageThreadID == 0 {
		h.sendMessage(ctx, b, msg, "⚠️ /close must be used inside a tech thread topic.")
		return
	}

	h.mu.Lock()
	at := h.techThreads[techThreadKey(msg.Chat.ID, msg.MessageThreadID)]
	delete(h.techThreads, techThreadKey(msg.Chat.ID, msg.MessageThreadID))
	h.mu.Unlock()

	var tt *storage.TechThread
	var wg *sync.WaitGroup
	if at != nil {
		tt = at.tt
		wg = &at.downloads
	} else {
		// Bot restarted after thread creation — no in-flight downloads
		var err error
		tt, err = h.storage.GetTechThreadByTopic(ctx, msg.Chat.ID, msg.MessageThreadID)
		if err != nil {
			h.sendMessage(ctx, b, msg, fmt.Sprintf("❌ DB error: %v", err))
			return
		}
		if tt == nil {
			h.sendMessage(ctx, b, msg, "⚠️ No open thread found for this topic.")
			return
		}
		var empty sync.WaitGroup
		wg = &empty
	}

	if err := h.storage.CloseTechThread(ctx, tt.ID); err != nil {
		log.Printf("⚠️  Failed to close tech thread in DB: %v", err)
	}

	b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		MessageThreadID: msg.MessageThreadID,
		Text:            "✅ Thread closed. Uploading messages and files to Linear in the background...",
	})

	go h.uploadThreadData(b, tt, msg.Chat.ID, msg.MessageThreadID, msg.From.Username, wg)
	log.Printf("✓ Tech thread closing: %s (by @%s)", tt.LinearIssueURL, msg.From.Username)
}

// getTechGroupInviteLink returns a cached permanent invite link for the tech group,
// creating one via the API on first call.
func (h *Handler) getTechGroupInviteLink(ctx context.Context, b *tgbot.Bot) string {
	h.mu.Lock()
	cached := h.techGroupInviteLink
	h.mu.Unlock()
	if cached != "" {
		return cached
	}

	link, err := b.CreateChatInviteLink(ctx, &tgbot.CreateChatInviteLinkParams{
		ChatID: h.cfg.TechGroupID,
		Name:   "cibot",
	})
	if err != nil {
		log.Printf("⚠️  Failed to create tech group invite link: %v", err)
		return ""
	}

	h.mu.Lock()
	h.techGroupInviteLink = link.InviteLink
	h.mu.Unlock()
	return link.InviteLink
}

// telegramTopicLink builds a t.me deep link to a specific forum topic.
// Supergroup chat IDs have a -100 prefix that must be stripped for the URL.
func telegramTopicLink(chatID int64, threadID int) string {
	s := fmt.Sprintf("%d", -chatID) // make positive
	if len(s) > 3 && s[:3] == "100" {
		s = s[3:]
	}
	return fmt.Sprintf("https://t.me/c/%s/%d", s, threadID)
}

// appendToThreadFile appends a formatted message line to messages.txt inside the thread folder.
func appendToThreadFile(folderPath string, msg *models.Message) {
	if msg.From == nil {
		return
	}
	ts := time.Unix(int64(msg.Date), 0).UTC().Format("2006-01-02 15:04:05")
	name := strings.TrimSpace(msg.From.FirstName + " " + msg.From.LastName)
	username := msg.From.Username
	sender := name
	if username != "" {
		sender = fmt.Sprintf("%s (@%s)", name, username)
	}
	content := messageContent(msg)
	line := fmt.Sprintf("[%s] %s: %s\n", ts, sender, content)

	f, err := os.OpenFile(filepath.Join(folderPath, "messages.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("⚠️  thread file open: %v", err)
		return
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		log.Printf("⚠️  thread file write: %v", err)
	}
}

// threadMedia holds metadata about a media attachment extracted from a Telegram message.
type threadMedia struct {
	fileID      string
	fileSize    int64
	filename    string
	contentType string
}

const maxThreadMediaSize = 25 * 1024 * 1024 // 25 MB

// extractThreadMedia returns downloadable media metadata from a message, or nil if none.
func extractThreadMedia(msg *models.Message) *threadMedia {
	switch {
	case len(msg.Photo) > 0:
		p := msg.Photo[len(msg.Photo)-1]
		return &threadMedia{p.FileID, int64(p.FileSize), fmt.Sprintf("photo_%d.jpg", msg.ID), "image/jpeg"}
	case msg.Document != nil:
		name := msg.Document.FileName
		if name == "" {
			name = fmt.Sprintf("document_%d", msg.ID)
		}
		ct := msg.Document.MimeType
		if ct == "" {
			ct = "application/octet-stream"
		}
		return &threadMedia{msg.Document.FileID, msg.Document.FileSize, name, ct}
	case msg.Video != nil:
		name := msg.Video.FileName
		if name == "" {
			name = fmt.Sprintf("video_%d.mp4", msg.ID)
		}
		ct := msg.Video.MimeType
		if ct == "" {
			ct = "video/mp4"
		}
		return &threadMedia{msg.Video.FileID, msg.Video.FileSize, name, ct}
	case msg.Audio != nil:
		name := msg.Audio.FileName
		if name == "" {
			name = fmt.Sprintf("audio_%d.mp3", msg.ID)
		}
		ct := msg.Audio.MimeType
		if ct == "" {
			ct = "audio/mpeg"
		}
		return &threadMedia{msg.Audio.FileID, msg.Audio.FileSize, name, ct}
	case msg.Voice != nil:
		return &threadMedia{msg.Voice.FileID, msg.Voice.FileSize, fmt.Sprintf("voice_%d.ogg", msg.ID), "audio/ogg"}
	case msg.VideoNote != nil:
		return &threadMedia{msg.VideoNote.FileID, int64(msg.VideoNote.FileSize), fmt.Sprintf("videonote_%d.mp4", msg.ID), "video/mp4"}
	case msg.Animation != nil:
		ct := msg.Animation.MimeType
		if ct == "" {
			ct = "video/mp4"
		}
		return &threadMedia{msg.Animation.FileID, msg.Animation.FileSize, fmt.Sprintf("animation_%d.mp4", msg.ID), ct}
	}
	return nil
}

// downloadMediaToFolder downloads a media attachment from a Telegram message into the thread folder.
func (h *Handler) downloadMediaToFolder(ctx context.Context, b *tgbot.Bot, folder string, msg *models.Message) {
	meta := extractThreadMedia(msg)
	if meta == nil {
		return
	}
	if meta.fileSize > maxThreadMediaSize {
		log.Printf("⚠️  thread media: %s too large (%d bytes), skipping", meta.filename, meta.fileSize)
		return
	}

	file, err := b.GetFile(ctx, &tgbot.GetFileParams{FileID: meta.fileID})
	if err != nil {
		log.Printf("⚠️  thread media: GetFile failed: %v", err)
		return
	}

	dlURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", h.cfg.TelegramToken, file.FilePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		log.Printf("⚠️  thread media: request error: %v", err)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("⚠️  thread media: download failed: %v", err)
		return
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("⚠️  thread media: read failed: %v", err)
		return
	}

	if err := os.WriteFile(filepath.Join(folder, meta.filename), data, 0644); err != nil {
		log.Printf("⚠️  thread media: save failed: %v", err)
	}
}

// contentTypeForFile returns a MIME type for a filename based on its extension.
func contentTypeForFile(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".mp3":
		return "audio/mpeg"
	case ".ogg":
		return "audio/ogg"
	case ".pdf":
		return "application/pdf"
	}
	return "application/octet-stream"
}

// uploadThreadData runs in a goroutine after /close: posts messages as a Linear comment,
// uploads media files as attachments, closes the topic, and reports completion.
func (h *Handler) uploadThreadData(b *tgbot.Bot, tt *storage.TechThread, chatID int64, threadID int, closedBy string, wg *sync.WaitGroup) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Wait for in-flight media downloads to finish (bounded by the 60s context)
	waitDone := make(chan struct{})
	go func() { wg.Wait(); close(waitDone) }()
	select {
	case <-waitDone:
	case <-ctx.Done():
		log.Printf("⚠️  uploadThreadData: download wait timed out, proceeding with available files")
	}

	// Upload all media files first, collect asset URLs for embedding in the comment.
	type uploadedFile struct {
		name     string
		assetURL string
		isImage  bool
	}
	entries, _ := os.ReadDir(tt.FilePath)
	var uploads []uploadedFile
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "messages.txt" {
			continue
		}
		fileData, err := os.ReadFile(filepath.Join(tt.FilePath, entry.Name()))
		if err != nil {
			log.Printf("⚠️  uploadThreadData: read %s: %v", entry.Name(), err)
			continue
		}
		ct := contentTypeForFile(entry.Name())
		assetURL, err := h.linear.UploadFile(ctx, entry.Name(), ct, fileData)
		if err != nil {
			log.Printf("⚠️  uploadThreadData: upload %s: %v", entry.Name(), err)
			continue
		}
		uploads = append(uploads, uploadedFile{entry.Name(), assetURL, strings.HasPrefix(ct, "image/")})
	}

	// Build comment: message log + inline images + links for non-images.
	msgData, _ := os.ReadFile(filepath.Join(tt.FilePath, "messages.txt"))
	var sb strings.Builder
	sb.WriteString("## Telegram Thread\n\n")
	sb.WriteString(fmt.Sprintf("Closed by @%s on %s\n", closedBy, time.Now().UTC().Format("2006-01-02 15:04 UTC")))
	if len(msgData) > 0 {
		sb.WriteString("\n---\n\n")
		sb.WriteString(string(msgData))
	}
	if len(uploads) > 0 {
		sb.WriteString("\n---\n\n")
		for _, u := range uploads {
			if u.isImage {
				sb.WriteString(fmt.Sprintf("![%s](%s)\n\n", u.name, u.assetURL))
			} else {
				sb.WriteString(fmt.Sprintf("[%s](%s)\n\n", u.name, u.assetURL))
			}
		}
	}
	if err := h.linear.CreateComment(ctx, tt.LinearIssueID, sb.String()); err != nil {
		log.Printf("⚠️  uploadThreadData: comment failed: %v", err)
	}
	uploaded := len(uploads)

	b.CloseForumTopic(ctx, &tgbot.CloseForumTopicParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
	})

	os.RemoveAll(tt.FilePath)

	elapsed := time.Since(start).Round(time.Second)
	text := fmt.Sprintf("✅ Upload complete. Took %s.", elapsed)
	if uploaded > 0 {
		text = fmt.Sprintf("✅ Upload complete. Took %s. %d file(s) attached to the Linear issue.", elapsed, uploaded)
	}
	b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
		Text:            text,
	})
	log.Printf("✓ Tech thread uploaded: %s (%d files, %s)", tt.LinearIssueURL, uploaded, elapsed)
}
