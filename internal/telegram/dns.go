package telegram

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	pskzdns "github.com/mr-exz/pskz-dns-api"
)

// dnsRecord is a local copy of a DNS record used in delete flow state.
type dnsRecord struct {
	ID    string
	Name  string
	Type  string
	Value string
	TTL   int
}

// handleDNS is the entry point for the /dns command.
func (h *Handler) handleDNS(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	if h.dns == nil {
		h.sendMessage(ctx, b, msg, "DNS management is not configured. Set DNS_EMAIL and DNS_PASSWORD.")
		return
	}

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "Accounts", CallbackData: "dns_act:accounts"},
				{Text: "List records", CallbackData: "dns_act:list"},
			},
			{
				{Text: "Add record", CallbackData: "dns_act:add"},
				{Text: "Delete record", CallbackData: "dns_act:del"},
			},
			{{Text: "Cancel", CallbackData: "cancel"}},
		},
	}

	sentMsg, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:      msg.Chat.ID,
		Text:        "DNS management - choose action:",
		ReplyMarkup: keyboard,
	})
	if err != nil {
		log.Printf("⚠️  DNS: failed to send menu: %v", err)
		return
	}

	key := stateKey{UserID: msg.From.ID}
	h.mu.Lock()
	h.states[key] = &pendingAdminSession{
		Cmd:       AdminCmdDNS,
		Step:      StepDNSMenu,
		MessageID: sentMsg.ID,
		ChatID:    msg.Chat.ID,
		UserID:    msg.From.ID,
		CreatedAt: time.Now(),
	}
	h.mu.Unlock()
}

// handleDNSActionCallback handles the main menu action selection.
func (h *Handler) handleDNSActionCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	state, ok := h.states[key].(*pendingAdminSession)
	h.mu.Unlock()

	if !ok || state == nil || state.Step != StepDNSMenu {
		return
	}

	action := strings.TrimPrefix(query.Data, "dns_act:")
	msg := query.Message.Message

	if action == "accounts" {
		accounts, err := h.dns.ListAccounts(ctx)
		h.mu.Lock()
		delete(h.states, key)
		h.mu.Unlock()
		if err != nil {
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    msg.Chat.ID,
				MessageID: msg.ID,
				Text:      fmt.Sprintf("Error fetching accounts: %v", err),
			})
			return
		}
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    msg.Chat.ID,
			MessageID: msg.ID,
			Text:      formatDNSAccounts(accounts),
		})
		return
	}

	// For list/add/del: show account picker first
	accounts, err := h.dns.ListAccounts(ctx)
	if err != nil {
		h.mu.Lock()
		delete(h.states, key)
		h.mu.Unlock()
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    msg.Chat.ID,
			MessageID: msg.ID,
			Text:      fmt.Sprintf("Error fetching accounts: %v", err),
		})
		return
	}

	state.Step = StepDNSSelectAcct
	state.DNSAction = action
	h.mu.Lock()
	h.states[key] = state
	h.mu.Unlock()

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      msg.Chat.ID,
		MessageID:   msg.ID,
		Text:        "Select account:",
		ReplyMarkup: buildDNSAccountsKeyboard(accounts),
	})
}

// handleDNSAcctCallback handles account selection.
func (h *Handler) handleDNSAcctCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	state, ok := h.states[key].(*pendingAdminSession)
	h.mu.Unlock()

	if !ok || state == nil || state.Step != StepDNSSelectAcct {
		return
	}

	idStr := strings.TrimPrefix(query.Data, "dns_acct:")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return
	}

	msg := query.Message.Message

	if err := h.dns.SelectAccount(ctx, id); err != nil {
		h.mu.Lock()
		delete(h.states, key)
		h.mu.Unlock()
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    msg.Chat.ID,
			MessageID: msg.ID,
			Text:      fmt.Sprintf("Error selecting account: %v", err),
		})
		return
	}

	state.DNSAccountID = id
	state.Step = StepDNSDomain
	h.mu.Lock()
	h.states[key] = state
	h.mu.Unlock()

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    msg.Chat.ID,
		MessageID: msg.ID,
		Text:      "Enter domain name (e.g. example.kz):",
	})
}

