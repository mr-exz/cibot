# CIBot - Telegram Support Bot

A support ticket bot in **Go** that integrates with **Telegram** and **Linear** with SQLite-backed configuration and support rotation.

## Features

- **Self-service issue creation** (`/support`) — interactive flow with category, request type, title, description, optional media
- **Support-assisted tickets** (`/ticket`) — paste a Telegram message link, bot creates a Linear issue from it
- **Automatic support rotation** (daily/weekly) with on-duty assignment and work-hours awareness
- **Admin commands** for managing categories, topics, support staff, and rotation — usable via DM or group
- **Linear label auto-creation** — category and request type labels created automatically if missing
- **SQLite persistence** — categories, request types, support assignments, topics stored in database
- **Migrations** via `golang-migrate` — versioned, applied once on startup

## Core components

- **Bot runtime**
    - Go service that receives Telegram updates and triggers workflows via long polling
- **Telegram integration**
    - Library: `github.com/go-telegram/bot`
    - Token is created via **@BotFather** and provided as `TELEGRAM_TOKEN`
- **Linear integration**
    - GraphQL API client with team/user/label caching
    - Auto-creates missing labels before issue creation
    - Assigns on-duty person automatically
- **Storage**
    - SQLite (`modernc.org/sqlite`, pure Go, no CGO) with WAL mode
    - Migrations via `golang-migrate/migrate` — versioned `*.up.sql` files, applied once and tracked automatically

## Project layout

```
.
├── cmd/
│   └── bot/              # entrypoint (main.go)
├── internal/
│   ├── config/           # Environment variable parsing
│   ├── telegram/         # Telegram handlers
│   │   ├── telegram.go   # Handler wiring, group/topic cache, session reaper
│   │   ├── commands.go   # Command registry (single source of truth for dispatch + help)
│   │   ├── state.go      # Session state structures
│   │   ├── support.go    # Self-service /support flow
│   │   ├── ticket.go     # Support-assisted /ticket flow
│   │   ├── admin.go      # Admin command entry points
│   │   ├── admin_flow.go # Admin multi-step flow handlers
│   │   └── util.go       # Keyboard builders, link parsers
│   ├── linear/           # Linear GraphQL client with label auto-creation and caching
│   └── storage/          # SQLite (modernc, pure Go) with golang-migrate
│       └── migrations/   # *.up.sql versioned migration files
├── .env.example
├── CHANGELOG.md
├── go.mod
├── go.sum
└── README.md
```

## Environment variables

- `TELEGRAM_TOKEN` (required) — Bot token from @BotFather
- `LINEAR_API_KEY` (required) — Linear API key
- `DB_PATH` (optional, default: `cibot.db`) — SQLite database file path
- `ALLOWED_CHAT_ID` (optional) — Comma-separated Telegram chat IDs to respond to (if empty, responds to all)
- `ADMIN_USERNAMES` (optional) — Comma-separated Telegram usernames allowed to use admin commands (with or without @)
  - Example: `ADMIN_USERNAMES=@alice,@charlie`

## Commands

### User Commands

- `/support` — Start self-service issue creation workflow
  - Select category → request type → enter title and description (optional media)
  - Auto-assigned to on-duty support person
  - Category and request type added as Linear labels (auto-created if missing)

- `/ticket` — Create ticket from an existing Telegram message (support-assisted)
  - Bot prompts for a Telegram message link (`https://t.me/c/...`)
  - Select category and request type
  - Link stored in Linear issue description

- `/help` — Show available commands (admin commands shown only to admins)

### Direct Messages (DMs)
- **Admins only** can send direct messages to the bot
- Non-admins sending DMs are silently ignored

### Admin Commands

- `/addcategory` — Interactive flow to create a category (name → emoji → Linear team key → topic)
- `/addtype` — Add a request type to a category
- `/addperson` — Add a support person with Telegram and Linear usernames
- `/setrotation` — Set rotation period (daily/weekly) for a category
- `/setworkhours` — Set timezone and work hours for a support person
- `/addtopic` — Register a forum topic (group → name → topic ID)
- `/topics` — List all registered topics *(admin only)*
- `/rotation` — Show current on-duty assignments *(admin only)*

## Setup

1. Create a Telegram bot with @BotFather and get the token
2. Create a Linear API key from your workspace settings
3. Copy `.env.example` to `.env` and fill in tokens
   ```bash
   TELEGRAM_TOKEN=your_bot_token
   LINEAR_API_KEY=your_linear_key
   ALLOWED_CHAT_ID=your_group_chat_id
   ADMIN_USERNAMES=@alice,@bob
   ```
4. Run the bot:
   ```bash
   go run ./cmd/bot
   ```
5. Add the bot to your group chat
6. Use `/addtopic` to register forum topics, `/addcategory`, `/addtype`, `/addperson` to configure support (all via DM or group)
7. Users can now use `/support` or `/ticket` to create issues
