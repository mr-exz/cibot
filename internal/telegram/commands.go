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

// groupOrder controls the section order in /help output.
var groupOrder = []string{"Support", "Admin", "Topics"}

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
			Desc:      "Create a ticket; reply to a message to use it as source, or run standalone for guided flow",
			GroupDesc: "Create a ticket — reply to any message to open it as a ticket",
			Group:     "Support",
			Handler:   h.handleTicketStart,
		},
		{
			Name:    "mylinear",
			Desc:    "Set or update your Linear account",
			Group:   "Support",
			Handler: h.handleMyLinear,
		},
		{
			Name:      "oncall",
			Desc:      "Show who is on support duty right now (run in a group for topic-specific results)",
			GroupDesc: "Show who is on support duty in this topic right now",
			Group:     "Support",
			Handler:   h.handleOnCall,
		},
		{
			Name:      "status",
			Desc:      "View your duty status and set availability — use in DM for full on-call info",
			GroupDesc: "Set your availability status (lunch, brb, away, back)",
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
			Desc:      "Manage category scopes",
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

// buildHelpText generates the /help message with two sections: Group and DM.
// Admin-only commands appear in the DM section only for admins.
func (h *Handler) buildHelpText(username string) string {
	admin := isAdmin(h.cfg, username)

	var sb strings.Builder
	sb.WriteString("Available commands:\n")

	// Group section — only commands with a GroupDesc set
	sb.WriteString("\nGroup:\n")
	for _, cmd := range h.cmdRegistry {
		if cmd.GroupDesc == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("  /%s — %s\n", cmd.Name, cmd.GroupDesc))
	}

	// DM section — all commands (admin-filtered), with full descriptions
	sb.WriteString("\nDM:\n")
	for _, group := range groupOrder {
		for _, cmd := range h.cmdRegistry {
			if cmd.Group != group {
				continue
			}
			if cmd.AdminOnly && !admin {
				continue
			}
			sb.WriteString(fmt.Sprintf("  /%s — %s\n", cmd.Name, cmd.Desc))
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}