// handleAdminDNSPending handles text input during DNS flows.
func (h *Handler) handleAdminDNSPending(ctx context.Context, b *tgbot.Bot, msg *models.Message, admin *pendingAdminSession) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}
	key := stateKey{UserID: msg.From.ID}

	switch admin.Step {
	case StepDNSDomain:
		admin.DNSDomain = text
		switch admin.DNSAction {
		case "list":
			records, err := h.dns.ListRecords(ctx, text)
			h.mu.Lock()
			delete(h.states, key)
			h.mu.Unlock()
			if err != nil {
				log.Printf("⚠️  DNS ListRecords %s: %v", text, err)
				b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
					ChatID:    admin.ChatID,
					MessageID: admin.MessageID,
					Text:      fmt.Sprintf("Error fetching records: %v", err),
				})
				return
			}
			log.Printf("✓ DNS ListRecords %s: %d records returned", text, len(records))
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    admin.ChatID,
				MessageID: admin.MessageID,
				Text:      formatDNSRecords(text, records),
			})

		case "add":
			admin.Step = StepDNSRecName
			h.mu.Lock()
			h.states[key] = admin
			h.mu.Unlock()
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    admin.ChatID,
				MessageID: admin.MessageID,
				Text:      fmt.Sprintf("Domain: %s\n\nEnter record name:", text),
			})

		case "del":
			records, err := h.dns.ListRecords(ctx, text)
			if err != nil {
				h.mu.Lock()
				delete(h.states, key)
				h.mu.Unlock()
				b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
					ChatID:    admin.ChatID,
					MessageID: admin.MessageID,
					Text:      fmt.Sprintf("Error fetching records: %v", err),
				})
				return
			}
			if len(records) == 0 {
				h.mu.Lock()
				delete(h.states, key)
				h.mu.Unlock()
				b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
					ChatID:    admin.ChatID,
					MessageID: admin.MessageID,
					Text:      fmt.Sprintf("No records found for %s.", text),
				})
				return
			}
			localRecords := make([]dnsRecord, len(records))
			for i, r := range records {
				localRecords[i] = dnsRecord{
					ID:    r.ID,
					Name:  r.Name,
					Type:  string(r.Type),
					Value: r.Value,
					TTL:   r.TTL,
				}
			}
			admin.DNSRecords = localRecords
			admin.Step = StepDNSSelectRec
			h.mu.Lock()
			h.states[key] = admin
			h.mu.Unlock()
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:      admin.ChatID,
				MessageID:   admin.MessageID,
				Text:        fmt.Sprintf("Records for %s - select to delete:", text),
				ReplyMarkup: buildDNSRecordsKeyboard(localRecords),
			})
		}

	case StepDNSRecName:
		admin.DNSRecordName = text
		admin.Step = StepDNSRecType
		h.mu.Lock()
		h.states[key] = admin
		h.mu.Unlock()
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      admin.ChatID,
			MessageID:   admin.MessageID,
			Text:        fmt.Sprintf("Domain: %s\nName: %s\n\nSelect record type:", admin.DNSDomain, text),
			ReplyMarkup: buildDNSTypeKeyboard(),
		})

	case StepDNSRecValue:
		admin.DNSRecordValue = text
		admin.Step = StepDNSRecTTL
		h.mu.Lock()
		h.states[key] = admin
		h.mu.Unlock()
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text: fmt.Sprintf("Domain: %s\nName: %s\nType: %s\nValue: %s\n\nEnter TTL in seconds (e.g. 300):",
				admin.DNSDomain, admin.DNSRecordName, admin.DNSRecordType, text),
		})

	case StepDNSRecTTL:
		ttl, err := strconv.Atoi(text)
		if err != nil || ttl <= 0 {
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    admin.ChatID,
				MessageID: admin.MessageID,
				Text: fmt.Sprintf("Domain: %s\nName: %s\nType: %s\nValue: %s\n\nInvalid TTL. Enter a positive number (e.g. 300):",
					admin.DNSDomain, admin.DNSRecordName, admin.DNSRecordType, admin.DNSRecordValue),
			})
			return
		}
		admin.DNSRecordTTL = ttl
		admin.Step = StepDNSConfirm
		h.mu.Lock()
		h.states[key] = admin
		h.mu.Unlock()
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    admin.ChatID,
			MessageID: admin.MessageID,
			Text: fmt.Sprintf("Create record?\n\nDomain: %s\nName: %s\nType: %s\nValue: %s\nTTL: %d",
				admin.DNSDomain, admin.DNSRecordName, admin.DNSRecordType, admin.DNSRecordValue, ttl),
			ReplyMarkup: &models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{
					{
						{Text: "Confirm", CallbackData: "dns_confirm:add"},
						{Text: "Cancel", CallbackData: "cancel"},
					},
				},
			},
		})
	}
}

