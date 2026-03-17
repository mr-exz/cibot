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
type commandDef struct {
	Name      string
	Desc      string
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
			Name:    "version",
			Desc:    "Show bot version",
			Group:   "Support",
			Handler: h.handleVersion,
		},
		{
			Name:    "support",
			Desc:    "Create a support request",
			Group:   "Support",
			Handler: h.handleSupportStart,
		},
		{
			Name:    "ticket",
			Desc:    "Reply to a message with this to create a ticket",
			Group:   "Support",
			Handler: h.handleTicketStart,
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
			Name:      "addperson",
			Desc:      "Add a support person",
			Group:     "Admin",
			AdminOnly: true,
			Handler:   h.handleAddPerson,
		},
		{
			Name:      "setrotation",
			Desc:      "Set on-call rotation for a category",
			Group:     "Admin",
			AdminOnly: true,
			Handler:   h.handleSetRotation,
		},
		{
			Name:      "setworkhours",
			Desc:      "Set work hours for a support person",
			Group:     "Admin",
			AdminOnly: true,
			Handler:   h.handleSetWorkHours,
		},
		{
			Name:      "rotation",
			Desc:      "Show current on-duty people",
			Group:     "Admin",
			AdminOnly: true,
			Handler:   h.handleRotation,
		},
		{
			Name:      "granttags",
			Desc:      "Grant the bot can_manage_tags permission in this group",
			Group:     "Admin",
			AdminOnly: true,
			Handler:   h.handleGrantTags,
		},
		{
			Name:      "groups",
			Desc:      "List and approve/disapprove groups",
			Group:     "Admin",
			AdminOnly: true,
			Handler:   h.handleGroups,
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

func (h *Handler) handleVersion(ctx context.Context, b *tgbot.Bot, msg *models.Message) {
	h.sendMessage(ctx, b, msg, fmt.Sprintf("cibot %s\nhttps://github.com/mr-exz/cibot", h.version))
}

// buildHelpText generates the /help message. Admin-only commands are shown
// only to admins; non-admins see only public commands.
func (h *Handler) buildHelpText(username string) string {
	admin := isAdmin(h.cfg, username)

	// Group commands preserving groupOrder
	grouped := make(map[string][]commandDef)
	for _, cmd := range h.cmdRegistry {
		if cmd.AdminOnly && !admin {
			continue
		}
		grouped[cmd.Group] = append(grouped[cmd.Group], cmd)
	}

	var sb strings.Builder
	sb.WriteString("Available commands:\n")

	for _, group := range groupOrder {
		cmds, ok := grouped[group]
		if !ok {
			continue
		}
		sb.WriteString(fmt.Sprintf("\n%s:\n", group))
		for _, cmd := range cmds {
			sb.WriteString(fmt.Sprintf("  /%s — %s\n", cmd.Name, cmd.Desc))
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}
