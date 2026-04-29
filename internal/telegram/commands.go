package telegram

import (
	"context"
	"fmt"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// cmdHandler is the signature every command handler must match.
type cmdHandler func(ctx context.Context, b *tgbot.Bot, msg *models.Message)

// commandDef describes a bot command: its name, one-line description,
// which UI group it belongs to, and whether it is admin-only.
// GroupDesc is shown in the Group section of /start; leave empty to omit the command from that section.
type commandDef struct {
	Name      string
	Desc      string
	GroupDesc string
	Group     string
	AdminOnly bool
	Handler   cmdHandler
}

// registerCommands returns the full command list. Adding a command here is
// the only thing needed — dispatch and /help are both derived from this list.
func (h *Handler) registerCommands() []commandDef {
	return []commandDef{
		{
			Name:    "start",
			Desc:    "Show available commands",
			Group:   "Support",
			Handler: h.handleStart,
		},
		{
			Name:    "version",
			Desc:    "Show bot version",
			Group:   "Support",
			Handler: h.handleVersion,
		},
		{
			Name:      "ticket",
			Desc:      "Reply to a message to create a support ticket",
			GroupDesc: "Reply to a message to create a support ticket",
			Group:     "Support",
			Handler:   h.handleTicketStart,
		},
		{
			Name:      "ticket_manual",
			Desc:      "Create a support ticket by filling in the details yourself",
			GroupDesc: "Create a support ticket by filling in the details yourself",
			Group:     "Support",
			Handler:   h.handleSupportStart,
		},
		{
			Name:    "mylinear",
			Desc:    "Link or update your Linear account",
			Group:   "Support",
			Handler: h.handleMyLinear,
		},
		{
			Name:      "oncall",
			Desc:      "Show who is on support duty right now",
			GroupDesc: "Show who is on support duty in this topic right now",
			Group:     "Support",
			Handler:   h.handleOnCall,
		},
		{
			Name:      "status",
			Desc:      "Check and update your on-call availability",
			GroupDesc: "Set your availability (lunch, brb, away, back)",
			Group:     "Support",
			Handler:   h.handleSetStatus,
		},
		{
			Name:      "addcategory",
			Desc:      "Add a support category",
			Group:     "Admin",
			AdminOnly: true,
			Handler:   h.handleAddCategory,
		},
		{
			Name:      "addtype",
			Desc:      "Add a request type to a category",
			Group:     "Admin",
			AdminOnly: true,
			Handler:   h.handleAddType,
		},
		{
			Name:      "setrotation",
			Desc:      "Set on-call rotation for a category",
			Group:     "Admin",
			AdminOnly: true,
			Handler:   h.handleSetRotation,
		},
		{
			Name:      "rotation",
			Desc:      "Show current on-duty people",
			Group:     "Admin",
			AdminOnly: true,
			Handler:   h.handleRotation,
		},
		{
			Name:      "groups",
			Desc:      "List and approve/disapprove groups",
			Group:     "Admin",
			AdminOnly: true,
			Handler:   h.handleGroups,
		},
		{
			Name:      "categories",
			Desc:      "Manage support categories",
			Group:     "Admin",
			AdminOnly: true,
			Handler:   h.handleManageCategories,
		},
		{
			Name:      "users",
			Desc:      "List known users and set member tags",
			Group:     "Admin",
			AdminOnly: true,
			Handler:   h.handleUsers,
		},
		{
			Name:      "export",
			Desc:      "Export message log as CSV and reset it",
			Group:     "Admin",
			AdminOnly: true,
			Handler:   h.handleExport,
		},
		{
			Name:      "persons",
			Desc:      "Manage support persons and category assignments",
			Group:     "Admin",
			AdminOnly: true,
			Handler:   h.handlePersonsCommand,
		},
		{
			Name:      "offboard",
			Desc:      "Remove a user from all bot-managed groups",
			Group:     "Admin",
			AdminOnly: true,
			Handler:   h.handleOffboard,
		},
		{
			Name:      "dns",
			Desc:      "Manage DNS records (experimental)",
			Group:     "Admin",
			AdminOnly: true,
			Handler:   h.handleDNS,
		},
		{
			Name:      "thread",
			Desc:      "Reply to a message to open a technical thread — creates a Linear issue and a dedicated topic",
			GroupDesc: "Reply to a message to escalate it as a technical thread",
			Group:     "Support",
			Handler:   h.handleThread,
		},
		{
			Name:      "close",
			Desc:      "Close this thread and post all messages to the Linear issue",
			GroupDesc: "Close this thread and post all messages to the Linear issue",
			Group:     "Support",
			Handler:   h.handleCloseThread,
		},
		{
			Name:      "addtopic",
			Desc:      "Register a forum topic",
			Group:     "Topics",
			AdminOnly: true,
			Handler:   h.handleAddTopic,
		},
		{
			Name:      "topics",
			Desc:      "List registered forum topics",
			Group:     "Topics",
			AdminOnly: true,
			Handler:   h.handleListTopics,
		},
	}
}

func (h *Handler) handleStart(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	h.sendMessage(ctx, b, msg, h.buildHelpText(msg.From.Username))
}

func (h *Handler) handleVersion(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	h.sendMessage(ctx, b, msg, fmt.Sprintf("cibot %s\nhttps://github.com/mr-exz/cibot", h.version))
}

func (h *Handler) buildHelpText(username string) string {
	admin := isAdmin(h.cfg, username)

	var sb strings.Builder
	sb.WriteString("Available commands:\n")

	// User — commands usable in both Group and DM
	sb.WriteString("\nUser — Group & DM:\n")
	for _, cmd := range h.cmdRegistry {
		if cmd.AdminOnly || cmd.GroupDesc == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("  /%s — %s\n", cmd.Name, cmd.GroupDesc))
	}

	// User — DM-only commands (no GroupDesc)
	sb.WriteString("\nUser — DM:\n")
	for _, cmd := range h.cmdRegistry {
		if cmd.AdminOnly || cmd.Group != "Support" || cmd.GroupDesc != "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("  /%s — %s\n", cmd.Name, cmd.Desc))
	}

	// Admin — only visible to admins
	if admin {
		sb.WriteString("\nAdmin:\n")
		for _, group := range []string{"Admin", "Topics"} {
			for _, cmd := range h.cmdRegistry {
				if cmd.Group != group {
					continue
				}
				sb.WriteString(fmt.Sprintf("  /%s — %s\n", cmd.Name, cmd.Desc))
			}
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}