// handleDNSTypeCallback handles record type selection in the add flow.
func (h *Handler) handleDNSTypeCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	state, ok := h.states[key].(*pendingAdminSession)
	h.mu.Unlock()

	if !ok || state == nil || state.Step != StepDNSRecType {
		return
	}

	recordType := strings.TrimPrefix(query.Data, "dns_type:")
	state.DNSRecordType = recordType
	state.Step = StepDNSRecValue
	h.mu.Lock()
	h.states[key] = state
	h.mu.Unlock()

	msg := query.Message.Message
	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    msg.Chat.ID,
		MessageID: msg.ID,
		Text: fmt.Sprintf("Domain: %s\nName: %s\nType: %s\n\nEnter record value:",
			state.DNSDomain, state.DNSRecordName, recordType),
	})
}

// handleDNSRecCallback handles record selection in the delete flow.
func (h *Handler) handleDNSRecCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	state, ok := h.states[key].(*pendingAdminSession)
	h.mu.Unlock()

	if !ok || state == nil || state.Step != StepDNSSelectRec {
		return
	}

	recordID := strings.TrimPrefix(query.Data, "dns_rec:")
	var recordName, recordType string
	for _, r := range state.DNSRecords {
		if r.ID == recordID {
			recordName = r.Name
			recordType = r.Type
			break
		}
	}

	state.DNSRecordID = recordID
	state.Step = StepDNSConfirm
	h.mu.Lock()
	h.states[key] = state
	h.mu.Unlock()

	msg := query.Message.Message
	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    msg.Chat.ID,
		MessageID: msg.ID,
		Text: fmt.Sprintf("Delete record?\n\nDomain: %s\nName: %s\nType: %s\nID: %s",
			state.DNSDomain, recordName, recordType, recordID),
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{Text: "Confirm delete", CallbackData: "dns_confirm:del"},
					{Text: "Cancel", CallbackData: "cancel"},
				},
			},
		},
	})
}

