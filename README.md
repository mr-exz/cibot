# CIBot - Telegram Support Bot

A support ticket bot in **Go** that integrates with **Telegram** and **Linear** with SQLite-backed configuration and support rotation.

## Features

- **Ticket creation** (`/ticket`) — reply to any message to create a Linear issue with reporter, message body, media, and source link auto-captured; or run standalone for a guided flow
- **Automatic support rotation** (daily/weekly) with on-duty assignment and work-hours awareness
- **On-call visibility** (`/oncall`) — any group member can see who is on duty right now per category, with real-time availability status
- **Support person status** (`/status`) — support persons set lunch/brb/away status via inline buttons; member tag updated automatically; shown in ticket confirmations
- **Linear account linking** (`/mylinear`) — users link their Linear account once; used for ticket assignment
- **Multi-group support** — bot works across multiple group chats; groups approved/disapproved via `/groups`
- **Member tagging** — admin forwards a user message to bot in DM to set a Telegram tag on that user in any approved group (Bot API 9.5)
- **Admin commands** for managing categories, topics, support staff, and rotation — usable via DM or group
- **Linear label auto-creation** — category and request type labels created automatically if missing
- **SQLite persistence** — all configuration stored in database with versioned migrations via `golang-migrate`
- **DNS management** *(experimental)* — `/dns` admin command to list, add, and delete ps.kz DNS records from Telegram

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
│   │   ├── dns.go        # DNS management flow (experimental)
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
- `ADMIN_USERNAMES` (optional) — Comma-separated Telegram usernames allowed to use admin commands (with or without @)
  - Example: `ADMIN_USERNAMES=@alice,@charlie`
- `DNS_EMAIL` (optional) — ps.kz account email for DNS management; enables `/dns` when set together with `DNS_PASSWORD`
- `DNS_PASSWORD` (optional) — ps.kz account password for DNS management

## Commands

### User commands

- `/start` — Show available commands
- `/ticket` — Create a support ticket
  - Reply to any message with `/ticket` to use it as the ticket source (category → type → priority → done)
  - Run `/ticket` standalone for a guided self-service flow (category → type → priority → title → description)
  - Reporter, message body, media attachments, and source link captured automatically
  - Auto-assigned to the on-duty support person; assignee status shown if they are on lunch/brb/away
- `/oncall` — Show who is on support duty right now per category (name, @username, availability status)
- `/status` — Set your support status via inline buttons: Lunch / BRB / Away / Back
  - Sets your Telegram member tag in all approved groups; clearing status removes the tag
  - Available to registered support persons only
- `/mylinear` — Set or update your linked Linear username
- `/version` — Show bot version and repository link

### Direct messages (DMs)
- Admins and registered group members can DM the bot
- Non-members are silently ignored

### Admin commands

- `/addcategory` — Create a support category (group → topic → name → emoji → Linear team key)
- `/addtype` — Add a request type to a category; reuse existing types or create new ones
- `/addperson` — Add a support person (Telegram username → Linear username → timezone → work hours → work days)
- `/persons` — List all support persons; tap to view schedule, remove from individual categories, or delete (with confirmation) (Telegram username → Linear username → timezone → work hours → work days); picker keyboards populated from existing DB values
- `/setrotation` — Set rotation period (daily/weekly) for a category
- `/setworkhours` — Update timezone and work schedule for a support person; same picker keyboards as `/addperson`
- `/rotation` — Show current on-duty assignments
- `/groups` — List all known groups with approve/disapprove buttons *(DM only)*
- `/categories` — Manage category scopes (global / group-level / topic-level) and delete categories; clone a category to another group/topic
- `/users` — List known users; tap a user to view profile, set member tag, or delete
- `/export` — Send the current message log as a CSV file and reset it
- `/offboard` — Remove a departed user from all bot-managed groups
- `/addtopic` — Register a forum topic (group → name → topic ID)
- `/topics` — List all registered topics with chat IDs and thread IDs
- `/dns` *(experimental)* — Manage ps.kz DNS records; requires `DNS_EMAIL` and `DNS_PASSWORD` env vars
  - **Accounts** — list billing accounts
  - **List records** — view all records for a domain
  - **Add record** — create a new DNS record (A, AAAA, CNAME, MX, TXT, NS, SRV, CAA) with confirmation step
  - **Delete record** — pick a record from list and delete with confirmation
- **Set member tag** — Forward any user message to the bot in DM → enter label (1–16 chars) → select group → tag applied

## Setup

1. Create a Telegram bot with @BotFather and get the token
2. Create a Linear API key from your workspace settings
3. Copy `.env.example` to `.env` and fill in tokens
   ```bash
   TELEGRAM_TOKEN=your_bot_token
   LINEAR_API_KEY=your_linear_key
   ADMIN_USERNAMES=@alice,@bob
   ```
4. Run the bot:
   ```bash
   go run ./cmd/bot
   ```
5. Add the bot to your group chats as **admin** with the following permissions:
   - Change Group Info, Delete Messages, Ban Users, Add Members, Pin Messages, Manage Topics, Manage Video Chats
   - **Manage Tags** — required for `/setlabel` (set member tags via Bot API 9.5)
6. DM the bot with `/groups` to approve the groups where it should operate
7. Use `/addtopic`, `/addcategory`, `/addtype`, `/addperson` to configure support (all via DM or group)
8. Users can now use `/ticket` to create issues; `/oncall` to see who is on duty