// handleDNSConfirmCallback handles the final confirmation for add/delete.
func (h *Handler) handleDNSConfirmCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})

	key := stateKey{UserID: query.From.ID}
	h.mu.Lock()
	state, ok := h.states[key].(*pendingAdminSession)
	if ok {
		delete(h.states, key)
	}
	h.mu.Unlock()

	if !ok || state == nil || state.Step != StepDNSConfirm {
		return
	}

	action := strings.TrimPrefix(query.Data, "dns_confirm:")
	msg := query.Message.Message

	switch action {
	case "add":
		record, err := h.dns.CreateRecord(ctx, state.DNSDomain, pskzdns.CreateRecordInput{
			Name:  state.DNSRecordName,
			Type:  pskzdns.RecordType(state.DNSRecordType),
			Value: state.DNSRecordValue,
			TTL:   state.DNSRecordTTL,
		})
		if err != nil {
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    msg.Chat.ID,
				MessageID: msg.ID,
				Text:      fmt.Sprintf("Error creating record: %v", err),
			})
			return
		}
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    msg.Chat.ID,
			MessageID: msg.ID,
			Text: fmt.Sprintf("Record created.\n\nID: %s\nName: %s\nType: %s\nValue: %s\nTTL: %d",
				record.ID, record.Name, record.Type, record.Value, record.TTL),
		})
		log.Printf("✓ DNS record created in %s by @%s: %s %s %s", state.DNSDomain, query.From.Username, record.Name, record.Type, record.Value)

	case "del":
		// Find the record details from state before deleting
		var delName, delType, delValue string
		for _, r := range state.DNSRecords {
			if r.ID == state.DNSRecordID {
				delName = r.Name
				delType = r.Type
				delValue = r.Value
				break
			}
		}
		if err := h.dns.DeleteRecord(ctx, state.DNSDomain, state.DNSRecordID); err != nil {
			b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:    msg.Chat.ID,
				MessageID: msg.ID,
				Text:      fmt.Sprintf("Error deleting record: %v", err),
			})
			return
		}
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    msg.Chat.ID,
			MessageID: msg.ID,
			Text: fmt.Sprintf("Record deleted.\n\nName: %s\nType: %s\nValue: %s",
				delName, delType, delValue),
		})
		log.Printf("✓ DNS record deleted from %s by @%s: %s %s", state.DNSDomain, query.From.Username, delName, delType)
	}
}

func formatDNSAccounts(accounts []pskzdns.Account) string {
	if len(accounts) == 0 {
		return "No accounts found."
	}
	var sb strings.Builder
	sb.WriteString("Accounts:\n\n")
	for _, a := range accounts {
		current := ""
		if a.IsCurrent {
			current = " (current)"
		}
		sb.WriteString(fmt.Sprintf("ID %d — %s%s\n", a.ID, a.CompanyName, current))
	}
	return sb.String()
}

func formatDNSRecords(domain string, records []pskzdns.Record) string {
	if len(records) == 0 {
		return fmt.Sprintf("No records found for %s.", domain)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Records for %s:\n\n", domain))
	for _, r := range records {
		sb.WriteString(fmt.Sprintf("%s  %s  TTL:%d  %s\n", r.Name, r.Type, r.TTL, r.Value))
	}
	return sb.String()
}

func buildDNSAccountsKeyboard(accounts []pskzdns.Account) *models.InlineKeyboardMarkup {
	var rows [][]models.InlineKeyboardButton
	for _, a := range accounts {
		label := fmt.Sprintf("%s (ID: %d)", a.CompanyName, a.ID)
		if a.IsCurrent {
			label += " *"
		}
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: label, CallbackData: fmt.Sprintf("dns_acct:%d", a.ID)},
		})
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "Cancel", CallbackData: "cancel"}})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buildDNSTypeKeyboard() *models.InlineKeyboardMarkup {
	types := []string{"A", "AAAA", "CNAME", "MX", "TXT", "NS", "SRV", "CAA"}
	var rows [][]models.InlineKeyboardButton
	for i := 0; i < len(types); i += 4 {
		var row []models.InlineKeyboardButton
		for j := i; j < i+4 && j < len(types); j++ {
			row = append(row, models.InlineKeyboardButton{
				Text:         types[j],
				CallbackData: "dns_type:" + types[j],
			})
		}
		rows = append(rows, row)
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "Cancel", CallbackData: "cancel"}})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buildDNSRecordsKeyboard(records []dnsRecord) *models.InlineKeyboardMarkup {
	var rows [][]models.InlineKeyboardButton
	for _, r := range records {
		label := fmt.Sprintf("%s %s %s", r.Name, r.Type, r.Value)
		if len(label) > 50 {
			label = label[:47] + "..."
		}
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: label, CallbackData: "dns_rec:" + r.ID},
		})
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "Cancel", CallbackData: "cancel"}})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}
